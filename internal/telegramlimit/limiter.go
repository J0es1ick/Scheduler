package telegramlimit

import (
	"context"
	"errors"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

const DefaultGlobalInterval = 40 * time.Millisecond
const DefaultRecipientInterval = 1100 * time.Millisecond

type Limiter struct {
	mu                sync.Mutex
	globalInterval    time.Duration
	recipientInterval time.Duration
	nextGlobal        time.Time
	nextRecipient     map[string]time.Time
	blockedUntil      time.Time
}

func New(globalInterval, recipientInterval time.Duration) *Limiter {
	return &Limiter{
		globalInterval:    max(0, globalInterval),
		recipientInterval: max(0, recipientInterval),
		nextRecipient:     make(map[string]time.Time),
	}
}

func (l *Limiter) Wait(ctx context.Context, recipient string) error {
	return l.wait(ctx, false, recipient)
}

func (l *Limiter) WaitGlobal(ctx context.Context) error {
	return l.wait(ctx, true, "")
}

func (l *Limiter) wait(ctx context.Context, includeGlobal bool, recipient string) error {
	for {
		l.mu.Lock()
		now := time.Now()
		next := l.blockedUntil
		if includeGlobal && l.nextGlobal.After(next) {
			next = l.nextGlobal
		}
		if recipientNext := l.nextRecipient[recipient]; recipientNext.After(next) {
			next = recipientNext
		}
		if !next.After(now) {
			if includeGlobal {
				l.nextGlobal = now.Add(l.globalInterval)
			}
			if recipient != "" {
				l.nextRecipient[recipient] = now.Add(l.recipientInterval)
			}
			l.pruneRecipients(now)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()

		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *Limiter) Observe(err error) (time.Duration, bool) {
	delay, limited := FloodRetryAfter(err)
	if !limited {
		return 0, false
	}
	l.block(delay)
	return delay, true
}

func FloodRetryAfter(err error) (time.Duration, bool) {
	var flood tele.FloodError
	if errors.As(err, &flood) {
		return time.Duration(max(0, flood.RetryAfter)+1) * time.Second, true
	}
	var apiErr *tele.Error
	if errors.As(err, &apiErr) && apiErr.Code == 429 {
		return time.Minute, true
	}
	return 0, false
}

func (l *Limiter) block(delay time.Duration) {
	l.mu.Lock()
	until := time.Now().Add(max(0, delay))
	if until.After(l.blockedUntil) {
		l.blockedUntil = until
	}
	l.mu.Unlock()
}

func (l *Limiter) pruneRecipients(now time.Time) {
	if len(l.nextRecipient) <= 10_000 {
		return
	}
	cutoff := now.Add(-time.Hour)
	for recipient, next := range l.nextRecipient {
		if next.Before(cutoff) {
			delete(l.nextRecipient, recipient)
		}
	}
}
