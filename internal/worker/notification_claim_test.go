package worker

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
)

func TestNotificationClaimRenewsOnlyPendingDeliveries(t *testing.T) {
	renewals := make(chan []string, 16)
	var heartbeats atomic.Int32
	guard, err := startNotificationClaimGuard(context.Background(), "owner", []string{"first", "second"},
		func(_ context.Context, token string, ids []string) error {
			if token != "owner" {
				return repository.ErrNotificationClaimLost
			}
			sort.Strings(ids)
			renewals <- ids
			return nil
		}, func() { heartbeats.Add(1) }, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.stop()
	if ids := <-renewals; !reflect.DeepEqual(ids, []string{"first", "second"}) {
		t.Fatalf("initial renewal=%v", ids)
	}
	if err = guard.finish("first", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
waitForPending:
	for {
		select {
		case ids := <-renewals:
			if reflect.DeepEqual(ids, []string{"second"}) {
				break waitForPending
			}
		case <-deadline.C:
			t.Fatal("pending claim was not renewed")
		}
	}
	if err = guard.finish("second", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if heartbeats.Load() < 2 {
		t.Fatal("delivery progress did not update readiness heartbeat")
	}
}

func TestNotificationClaimLossCancelsDelivery(t *testing.T) {
	var calls atomic.Int32
	guard, err := startNotificationClaimGuard(context.Background(), "owner", []string{"delivery"},
		func(context.Context, string, []string) error {
			if calls.Add(1) > 1 {
				return repository.ErrNotificationClaimLost
			}
			return nil
		}, nil, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.stop()
	select {
	case <-guard.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lost claim did not cancel the batch")
	}
	called := false
	err = guard.finish("delivery", func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, repository.ErrNotificationClaimLost) || called {
		t.Fatalf("stale claim was finalized: called=%t err=%v", called, err)
	}
}

func TestNotificationClaimFinalizationFailureStopsBatch(t *testing.T) {
	guard, err := startNotificationClaimGuard(context.Background(), "owner", []string{"first", "second"},
		func(context.Context, string, []string) error { return nil }, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.stop()
	failure := errors.New("database unavailable")
	if err = guard.finish("first", func(context.Context) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !errors.Is(context.Cause(guard.ctx), failure) {
		t.Fatalf("batch continued after finalization error: %v", context.Cause(guard.ctx))
	}
}

func TestNotificationClaimShutdownWaitsForRenewal(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	guard, err := startNotificationClaimGuard(context.Background(), "owner", []string{"delivery"},
		func(ctx context.Context, _ string, _ []string) error {
			if calls.Add(1) == 1 {
				return nil
			}
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}, nil, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		guard.stop()
		t.Fatal("renewal did not start")
	}
	guard.stop()
	select {
	case <-guard.done:
	default:
		t.Fatal("shutdown left a renewal running")
	}
}
