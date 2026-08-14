package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestRunActiveSourcesDoesNotLetSlowSourceBlockFollowingSources(t *testing.T) {
	sources := []*domain.DataSource{{ID: "first"}, {ID: "second"}, {ID: "last"}}
	started := make(chan struct{}, len(sources))
	release := make(chan struct{})
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
				started <- struct{}{}
				if source.ID != "last" {
					<-release
				}
				return nil
			},
		)
	}()

	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("two workers did not reach the deterministic start barrier")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", maximum.Load())
	}
}

func TestClassifyActiveSourceRunError(t *testing.T) {
	tests := []struct {
		name               string
		runErr             error
		cancelContext      bool
		wantDegraded       bool
		wantInfrastructure bool
	}{
		{name: "success"},
		{
			name:         "external source failure remains degraded",
			runErr:       sourceDegradedError(errors.New("upstream unavailable")),
			wantDegraded: true,
		},
		{
			name:               "unclassified failure fails closed",
			runErr:             errors.New("unexpected repository failure"),
			wantInfrastructure: true,
		},
		{
			name: "infrastructure dominates mixed source failures",
			runErr: errors.Join(
				sourceDegradedError(errors.New("upstream unavailable")),
				fmt.Errorf("%w: transaction failed", ErrParserInfrastructure),
			),
			wantDegraded:       true,
			wantInfrastructure: true,
		},
		{
			name:               "cancellation is infrastructure state",
			runErr:             sourceDegradedError(context.Canceled),
			cancelContext:      true,
			wantDegraded:       true,
			wantInfrastructure: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if test.cancelContext {
				cancel()
			} else {
				defer cancel()
			}
			err := classifyActiveSourceRunError(ctx, test.runErr)
			if got := errors.Is(err, ErrSourceDegraded); got != test.wantDegraded {
				t.Fatalf("degraded classification = %t, want %t; err=%v", got, test.wantDegraded, err)
			}
			if got := errors.Is(err, ErrParserInfrastructure); got != test.wantInfrastructure {
				t.Fatalf("infrastructure classification = %t, want %t; err=%v", got, test.wantInfrastructure, err)
			}
		})
	}
}
