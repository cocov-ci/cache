package redis

import (
	"context"
	"encoding/json"
	"github.com/heyvito/go-leader/leader"
	"strings"
	"time"

	"github.com/heyvito/redlock-go"
	"github.com/redis/go-redis/v9"
)

type RepoIdentifier struct {
	Name string `json:"name"`
	ID   int64  `json:"id"`
}

type Client interface {
	RepoDataFromJID(jid string) (*RepoIdentifier, error)
	Locking(id string, timeout time.Duration, fn func() error) error
	MakeLeader(opts leader.Opts) (lead leader.Leader, onPromote <-chan time.Time, onDemote <-chan time.Time, onError <-chan error)
	NextHousekeepingTask() (string, error)
}

func New(url string) (Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}

	i := &impl{r: redis.NewClient(opts)}
	if err = i.r.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	i.l = redlock.New(i.r)
	return i, nil
}

type impl struct {
	r *redis.Client
	l redlock.Redlock
}

func (i impl) NextHousekeepingTask() (string, error) {
	v, err := i.r.BLPop(context.Background(), 2*time.Second, "cocov:cached:housekeeping_tasks").Result()
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
	val, err := i.r.Get(context.Background(), "cocov:cached:client:"+jid).Result()
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
	key := strings.Join([]string{"cocov:cached:cache_lock", id}, ":")
	return i.l.Locking(key, int(ttl.Milliseconds()), fn)
}
