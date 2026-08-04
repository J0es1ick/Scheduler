package admin

import (
	"sync"
	"time"
)

const (
	loginAttemptWindow  = 15 * time.Minute
	loginBlockDuration  = 15 * time.Minute
	maxLoginFailures    = 5
	maxTrackedLoginKeys = 10_000
)

type loginAttempt struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

type loginGuard struct {
	mu          sync.Mutex
	attempts    map[string]loginAttempt
	maxFailures int
}

func newLoginGuard(maxFailures ...int) *loginGuard {
	limit := maxLoginFailures
	if len(maxFailures) > 0 && maxFailures[0] > 0 {
		limit = maxFailures[0]
	}
	return &loginGuard{attempts: make(map[string]loginAttempt), maxFailures: limit}
}

func (g *loginGuard) allowed(key string, now time.Time) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	attempt, ok := g.attempts[key]
	if !ok {
		return true, 0
	}
	if attempt.blockedUntil.After(now) {
		return false, attempt.blockedUntil.Sub(now)
	}
	if now.Sub(attempt.windowStart) >= loginAttemptWindow {
		delete(g.attempts, key)
	}
	return true, 0
}

func (g *loginGuard) failed(key string, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.attempts) >= maxTrackedLoginKeys {
		for candidate, current := range g.attempts {
			if !current.blockedUntil.After(now) && now.Sub(current.windowStart) >= loginAttemptWindow {
				delete(g.attempts, candidate)
			}
		}
	}

	attempt := g.attempts[key]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= loginAttemptWindow {
		attempt = loginAttempt{windowStart: now}
	}
	attempt.failures++
	if attempt.failures >= g.maxFailures {
		attempt.blockedUntil = now.Add(loginBlockDuration)
	}
	g.attempts[key] = attempt
}

func (g *loginGuard) succeeded(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, key)
}
