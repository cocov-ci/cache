package housekeeping

import (
	"context"
	"encoding/json"
	"github.com/cocov-ci/cache/locator"
	"github.com/cocov-ci/cache/mocha"
	"github.com/cocov-ci/cache/mocks"
	"github.com/cocov-ci/cache/redis"
	"github.com/google/uuid"
	"github.com/heyvito/mochaccino"
	redis2 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"os"
	"sync"
	"testing"
	"time"
)

// These tests will require a Redis server to be available.

func waitUntil(t *testing.T, timeout time.Duration, fn func(done func())) {
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

	go fn(doneFunc)

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
	api      *mocks.APIClient
	storage  *mocks.MockProvider
}

func (b *testBag) emitTask(t *testing.T, task any) {
	jData, err := json.Marshal(task)
	require.NoError(t, err)
	require.NoError(t, b.rawRedis.RPush(context.Background(), "cocov:cached:housekeeping_tasks", string(jData)).Err())
}

func (b *testBag) makeJanitor(t *testing.T) *Janitor {
	j := New(b.redis, b.api, b.storage)
	go j.Start()
	t.Cleanup(j.Stop)
	b.janitors = append(b.janitors, j)
	waitUntil(t, 2*time.Second, waitLeader(j))
	return j
}

func makeTestBag(t *testing.T) *testBag {
	rURL, ok := os.LookupEnv("CACHE_REDIS_URL")
	if !ok {
		rURL = "redis://localhost:6379"
	}

	r, err := redis.New(rURL)
	require.NoError(t, err)

	opts, err := redis2.ParseURL(rURL)
	require.NoError(t, err)
	r2 := redis2.NewClient(opts)
	require.NoError(t, r2.FlushDB(context.Background()).Err())

	ctrl := gomock.NewController(t)
	api := mocks.NewAPIClient(ctrl)
	prov := mocks.NewMockProvider(ctrl)

	l, err := zap.NewDevelopment()
	require.NoError(t, err)
	zap.ReplaceGlobals(l)

	return &testBag{
		redis:    r,
		rawRedis: r2,
		api:      api,
		storage:  prov,
	}
}

func waitLeader(j *Janitor) func(done func()) {
	return func(done func()) {
		for {
			if j.leading {
				done()
				break
			}
			time.Sleep(1 * time.Millisecond)
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
	bag.makeJanitor(t)

	bag.storage.EXPECT().DeleteArtifact(gomock.Eq(&locator.ArtifactLocator{
		RepositoryID: 10,
		NameHash:     "a",
	})).Return(nil)

	bag.storage.EXPECT().DeleteArtifact(&locator.ArtifactLocator{
		RepositoryID: 10,
		NameHash:     "b",
	}).Return(nil)

	bag.emitTask(t, EvictTask{
		Name:       "evict-artifact",
		ID:         uuid.NewString(),
		Repository: 10,
		Objects:    []string{"a", "b"},
	})

	time.Sleep(1 * time.Second)
}

func TestFoo(t *testing.T) {
	ctrl := mochaccino.NewController(t)
	store := mocha.NewProviderMock(ctrl)
	store.EXPECT().
		DeleteArtifact().
		With(&locator.ArtifactLocator{
			RepositoryID: 10,
			NameHash:     "a",
		}).
		Return(nil)
	store.EXPECT().
		DeleteArtifact().
		With(&locator.ArtifactLocator{
			RepositoryID: 10,
			NameHash:     "b",
		}).
		Return(nil)
}
