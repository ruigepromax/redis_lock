package main

import (
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	redis.UniversalClient
}

func NewClient(client redis.UniversalClient) *Client {
	return &Client{client}
}

func (cl *Client) NewLock(key string) *RedisLock {
	return &RedisLock{
		Client: cl,
		key:    key,
		uuid:   uuid.New().String(),
	}
}
