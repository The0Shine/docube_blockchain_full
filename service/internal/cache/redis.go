package cache

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps the go-redis client with application-specific helpers.
type RedisClient struct {
	client *redis.Client
	ttl    time.Duration
}

// RedisConfig holds Redis connection parameters.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	TTL      int // seconds
}

// NewRedisClient creates and pings a new Redis connection.
func NewRedisClient(cfg RedisConfig) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	ttl := time.Duration(cfg.TTL) * time.Second
	if ttl == 0 {
		ttl = 15 * time.Minute
	}

	log.Printf("[REDIS] Connected to %s (db=%d, ttl=%s)", cfg.Addr, cfg.DB, ttl)

	return &RedisClient{
		client: rdb,
		ttl:    ttl,
	}, nil
}

// Get retrieves a value by key. Returns empty string and false on miss.
func (r *RedisClient) Get(ctx context.Context, key string) (string, bool) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false
	}
	if err != nil {
		log.Printf("[REDIS] GET error key=%s: %v", key, err)
		return "", false
	}
	return val, true
}

// Set stores a key-value pair with the configured TTL.
func (r *RedisClient) Set(ctx context.Context, key, value string) {
	if err := r.client.Set(ctx, key, value, r.ttl).Err(); err != nil {
		log.Printf("[REDIS] SET error key=%s: %v", key, err)
	}
}

// Del removes one or more keys.
func (r *RedisClient) Del(ctx context.Context, keys ...string) {
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		log.Printf("[REDIS] DEL error keys=%v: %v", keys, err)
	}
}

// Close closes the underlying Redis connection.
func (r *RedisClient) Close() error {
	return r.client.Close()
}
