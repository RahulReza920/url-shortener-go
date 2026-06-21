// Package ratelimit implements a fixed-window per-IP limiter backed by
// Redis, applied only to the create-link endpoint (the redirect path is
// intentionally unthrottled — see SPEC.md section 8).
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	rdb    *redis.Client
	window time.Duration
}

func New(rdb *redis.Client, window time.Duration) *Limiter {
	return &Limiter{rdb: rdb, window: window}
}

// Allow reports whether ip may perform another action under the given
// limit (max requests per window), incrementing its counter as a side
// effect.
func (l *Limiter) Allow(ctx context.Context, ip string, max int64) (bool, error) {
	key := fmt.Sprintf("ratelimit:create:%s", ip)
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		l.rdb.Expire(ctx, key, l.window)
	}
	return count <= max, nil
}
