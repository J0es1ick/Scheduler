package worker

import (
	"context"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
)

const notificationClaimRenewInterval = 20 * time.Second
const notificationClaimQueryTimeout = 5 * time.Second

type notificationClaimGuard struct {
	ctx       context.Context
	cancel    context.CancelCauseFunc
	done      chan struct{}
	mu        sync.Mutex
	pending   map[string]struct{}
	token     string
	renew     func(context.Context, string, []string) error
	heartbeat func()
}

func startNotificationClaimGuard(
	parent context.Context,
	token string,
	ids []string,
	renew func(context.Context, string, []string) error,
	heartbeat func(),
	interval time.Duration,
) (*notificationClaimGuard, error) {
	if token == "" || len(ids) == 0 {
		return nil, repository.ErrNotificationClaimLost
	}
	ctx, cancel := context.WithCancelCause(parent)
	guard := &notificationClaimGuard{
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		pending: make(map[string]struct{}, len(ids)), token: token,
		renew: renew, heartbeat: heartbeat,
	}
	for _, id := range ids {
		guard.pending[id] = struct{}{}
	}
	if err := guard.renewPending(); err != nil {
		cancel(err)
		return nil, err
	}
	go func() {
		defer close(guard.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := guard.renewPending(); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	return guard, nil
}

func (g *notificationClaimGuard) renewPending() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := context.Cause(g.ctx); err != nil {
		return err
	}
	if len(g.pending) == 0 {
		return nil
	}
	ids := make([]string, 0, len(g.pending))
	for id := range g.pending {
		ids = append(ids, id)
	}
	ctx, cancel := context.WithTimeout(g.ctx, notificationClaimQueryTimeout)
	defer cancel()
	if err := g.renew(ctx, g.token, ids); err != nil {
		g.cancel(err)
		return err
	}
	if g.heartbeat != nil {
		g.heartbeat()
	}
	return nil
}

func (g *notificationClaimGuard) finish(id string, mark func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := context.Cause(g.ctx); err != nil {
		return err
	}
	if _, exists := g.pending[id]; !exists {
		return repository.ErrNotificationClaimLost
	}
	ctx, cancel := context.WithTimeout(g.ctx, notificationClaimQueryTimeout)
	defer cancel()
	if err := mark(ctx); err != nil {
		g.cancel(err)
		return err
	}
	delete(g.pending, id)
	if g.heartbeat != nil {
		g.heartbeat()
	}
	return nil
}

func (g *notificationClaimGuard) discard(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := context.Cause(g.ctx); err != nil {
		return err
	}
	if _, exists := g.pending[id]; !exists {
		return repository.ErrNotificationClaimLost
	}
	delete(g.pending, id)
	if g.heartbeat != nil {
		g.heartbeat()
	}
	return nil
}

func (g *notificationClaimGuard) stop() {
	g.cancel(context.Canceled)
	<-g.done
}
