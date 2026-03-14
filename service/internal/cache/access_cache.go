package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// AccessCacheEntry represents a cached access check result.
type AccessCacheEntry struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// AccessCache provides caching for document access permission checks.
type AccessCache struct {
	redis *RedisClient
}

// NewAccessCache creates a new AccessCache backed by the given RedisClient.
func NewAccessCache(redis *RedisClient) *AccessCache {
	return &AccessCache{redis: redis}
}

// accessKey builds the Redis key for an access check.
// Format: access:{documentID}:{userID}
func accessKey(documentID, userID string) string {
	return fmt.Sprintf("access:%s:%s", documentID, userID)
}

// GetAccessCheck returns a cached access check result.
// Returns nil on cache miss.
func (c *AccessCache) GetAccessCheck(ctx context.Context, documentID, userID string) *AccessCacheEntry {
	key := accessKey(documentID, userID)
	val, ok := c.redis.Get(ctx, key)
	if !ok {
		return nil
	}

	var entry AccessCacheEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		log.Printf("[CACHE] Failed to unmarshal access cache key=%s: %v", key, err)
		return nil
	}

	log.Printf("[CACHE] HIT access check: doc=%s user=%s allowed=%v", documentID, userID, entry.Allowed)
	return &entry
}

// SetAccessCheck stores an access check result in cache.
func (c *AccessCache) SetAccessCheck(ctx context.Context, documentID, userID string, allowed bool, reason string) {
	key := accessKey(documentID, userID)
	entry := AccessCacheEntry{
		Allowed: allowed,
		Reason:  reason,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[CACHE] Failed to marshal access cache: %v", err)
		return
	}

	c.redis.Set(ctx, key, string(data))
	log.Printf("[CACHE] SET access check: doc=%s user=%s allowed=%v", documentID, userID, allowed)
}

// InvalidateAccess removes the cached access check for a specific document+user pair.
func (c *AccessCache) InvalidateAccess(ctx context.Context, documentID, userID string) {
	key := accessKey(documentID, userID)
	c.redis.Del(ctx, key)
	log.Printf("[CACHE] INVALIDATED access: doc=%s user=%s", documentID, userID)
}
