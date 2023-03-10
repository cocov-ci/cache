package redis

import (
	"context"
	"strings"
	"time"

	"github.com/heyvito/redlock-go"
	"github.com/redis/go-redis/v9"
)

type Client interface {
	RepoNameFromJID(jid string) (bool, string, error)
	Locking(path []string, timeout time.Duration, fn func() error) error
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

func (i impl) RepoNameFromJID(jid string) (bool, string, error) {
	val, err := i.r.Get(context.Background(), "cocov:cache_client:"+jid).Result()
	if err == redis.Nil {
		return false, "", nil
	} else if err != nil {
		return false, "", err
	}

	return true, val, nil
}

func (i impl) Locking(path []string, ttl time.Duration, fn func() error) error {
	key := strings.Join(append([]string{"cocov:cache_lock"}, path...), ":")
	return i.l.Locking(key, int(ttl.Milliseconds()), fn)
}
