package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestRunActiveSourcesDoesNotLetSlowSourceBlockFollowingSources(t *testing.T) {
	sources := []*domain.DataSource{{ID: "slow"}, {ID: "fast"}, {ID: "last"}}
	fastStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runActiveSources(ctx, sources, 2, time.Second,
			func(_ context.Context, source *domain.DataSource) error {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := maximum.Load()
					if current <= observed || maximum.CompareAndSwap(observed, current) {
						break
					}
				}
				if source.ID == "slow" {
					<-releaseSlow
				} else if source.ID == "fast" {
					close(fastStarted)
				}
				return nil
			},
		)
	}()

	select {
	case <-fastStarted:
		close(releaseSlow)
	case <-ctx.Done():
		t.Fatal("быстрый источник не запустился параллельно с медленным")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", maximum.Load())
	}
}
