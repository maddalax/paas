package app

import (
	"dockman/app/logger"
	"dockman/app/util"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"time"
)

type DistributedLock struct {
	key     string
	timeout time.Duration
	c       *KvClient
}

func (c *KvClient) NewLock(key string, timeout time.Duration) *DistributedLock {
	return &DistributedLock{
		key:     key,
		c:       c,
		timeout: timeout,
	}
}

func (l *DistributedLock) Bucket() string {
	return fmt.Sprintf("locks-%s", l.key)
}

func (l *DistributedLock) Lock() error {
	bucket, err := l.c.GetOrCreateBucket(&nats.KeyValueConfig{
		Bucket: l.Bucket(),
		TTL:    l.timeout,
	})
	if err != nil {
		return err
	}
	_, err = bucket.Create(l.key, []byte("locked"))
	if err != nil {
		if errors.Is(err, nats.ErrKeyExists) {
			// wait for the lock to be released
			success := util.WaitFor(l.timeout, 25*time.Millisecond, func() bool {
				logger.DebugWithFields("waiting for lock", map[string]any{
					"key": l.key,
				})
				_, err = bucket.Create(l.key, []byte("locked"))
				return err == nil
			})
			if !success {
				return errors.New("lock timeout")
			}
		} else {
			return err
		}
	}
	return nil
}

func (l *DistributedLock) Unlock() error {
	bucket, err := l.c.GetBucket(l.Bucket())
	if err != nil {
		return err
	}
	logger.DebugWithFields("unlocking %s", map[string]any{
		"key": l.key,
	})
	return bucket.Delete(l.key)
}
