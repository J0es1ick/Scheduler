package site

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	publicInfoCacheTTL = time.Minute
	publicInfoErrorTTL = 5 * time.Second
)

type publicInfoCache struct {
	mu sync.Mutex

	value      *PublicInfo
	valueUntil time.Time
	lastErr    error
	errorUntil time.Time
	loading    bool
	wait       chan struct{}
	now        func() time.Time
}

func newPublicInfoCache() *publicInfoCache {
	return &publicInfoCache{now: time.Now}
}

func (c *publicInfoCache) Get(
	ctx context.Context,
	load func(context.Context) (*PublicInfo, error),
) (*PublicInfo, error) {
	for {
		now := c.now()
		c.mu.Lock()
		if c.value != nil && now.Before(c.valueUntil) {
			result := clonePublicInfo(c.value)
			c.mu.Unlock()
			return result, nil
		}
		if c.lastErr != nil && now.Before(c.errorUntil) {
			err := c.lastErr
			c.mu.Unlock()
			return nil, err
		}
		if c.loading {
			wait := c.wait
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
				continue
			}
		}

		c.loading = true
		c.wait = make(chan struct{})
		c.mu.Unlock()

		result, err := load(ctx)
		if err == nil && result == nil {
			err = errors.New("public info loader returned no data")
		}

		c.mu.Lock()
		finishedAt := c.now()
		if err == nil {
			c.value = clonePublicInfo(result)
			c.valueUntil = finishedAt.Add(publicInfoCacheTTL)
			c.lastErr = nil
			c.errorUntil = time.Time{}
		} else if !errors.Is(err, context.Canceled) {
			c.lastErr = err
			c.errorUntil = finishedAt.Add(publicInfoErrorTTL)
		}
		c.loading = false
		close(c.wait)
		c.mu.Unlock()

		if err != nil {
			return nil, err
		}
		return clonePublicInfo(result), nil
	}
}

func clonePublicInfo(value *PublicInfo) *PublicInfo {
	if value == nil {
		return nil
	}
	result := *value
	result.UniversityNames = append([]string(nil), value.UniversityNames...)
	return &result
}
