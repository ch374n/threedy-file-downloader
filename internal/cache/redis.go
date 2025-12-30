package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(addr, password string, db int, ttl time.Duration) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
		ttl:    ttl,
	}, nil
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		// Key doesn't exist - cache miss
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get error: %w", err)
	}
	// Cache hit
	return data, true, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, data []byte) error {
	err := c.client.Set(ctx, key, data, c.ttl).Err()
	if err != nil {
		return fmt.Errorf("redis set error: %w", err)
	}
	return nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}
