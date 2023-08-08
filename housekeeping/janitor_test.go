package housekeeping

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cocov-ci/cache/locator"
	"github.com/cocov-ci/cache/mocks"
	"github.com/cocov-ci/cache/redis"
	"github.com/google/uuid"
	"github.com/heyvito/locking-pool"
	redis2 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

// These tests will require a Redis server to be available.

func waitUntil(t *testing.T, timeout time.Duration, fn func(t *testing.T, done func())) {
	t.Helper()
	mut := sync.Mutex{}
	reported := false
	timer := time.NewTimer(timeout)
	ok := make(chan bool)

	go func() {
		<-timer.C
		timer.Stop()
		mut.Lock()
		defer mut.Unlock()

		if reported {
			return
		}
		reported = true
		ok <- false
		close(ok)
	}()

	doneFunc := func() {
		mut.Lock()
		defer mut.Unlock()
		if reported {
			return
		}
		reported = true
		ok <- true
		close(ok)
	}

	go fn(t, doneFunc)

	if <-ok {
		return
	}

	t.Fatalf("Function failed to call done() after %s", timeout)
}

type testBag struct {
	// Just to keep references around
	janitors []*Janitor

	redis    redis.Client
	rawRedis *redis2.Client
	api      *mocks.MockAPIClient
	storage  *mocks.MockProvider
}

func (b *testBag) emitTask(t *testing.T, task any) {
	t.Helper()
	jData, err := json.Marshal(task)
	require.NoError(t, err)
	require.NoError(t, b.rawRedis.RPush(context.Background(), "cocov:cached:housekeeping_tasks", string(jData)).Err())
}

func (b *testBag) makeJanitor(t *testing.T) *Janitor {
	t.Helper()
	j := New(b.redis, b.api, b.storage)
	j.keepJobCount()
	go j.Start()
	t.Cleanup(j.Stop)
	b.janitors = append(b.janitors, j)
	waitUntil(t, 2*time.Second, waitLeader(j))
	return j
}

var poolMu = &sync.Mutex{}
var databasePool locking_pool.LockingPool[int]

func getRedisDatabase(t *testing.T) (string, *redis2.Client) {
	t.Helper()
	baseURL, ok := os.LookupEnv("CACHE_REDIS_URL")
	if !ok {
		baseURL = "redis://localhost:6379"
	}

	redisURL, err := url.Parse(baseURL)
	require.NoError(t, err)
	redisURL.Path = ""

	poolMu.Lock()
	if databasePool == nil {
		pool := make([]int, 16)
		for i := range pool {
			pool[i] = i
		}
		databasePool = locking_pool.New(pool)
	}
	poolMu.Unlock()

	id := databasePool.Get()
	t.Cleanup(func() { databasePool.Return(id) })

	redisURL.Path = fmt.Sprintf("/%d", id)
	finalURL := redisURL.String()

	opts, err := redis2.ParseURL(finalURL)
	require.NoError(t, err)
	r := redis2.NewClient(opts)
	require.NoError(t, r.FlushDB(context.Background()).Err())

	return finalURL, r
}

func makeTestBag(t *testing.T) *testBag {
	t.Helper()

	ctrl := gomock.NewController(t)
	api := mocks.NewMockAPIClient(ctrl)
	prov := mocks.NewMockProvider(ctrl)

	l, err := zap.NewDevelopment()
	require.NoError(t, err)
	zap.ReplaceGlobals(l)
	redisURL, r2 := getRedisDatabase(t)
	r, err := redis.New(redisURL)
	require.NoError(t, err)
	return &testBag{
		redis:    r,
		rawRedis: r2,
		api:      api,
		storage:  prov,
	}
}

func waitLeader(j *Janitor) func(_ *testing.T, done func()) {
	return func(_ *testing.T, done func()) {
		for {
			if j.leading {
				done()
				break
			}
			time.Sleep(1 * time.Millisecond)
		}
	}
}

func waitEmptyTaskList(bag *testBag) func(*testing.T, func()) {
	return func(t *testing.T, done func()) {
		defer done()
		for {
			i, err := bag.rawRedis.LLen(context.Background(), "cocov:cached:housekeeping_tasks").Result()
			require.NoError(t, err)
			if i == 0 {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}

func waitJobs(janitor *Janitor, count int) func(*testing.T, func()) {
	return func(t *testing.T, done func()) {
		defer done()
		for {
			if len(janitor.jobsPerformed) == count {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func TestLead(t *testing.T) {
	bag := makeTestBag(t)
	j := bag.makeJanitor(t)

	waitUntil(t, 2*time.Second, waitLeader(j))
}

func TestEvictArtifact(t *testing.T) {
	bag := makeTestBag(t)
	j := bag.makeJanitor(t)

	gomock.InOrder(
		bag.storage.EXPECT().DeleteArtifact(gomock.Eq(&locator.ArtifactLocator{
			RepositoryID: 10,
			NameHash:     "a",
		})).Return(nil),

		bag.storage.EXPECT().DeleteArtifact(&locator.ArtifactLocator{
			RepositoryID: 10,
			NameHash:     "b",
		}).Return(nil),
	)

	bag.emitTask(t, EvictTask{
		Name:       "evict-artifact",
		ID:         uuid.NewString(),
		Repository: 10,
		Objects:    []string{"a", "b"},
	})

	waitUntil(t, 2*time.Second, waitEmptyTaskList(bag))
	waitUntil(t, 2*time.Second, waitJobs(j, 1))
}

func TestPurgeRepository(t *testing.T) {
	bag := makeTestBag(t)
	j := bag.makeJanitor(t)

	bag.storage.EXPECT().PurgeRepository(int64(10)).Return(nil)

	bag.emitTask(t, PurgeTask{
		Name:       "purge-repository",
		ID:         uuid.NewString(),
		Repository: 10,
	})

	waitUntil(t, 2*time.Second, waitEmptyTaskList(bag))
	waitUntil(t, 2*time.Second, waitJobs(j, 1))
}

func TestEvictTool(t *testing.T) {
	bag := makeTestBag(t)
	j := bag.makeJanitor(t)

	bag.storage.EXPECT().DeleteTool(gomock.Eq(&locator.ToolLocator{
		NameHash: "a",
	})).Return(nil)

	bag.storage.EXPECT().DeleteTool(gomock.Eq(&locator.ToolLocator{
		NameHash: "b",
	})).Return(nil)

	bag.emitTask(t, EvictToolTask{
		Name:    "evict-tool",
		ID:      uuid.NewString(),
		Objects: []string{"a", "b"},
	})

	waitUntil(t, 2*time.Second, waitEmptyTaskList(bag))
	waitUntil(t, 2*time.Second, waitJobs(j, 1))
}

func TestPurgeTool(t *testing.T) {
	bag := makeTestBag(t)
	j := bag.makeJanitor(t)

	bag.storage.EXPECT().PurgeTool().Return(nil)

	bag.emitTask(t, PurgeToolTask{
		Name: "purge-tool",
		ID:   uuid.NewString(),
	})

	waitUntil(t, 2*time.Second, waitEmptyTaskList(bag))
	waitUntil(t, 2*time.Second, waitJobs(j, 1))
}
