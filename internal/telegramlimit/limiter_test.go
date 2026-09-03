package telegramlimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tele "gopkg.in/telebot.v3"
)

func TestLimiterSerializesConcurrentRequests(t *testing.T) {
	limiter := New(20*time.Millisecond, 0)
	started := time.Now()
	completed := make(chan time.Duration, 3)
	var group sync.WaitGroup
	for index := range 3 {
		group.Add(1)
		go func(recipient string) {
			defer group.Done()
			if err := limiter.WaitGlobal(context.Background()); err != nil {
				t.Errorf("wait: %v", err)
				return
			}
			completed <- time.Since(started)
		}(string(rune('a' + index)))
	}
	group.Wait()
	close(completed)
	latest := time.Duration(0)
	for elapsed := range completed {
		if elapsed > latest {
			latest = elapsed
		}
	}
	if latest < 30*time.Millisecond {
		t.Fatalf("concurrent requests completed after %v", latest)
	}
}

func TestLimiterUsesRecipientInterval(t *testing.T) {
	limiter := New(0, 25*time.Millisecond)
	if err := limiter.Wait(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := limiter.Wait(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond {
		t.Fatalf("recipient interval was %v", elapsed)
	}
}

func TestLimiterAppliesFloodBackoffToAllRecipients(t *testing.T) {
	limiter := New(0, 0)
	limiter.block(50 * time.Millisecond)
	started := time.Now()
	if err := limiter.Wait(context.Background(), "another-recipient"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 40*time.Millisecond {
		t.Fatalf("global backoff was %v", elapsed)
	}
}

func TestFloodRetryAfter(t *testing.T) {
	delay, limited := FloodRetryAfter(tele.FloodError{RetryAfter: 3})
	if !limited || delay != 4*time.Second {
		t.Fatalf("delay=%v limited=%t", delay, limited)
	}
	if delay, limited = FloodRetryAfter(errors.New("network")); limited || delay != 0 {
		t.Fatalf("ordinary error delay=%v limited=%t", delay, limited)
	}
	if delay, limited = FloodRetryAfter(tele.NewError(429, "Too Many Requests")); !limited || delay != time.Minute {
		t.Fatalf("plain 429 delay=%v limited=%t", delay, limited)
	}
}
