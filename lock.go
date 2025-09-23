package main

import (
	"context"
	_ "embed"
	"errors"
	"github.com/google/uuid"
	"log"
	"strconv"
	"strings"
	"time"
)

//go:embed lock.lua
var lockScript string

//go:embed renew.lua
var renewScript string

//go:embed delete.lua
var deleteScript string

var internalLockLeaseTime = 30 * time.Second

var lockWatchdogTimeout = internalLockLeaseTime / 3

var unlock_message = 0

var (
	ErrFailed  = errors.New("redis_sync: failed to acquire lock")
	RefreshErr = errors.New("refresh lock time fail")
	ErrUnlock  = errors.New("unlock fail, lock was already expired or held by other")
)

type Lock interface {
	Lock(ctx context.Context) error
	TryLock(ctx context.Context) (bool, error)
	Unlock(ctx context.Context) error
}

type notifier struct {
	c chan struct{}
}

func newNotifier() *notifier {
	return &notifier{make(chan struct{})}
}

func (nt *notifier) notify() {
	close(nt.c)
}

type RedisLock struct {
	Client   *Client
	key      string
	uuid     string
	notifier *notifier
}

func NewLock(key string, client *Client) *RedisLock {
	return &RedisLock{
		Client: client,
		key:    key,
	}
}

func (l *RedisLock) Lock(ctx context.Context) error {
	return l.lock(ctx, -1)
}

func (l *RedisLock) lock(ctx context.Context, leaseTime time.Duration) (err error) {
	ttl, err := l.tryAcquire(ctx, -1, leaseTime)
	if err != nil || ttl == nil {
		return
	}

	pubsub := l.Client.Subscribe(ctx, l.getChannelName())
	ch := pubsub.Channel()
	defer pubsub.Close()
	var timer *time.Timer

	for {
		ttl, err := l.tryAcquire(ctx, -1, leaseTime)
		if err != nil || ttl == nil {
			break
		}

		if timer == nil {
			timer = time.NewTimer(*ttl)
		} else {
			timer.Reset(*ttl)
		}

		select {
		case <-ch:
			timer.Stop()
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ErrFailed
		}
	}

	return
}

func (l *RedisLock) TryLock(ctx context.Context) (bool, error) {
	ttl, err := l.tryAcquire(ctx, -1, -1)
	if err != nil || ttl == nil {
		return true, nil
	}

	return false, err
}

func (l *RedisLock) tryAcquire(ctx context.Context, waitTime time.Duration, leaseTime time.Duration) (ttlRemaining *time.Duration, err error) {
	value := uuid.New().String()
	if leaseTime > 0 {

	} else {
		ttlRemaining, err = l.tryLockInnerAsync(ctx, value, waitTime, internalLockLeaseTime)
	}
	//lock fail
	if err != nil || ttlRemaining != nil {
		return
	}

	//watchDog
	n := newNotifier()
	l.uuid = value
	l.notifier = n

	if leaseTime > 0 {

	} else {
		go l.scheduleExpirationRenewal(n, value)
	}
	return
}

func (l *RedisLock) tryLockInnerAsync(ctx context.Context, value string, waitTime time.Duration, leaseTime time.Duration) (*time.Duration, error) {
	leaseVal := strconv.FormatInt(int64(leaseTime/time.Millisecond), 10)

	key := []string{l.key}
	args := []interface{}{
		leaseVal,
		value,
	}

	pttl, err := l.Client.Eval(ctx, lockScript, key, args).Result()

	if err != nil {
		if strings.Contains(err.Error(), "redis: nil") {
			err = nil
		}
		return nil, err
	}

	t := time.Duration(pttl.(int64)) * time.Millisecond

	return &t, err

}

func (l *RedisLock) getChannelName() string {
	return "redisson_lock__channel" + l.key
}

func (l *RedisLock) Refresh(ctx context.Context, expireTime time.Duration) error {
	_, err := l.renewExpirationAsync(ctx, l.uuid, expireTime)
	return err
}

func (l *RedisLock) scheduleExpirationRenewal(notifier *notifier, value string) {

	ticker := time.NewTicker(lockWatchdogTimeout)

	for {
		select {
		case <-ticker.C:
			_, err := l.renewExpirationAsync(context.Background(), value, internalLockLeaseTime)
			if err != nil {
				log.Println(err)
				return
			}
		case <-notifier.c:
			ticker.Stop()
			return
		}
	}
}

func (l *RedisLock) renewExpirationAsync(ctx context.Context, value string, expireTime time.Duration) (bool, error) {

	expireVal := strconv.FormatInt(int64(expireTime/time.Millisecond), 10)

	key := []string{l.key}
	args := []interface{}{
		expireVal,
		value,
	}

	res, err := l.Client.Eval(ctx, renewScript, key, args).Result()
	val, ok := res.(int64)
	if !ok {
		return false, err
	}
	if val == 0 {
		return false, RefreshErr
	}

	return true, err
}

func (l *RedisLock) Unlock(ctx context.Context) error {
	n := l.notifier

	_, err := l.unlockInnerAsync(ctx, l.uuid)
	if err != nil {
		return err
	}
	if n != nil {
		n.notify()
	}
	return nil
}

func (l *RedisLock) unlockInnerAsync(ctx context.Context, value string) (bool, error) {

	key := []string{
		l.key,
		l.getChannelName(),
	}
	args := []interface{}{
		unlock_message,
		value,
	}

	res, err := l.Client.Eval(ctx, deleteScript, key, args).Result()

	val, ok := res.(int64)
	if !ok {
		return false, err
	}
	if val == 0 {
		return false, ErrUnlock
	}
	return val == 1, nil
}
