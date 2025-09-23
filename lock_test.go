package main

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"sync"
	"testing"
	"time"
)

var key = "test_lock"

var opts = &redis.Options{
	Addr:     "localhost:6379",
	Password: "",
	DB:       0,
}

func TestNewClient(t *testing.T) {

	// Create Redis client
	rc := redis.NewClient(opts)

	ctx := context.Background()
	pong, err := rc.Ping(ctx).Result()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v\n", err)
	}
	fmt.Printf("Connection successful: %s\n", pong)

}

func TestLockClient(t *testing.T) {

	rc := redis.NewClient(opts)
	client := NewClient(rc)

	ctx := context.Background()
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v\n", err)
	}
	fmt.Printf("Connection successful: %s\n", pong)

}

func TestLockAndUnlock(t *testing.T) {
	c := redis.NewClient(opts)
	client := NewClient(c)
	lock := client.NewLock(key)
	ctx := context.Background()
	err := lock.Lock(ctx)
	if err != nil {
		t.Errorf("Failed to lock Redis: %v\n", err)
	}

	flag := true
	go func() {
		err := lock.Lock(ctx)
		if err != nil {
			t.Errorf("Failed to lock Redis: %v\n", err)
		}
		if flag {
			t.Errorf("Lock is invalid")
		}
	}()
	time.Sleep(1 * time.Second)
	flag = false
	err = lock.Unlock(ctx)
	if err != nil {
		t.Errorf("Failed to unlock Redis: %v\n", err)
	}
}

func TestMutiLock(t *testing.T) {

	c := redis.NewClient(opts)
	client := NewClient(c)

	lock := client.NewLock(key)

	count := 0
	numThreads := 100
	var wg sync.WaitGroup
	ch := make(chan struct{})
	ctx := context.Background()

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ch:
			}

			err := lock.Lock(ctx)
			defer lock.Unlock(ctx)
			count++
			if err != nil {
				t.Errorf("Failed to lock Redis: %v\n", err)
			}
		}()
	}

	close(ch)
	wg.Wait()

	exp := numThreads
	if count != exp {
		t.Fatalf("expected %v, count %v", exp, count)
	}

}

func TestTryLockAndUnlock(t *testing.T) {
	c := redis.NewClient(opts)
	client := NewClient(c)

	lock := client.NewLock(key)

	ctx := context.Background()

	err := lock.Lock(ctx)
	if err != nil {
		t.Errorf("Failed to lock Redis: %v\n", err)
	}

	flag := false
	go func() {
		isLock, err := lock.TryLock(context.Background())
		if err != nil {
			t.Errorf("Failed to try lock Redis: %v\n", err)
		}
		if isLock || flag {
			t.Errorf("try lock success")
		}
	}()

	time.Sleep(1 * time.Second)

	flag = true
	err = lock.Unlock(ctx)
	if err != nil {
		t.Errorf("Failed to unlock Redis: %v\n", err)
	}
}

func TestWatchDog(t *testing.T) {
	c := redis.NewClient(opts)
	client := NewClient(c)
	lock := client.NewLock(key)
	ctx := context.Background()
	err := lock.Lock(ctx)
	if err != nil {
		t.Errorf("Failed to lock Redis: %v\n", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(30 * time.Second)
		isLocked, err := lock.TryLock(ctx)
		if err != nil || isLocked {
			t.Errorf("Watchdog mechanism failed: %v\n", err)
		}
	}()
	wg.Wait()
	err = lock.Unlock(ctx)
	if err != nil {
		t.Errorf("Failed to unlock Redis: %v\n", err)
	}
}

func TestRefresh(t *testing.T) {
	c := redis.NewClient(opts)
	client := NewClient(c)
	lock := client.NewLock(key)
	ctx := context.Background()
	ch := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ch:
		}
		ttl, err := lock.tryLockInnerAsync(ctx, key, -1, -1)
		if err != nil {
			t.Errorf("Failed to try lock Redis: %v\n", err)
		}
		if ttl == nil {
			t.Errorf("Failed lock")
		}
		if *ttl+2*time.Second < 1*time.Hour {
			t.Errorf("Refresh fail,ttl is: %v\n", *ttl)
		}
	}()

	err := lock.Lock(ctx)
	if err != nil {
		t.Errorf("Failed to lock Redis: %v\n", err)
	}
	err = lock.Refresh(ctx, 1*time.Hour)
	if err != nil {
		t.Errorf("Failed to set Redis lock expiration time: %v\n", err)
	}
	close(ch)
	wg.Wait()
	err = lock.Unlock(ctx)
	if err != nil {
		t.Errorf("Failed to unlock Redis: %v\n", err)
	}

}
