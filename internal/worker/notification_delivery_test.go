package worker

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

type notificationRepositoryStub struct {
	notificationDeliveryRepository
	items         []domain.NotificationDelivery
	outbox        []domain.BotOutboxDelivery
	renew         func(context.Context, string, []string) error
	active        func(context.Context, string, string) (bool, error)
	mark          func(context.Context, string, string) error
	scheduleLimit int
	outboxLimit   int
	consume       bool
	rateLimited   func(context.Context, string, string, time.Duration, error) error
	outboxLimited func(context.Context, string, string, time.Duration, error) error
}

func (r *notificationRepositoryStub) ClaimPending(_ context.Context, limit int) ([]domain.NotificationDelivery, error) {
	r.scheduleLimit = limit
	return r.items, nil
}

func (r *notificationRepositoryStub) ClaimBotOutbox(_ context.Context, limit int) ([]domain.BotOutboxDelivery, error) {
	r.outboxLimit = limit
	if r.consume {
		items := r.outbox[:min(limit, len(r.outbox))]
		r.outbox = r.outbox[len(items):]
		return items, nil
	}
	return r.outbox, nil
}

func TestNotificationWorkerDrainsLargeQueueInBoundedRounds(t *testing.T) {
	const total = 10_000
	repo := &notificationRepositoryStub{
		consume: true,
		renew:   func(context.Context, string, []string) error { return nil },
		active:  func(context.Context, string, string) (bool, error) { return true, nil },
	}
	for index := range total {
		repo.outbox = append(repo.outbox, domain.BotOutboxDelivery{ID: fmt.Sprint(index), UserID: fmt.Sprint(index + 1), Body: "notice", ClaimToken: "owner"})
	}
	marked := map[string]bool{}
	repo.mark = func(_ context.Context, id, _ string) error {
		if marked[id] {
			t.Errorf("duplicate delivery %s", id)
		}
		marked[id] = true
		return nil
	}
	var sent int
	w := &NotificationWorker{
		repository: repo, batchSize: 250, maxBatches: 4,
		waitSend: func(ctx context.Context, _ string) error { return ctx.Err() },
		limiter:  telegramlimit.New(0, 0),
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			sent++
			return &tele.Message{}, nil
		}),
	}
	for round := range 10 {
		if err := w.tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		if sent != (round+1)*1000 {
			t.Fatalf("round %d sent=%d", round, sent)
		}
	}
	if len(marked) != total || len(repo.outbox) != 0 {
		t.Fatalf("marked=%d pending=%d", len(marked), len(repo.outbox))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := w.tick(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func (r *notificationRepositoryStub) RenewDeliveryClaims(ctx context.Context, token string, ids []string) error {
	return r.renew(ctx, token, ids)
}

func (r *notificationRepositoryStub) RenewBotOutboxClaims(ctx context.Context, token string, ids []string) error {
	return r.renew(ctx, token, ids)
}

func (r *notificationRepositoryStub) IsDeliveryActive(ctx context.Context, id, token string) (bool, error) {
	return r.active(ctx, id, token)
}

func (r *notificationRepositoryStub) IsBotOutboxActive(ctx context.Context, id, token string) (bool, error) {
	return r.active(ctx, id, token)
}

func (r *notificationRepositoryStub) DeliveryDecision(
	ctx context.Context,
	id, token string,
) (repository.NotificationQueueDecision, error) {
	if r.active == nil {
		return repository.NotificationQueueReady, nil
	}
	active, err := r.active(ctx, id, token)
	if err != nil {
		return "", err
	}
	if !active {
		return repository.NotificationQueueCancel, nil
	}
	return repository.NotificationQueueReady, nil
}

func (r *notificationRepositoryStub) BotOutboxDecision(
	ctx context.Context,
	id, token string,
) (repository.NotificationQueueDecision, error) {
	return r.DeliveryDecision(ctx, id, token)
}

func (r *notificationRepositoryStub) MarkCancelled(ctx context.Context, id, token string) error {
	if r.mark == nil {
		return nil
	}
	return r.mark(ctx, id, token)
}

func (r *notificationRepositoryStub) MarkBotOutboxCancelled(ctx context.Context, id, token string) error {
	return r.MarkCancelled(ctx, id, token)
}

func (r *notificationRepositoryStub) MarkDeferred(ctx context.Context, id, token string) error {
	return r.MarkCancelled(ctx, id, token)
}

func (r *notificationRepositoryStub) MarkBotOutboxDeferred(ctx context.Context, id, token string) error {
	return r.MarkCancelled(ctx, id, token)
}

func (r *notificationRepositoryStub) MarkDelivered(ctx context.Context, id, token string) error {
	return r.mark(ctx, id, token)
}

func (r *notificationRepositoryStub) MarkBotOutboxDelivered(ctx context.Context, id, token string) error {
	return r.mark(ctx, id, token)
}

func (r *notificationRepositoryStub) MarkRateLimited(
	ctx context.Context,
	id, token string,
	retryAfter time.Duration,
	deliveryErr error,
) error {
	return r.rateLimited(ctx, id, token, retryAfter, deliveryErr)
}

func (r *notificationRepositoryStub) MarkBotOutboxRateLimited(
	ctx context.Context,
	id, token string,
	retryAfter time.Duration,
	deliveryErr error,
) error {
	return r.outboxLimited(ctx, id, token, retryAfter, deliveryErr)
}

type notificationSenderFunc func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error)

func (f notificationSenderFunc) Send(to tele.Recipient, what interface{}, opts ...interface{}) (*tele.Message, error) {
	return f(to, what, opts...)
}

func TestNotificationWorkerFencesBothQueuesAndBoundsBatch(t *testing.T) {
	marked := make(map[string]string)
	repo := &notificationRepositoryStub{
		items:  []domain.NotificationDelivery{{ID: "schedule", UserID: "101", ClaimToken: "schedule-owner", Summary: "updated"}},
		outbox: []domain.BotOutboxDelivery{{ID: "outbox", UserID: "102", ClaimToken: "outbox-owner", Body: "alert"}},
		renew: func(_ context.Context, token string, ids []string) error {
			if len(ids) != 1 || token != ids[0]+"-owner" {
				return repository.ErrNotificationClaimLost
			}
			return nil
		},
		active: func(_ context.Context, id, token string) (bool, error) {
			return token == id+"-owner", nil
		},
		mark: func(_ context.Context, id, token string) error {
			marked[id] = token
			return nil
		},
	}
	var sent int
	w := &NotificationWorker{
		repository: repo, limiter: telegramlimit.New(0, 0),
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			sent++
			return &tele.Message{}, nil
		}),
	}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sent != 2 || marked["schedule"] != "schedule-owner" || marked["outbox"] != "outbox-owner" {
		t.Fatalf("sent=%d marked=%v", sent, marked)
	}
	if repo.scheduleLimit != notificationBatchSize || repo.outboxLimit != notificationBatchSize {
		t.Fatalf("unbounded delivery batch: schedule=%d outbox=%d", repo.scheduleLimit, repo.outboxLimit)
	}
}

func TestNotificationWorkerContinuesBatchAfterClaimedCancellation(t *testing.T) {
	items := []domain.NotificationDelivery{
		{ID: "cancelled", UserID: "101", ClaimToken: "owner", Summary: "cancelled"},
		{ID: "ready", UserID: "101", ClaimToken: "owner", Summary: "ready"},
	}
	marked := make(map[string]bool)
	repo := &notificationRepositoryStub{
		active: func(_ context.Context, id, _ string) (bool, error) {
			return id != "cancelled", nil
		},
		mark: func(_ context.Context, id, _ string) error {
			marked[id] = true
			return nil
		},
	}
	var sent int
	w := &NotificationWorker{
		repository: repo,
		limiter:    telegramlimit.New(0, 0),
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			sent++
			return &tele.Message{}, nil
		}),
	}
	guard := &notificationClaimGuard{
		ctx: context.Background(), pending: map[string]struct{}{"cancelled": {}, "ready": {}},
	}
	if err := w.deliverScheduleBatch(context.Background(), items, guard); err != nil {
		t.Fatal(err)
	}
	if sent != 1 || !marked["cancelled"] || !marked["ready"] || len(guard.pending) != 0 {
		t.Fatalf("sent=%d marked=%v pending=%v", sent, marked, guard.pending)
	}
}

func TestNotificationWorkerDoesNotSendReclaimedDelivery(t *testing.T) {
	repo := &notificationRepositoryStub{
		outbox: []domain.BotOutboxDelivery{{ID: "outbox", UserID: "102", ClaimToken: "old-owner"}},
		renew:  func(context.Context, string, []string) error { return repository.ErrNotificationClaimLost },
	}
	var sent int
	w := &NotificationWorker{
		repository: repo, limiter: telegramlimit.New(0, 0),
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			sent++
			return &tele.Message{}, nil
		}),
	}
	if err := w.tick(context.Background()); !errors.Is(err, repository.ErrNotificationClaimLost) {
		t.Fatalf("stale claim error=%v", err)
	}
	if sent != 0 {
		t.Fatalf("sent %d messages without a valid claim", sent)
	}
}

func TestNotificationWorkerRenewsDuringSlowSendAndStopsAfterLeaseLoss(t *testing.T) {
	started := make(chan struct{})
	revoked := make(chan struct{})
	var renewals atomic.Int32
	var sent, marked atomic.Int32
	repo := &notificationRepositoryStub{
		outbox: []domain.BotOutboxDelivery{
			{ID: "first", UserID: "101", ClaimToken: "owner"},
			{ID: "second", UserID: "102", ClaimToken: "owner"},
		},
		renew: func(ctx context.Context, _ string, _ []string) error {
			if renewals.Add(1) == 1 {
				return nil
			}
			select {
			case <-started:
			case <-ctx.Done():
				return ctx.Err()
			}
			close(revoked)
			return repository.ErrNotificationClaimLost
		},
		active: func(context.Context, string, string) (bool, error) { return true, nil },
		mark: func(context.Context, string, string) error {
			marked.Add(1)
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	w := &NotificationWorker{
		repository: repo, limiter: telegramlimit.New(0, 0), claimRenewInterval: 5 * time.Millisecond,
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			if sent.Add(1) == 1 {
				close(started)
			}
			select {
			case <-revoked:
			case <-ctx.Done():
			}
			return &tele.Message{}, nil
		}),
	}
	err := w.tick(ctx)
	if !errors.Is(err, repository.ErrNotificationClaimLost) || sent.Load() != 1 || marked.Load() != 0 {
		t.Fatalf("lease loss: err=%v sends=%d marks=%d", err, sent.Load(), marked.Load())
	}
}

func TestNotificationWorkerRechecksEligibilityAfterRateLimitWait(t *testing.T) {
	var activeChecked atomic.Bool
	repo := &notificationRepositoryStub{
		outbox: []domain.BotOutboxDelivery{{ID: "outbox", UserID: "102", ClaimToken: "owner"}},
		renew:  func(context.Context, string, []string) error { return nil },
		active: func(context.Context, string, string) (bool, error) {
			activeChecked.Store(true)
			return true, nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	limiter := telegramlimit.New(0, time.Hour)
	if err := limiter.Wait(context.Background(), "102"); err != nil {
		t.Fatal(err)
	}
	w := &NotificationWorker{
		repository: repo, limiter: limiter,
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			t.Error("sent after cancellation")
			return &tele.Message{}, nil
		}),
	}
	if err := w.tick(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if activeChecked.Load() {
		t.Fatal("eligibility checked before the rate-limit wait completed")
	}
}

type floodErrorStub struct {
	retryAfter int
}

func (e floodErrorStub) Error() string {
	return "flood control"
}

func (e floodErrorStub) As(target any) bool {
	flood, ok := target.(*tele.FloodError)
	if ok {
		*flood = tele.FloodError{RetryAfter: e.retryAfter}
	}
	return ok
}

func TestNotificationWorkerRecordsFloodWithoutConsumingAttempt(t *testing.T) {
	var scheduleDelay, outboxDelay time.Duration
	repo := &notificationRepositoryStub{
		rateLimited: func(_ context.Context, id, token string, retryAfter time.Duration, err error) error {
			if id != "schedule" || token != "schedule-token" || err == nil {
				t.Fatalf("unexpected schedule rate limit: id=%s token=%s err=%v", id, token, err)
			}
			scheduleDelay = retryAfter
			return nil
		},
		outboxLimited: func(_ context.Context, id, token string, retryAfter time.Duration, err error) error {
			if id != "outbox" || token != "outbox-token" || err == nil {
				t.Fatalf("unexpected outbox rate limit: id=%s token=%s err=%v", id, token, err)
			}
			outboxDelay = retryAfter
			return nil
		},
	}
	w := &NotificationWorker{
		repository: repo,
		limiter:    telegramlimit.New(0, 0),
	}
	deliveryErr := floodErrorStub{retryAfter: 2}
	if err := w.recordFailure(context.Background(), domain.NotificationDelivery{
		ID: "schedule", ClaimToken: "schedule-token", Attempts: 5,
	}, deliveryErr); err != nil {
		t.Fatal(err)
	}
	if err := w.recordBotOutboxFailure(context.Background(), domain.BotOutboxDelivery{
		ID: "outbox", ClaimToken: "outbox-token", Attempts: 5,
	}, deliveryErr); err != nil {
		t.Fatal(err)
	}
	if scheduleDelay != 3*time.Second || outboxDelay != 3*time.Second {
		t.Fatalf("schedule delay=%v outbox delay=%v", scheduleDelay, outboxDelay)
	}
}

func TestNotificationWorkerRequeuesRemainingScheduleClaimsAfterFlood(t *testing.T) {
	items := []domain.NotificationDelivery{
		{ID: "first", UserID: "101", ClaimToken: "owner", Summary: "first"},
		{ID: "second", UserID: "102", ClaimToken: "owner", Summary: "second"},
	}
	marked := make(map[string]time.Duration)
	repo := &notificationRepositoryStub{
		active: func(context.Context, string, string) (bool, error) { return true, nil },
		rateLimited: func(_ context.Context, id, _ string, retryAfter time.Duration, _ error) error {
			marked[id] = retryAfter
			return nil
		},
	}
	var sent int
	w := &NotificationWorker{
		repository: repo,
		limiter:    telegramlimit.New(0, 0),
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			sent++
			return nil, floodErrorStub{}
		}),
	}
	guard := &notificationClaimGuard{
		ctx: context.Background(), pending: map[string]struct{}{"first": {}, "second": {}},
	}
	if err := w.deliverScheduleBatch(context.Background(), items, guard); !errors.Is(err, errTelegramRateLimited) {
		t.Fatalf("rate-limit result=%v", err)
	}
	if sent != 1 || len(marked) != 2 || marked["first"] != time.Second || marked["second"] != time.Second {
		t.Fatalf("sent=%d marked=%v", sent, marked)
	}
	if len(guard.pending) != 0 {
		t.Fatalf("claims remain held: %v", guard.pending)
	}
}

func TestNotificationWorkerRequeuesRemainingOutboxClaimsAfterFlood(t *testing.T) {
	items := []domain.BotOutboxDelivery{
		{ID: "first", UserID: "101", ClaimToken: "owner", Body: "first"},
		{ID: "second", UserID: "102", ClaimToken: "owner", Body: "second"},
	}
	marked := make(map[string]time.Duration)
	repo := &notificationRepositoryStub{
		outbox: items,
		renew:  func(context.Context, string, []string) error { return nil },
		active: func(context.Context, string, string) (bool, error) { return true, nil },
		outboxLimited: func(_ context.Context, id, _ string, retryAfter time.Duration, _ error) error {
			marked[id] = retryAfter
			return nil
		},
	}
	var sent int
	w := &NotificationWorker{
		repository: repo,
		limiter:    telegramlimit.New(0, 0),
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			sent++
			return nil, floodErrorStub{}
		}),
	}
	if _, err := w.drainBatch(context.Background(), nil, 10); err != nil {
		t.Fatal(err)
	}
	if sent != 1 || len(marked) != 2 || marked["first"] != time.Second || marked["second"] != time.Second {
		t.Fatalf("sent=%d marked=%v", sent, marked)
	}
}
