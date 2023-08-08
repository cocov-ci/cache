package redis

import (
	"context"
	"encoding/json"
	"github.com/heyvito/go-leader/leader"
	"go.uber.org/zap"
	"strings"
	"time"

	"github.com/heyvito/redlock-go"
	"github.com/redis/go-redis/v9"
)

type RepoIdentifier struct {
	Name string `json:"name"`
	ID   int64  `json:"id"`
}

const housekeepingTasksListName = "cocov:cached:housekeeping_tasks"
const cachedClientPrefix = "cocov:cached:client:"
const cacheLockKey = "cocov:cached:cache_lock"

type Client interface {
	RepoDataFromJID(jid string) (*RepoIdentifier, error)
	Locking(id string, timeout time.Duration, fn func() error) error
	MakeLeader(opts leader.Opts) (lead leader.Leader, onPromote <-chan time.Time, onDemote <-chan time.Time, onError <-chan error)
	NextHousekeepingTask() (string, error)
	RequeueHousekeepingTask(task string) error
}

func New(url string) (Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}

	i := &impl{
		r:   redis.NewClient(opts),
		log: zap.L().With(zap.String("component", "janitor")),
	}
	if err = i.r.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	i.l = redlock.New(i.r)
	return i, nil
}

type impl struct {
	r   *redis.Client
	l   redlock.Redlock
	log *zap.Logger
}

func (i impl) NextHousekeepingTask() (string, error) {
	v, err := i.r.BLPop(context.Background(), 2*time.Second, housekeepingTasksListName).Result()
	if err == redis.Nil {
		return "", nil
	} else if err != nil {
		return "", err
	}

	return v[1], nil
}

func (i impl) MakeLeader(opts leader.Opts) (lead leader.Leader, onPromote <-chan time.Time, onDemote <-chan time.Time, onError <-chan error) {
	opts.Redis = i.r
	return leader.NewLeader(opts)
}

func (i impl) RepoDataFromJID(jid string) (*RepoIdentifier, error) {
	val, err := i.r.Get(context.Background(), cachedClientPrefix+jid).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	id := RepoIdentifier{}
	if err = json.Unmarshal([]byte(val), &id); err != nil {
		return nil, err
	}

	return &id, nil
}

func (i impl) Locking(id string, ttl time.Duration, fn func() error) error {
	key := strings.Join([]string{cacheLockKey, id}, ":")
	return i.l.Locking(key, int(ttl.Milliseconds()), fn)
}

func (i impl) RequeueHousekeepingTask(task string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	timeout := 500 * time.Millisecond
	for {
		err := i.r.LPush(context.Background(), housekeepingTasksListName, task).Err()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				i.log.Error("Failed requeueing task. Will not retry.",
					zap.NamedError("context_error", ctx.Err()),
					zap.NamedError("requeue_error", err))
				return err
			}

			i.log.Warn("Failed requeueing task. Will retry.",
				zap.Duration("waiting", timeout),
				zap.Error(err))
			time.Sleep(timeout)
			timeout *= 2
			continue
		}
		break
	}
	return nil
}
