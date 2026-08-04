package site

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicInfoCacheCombinesConcurrentLoads(t *testing.T) {
	cache := newPublicInfoCache()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	loader := func(context.Context) (*PublicInfo, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return &PublicInfo{Universities: 2, UniversityNames: []string{"one", "two"}}, nil
	}

	const readers = 8
	var wg sync.WaitGroup
	wg.Add(readers)
	errors := make(chan error, readers)
	for range readers {
		go func() {
			defer wg.Done()
			result, err := cache.Get(context.Background(), loader)
			if err == nil && result.Universities != 2 {
				t.Errorf("universities = %d, want 2", result.Universities)
			}
			errors <- err
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("cache get: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", calls.Load())
	}
}

func TestPublicInfoCacheExpires(t *testing.T) {
	cache := newPublicInfoCache()
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	var calls atomic.Int32
	loader := func(context.Context) (*PublicInfo, error) {
		return &PublicInfo{Users: int(calls.Add(1))}, nil
	}

	first, err := cache.Get(context.Background(), loader)
	if err != nil {
		t.Fatalf("first cache get: %v", err)
	}
	second, err := cache.Get(context.Background(), loader)
	if err != nil {
		t.Fatalf("second cache get: %v", err)
	}
	if first.Users != 1 || second.Users != 1 {
		t.Fatalf("cached users = %d and %d, want 1 and 1", first.Users, second.Users)
	}

	now = now.Add(publicInfoCacheTTL)
	third, err := cache.Get(context.Background(), loader)
	if err != nil {
		t.Fatalf("expired cache get: %v", err)
	}
	if third.Users != 2 {
		t.Fatalf("users after expiry = %d, want 2", third.Users)
	}
}
