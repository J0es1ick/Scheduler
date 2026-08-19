package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	tele "gopkg.in/telebot.v3"
)

var ErrBotBusy = errors.New("bot handler capacity exhausted")

var ErrSenderBusy = errors.New("sender handler queue is full")

var handlerActive atomic.Int64
var handlerWaiting atomic.Int64
var handlerRejected atomic.Int64

const busyResponseText = "Бот занят обработкой запросов. Попробуйте ещё раз через несколько секунд."

var busyNotices = struct {
	sync.Mutex
	last map[int64]time.Time
}{last: make(map[int64]time.Time)}

func HandlerLoad() (active, waiting, rejected int64) {
	return handlerActive.Load(), handlerWaiting.Load(), handlerRejected.Load()
}

func RecoverPanics() tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("telegram handler panic: %v", recovered)
					slog.Error(
						"Telegram handler panic recovered",
						"user_id", senderID(c),
						"panic", recovered,
						"stack", string(debug.Stack()),
					)
				}
			}()
			return next(c)
		}
	}
}

func HandleError(err error, c tele.Context) {
	if errors.Is(err, ErrBotBusy) || errors.Is(err, ErrSenderBusy) {
		if c == nil {
			return
		}
		if c.Callback() != nil {
			_ = c.Respond(&tele.CallbackResponse{
				Text: busyResponseText,
			})
		} else if shouldSendBusyNotice(serializationID(c), time.Now()) {
			_ = c.Send(busyResponseText)
		}
		return
	}
	slog.Error("Telegram handler failed", "user_id", senderID(c), "err", err)
}

func LimitConcurrent(maxConcurrent int) tele.MiddlewareFunc {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	slots := make(chan struct{}, maxConcurrent)
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			select {
			case slots <- struct{}{}:
			default:
				handlerRejected.Add(1)
				return ErrBotBusy
			}
			handlerActive.Add(1)
			defer func() {
				handlerActive.Add(-1)
				<-slots
			}()
			return next(c)
		}
	}
}

func SerializeBySender(maxPending ...int) tele.MiddlewareFunc {
	queueLimit := 8
	if len(maxPending) > 0 {
		queueLimit = max(0, maxPending[0])
	}
	maxActive := queueLimit + 1
	var registryMu sync.Mutex
	locks := make(map[int64]*senderMutex)

	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			id := serializationID(c)
			if id == 0 {
				return next(c)
			}

			registryMu.Lock()
			lock := locks[id]
			if lock == nil {
				lock = &senderMutex{gate: make(chan struct{}, maxActive)}
				locks[id] = lock
			}
			lock.users++
			registryMu.Unlock()

			select {
			case lock.gate <- struct{}{}:
				handlerWaiting.Add(1)
			default:
				registryMu.Lock()
				lock.users--
				if lock.users == 0 {
					delete(locks, id)
				}
				registryMu.Unlock()
				handlerRejected.Add(1)
				return ErrSenderBusy
			}
			lock.mu.Lock()
			handlerWaiting.Add(-1)
			defer func() {
				lock.mu.Unlock()
				<-lock.gate
				registryMu.Lock()
				lock.users--
				if lock.users == 0 {
					delete(locks, id)
				}
				registryMu.Unlock()
			}()

			return next(c)
		}
	}
}

type senderMutex struct {
	mu    sync.Mutex
	gate  chan struct{}
	users int
}

type HandlerTracker struct {
	mu        sync.Mutex
	accepting bool
	active    sync.WaitGroup
}

func NewHandlerTracker() *HandlerTracker {
	return &HandlerTracker{accepting: true}
}

func (t *HandlerTracker) Middleware() tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			t.mu.Lock()
			if !t.accepting {
				t.mu.Unlock()
				return ErrBotBusy
			}
			t.active.Add(1)
			t.mu.Unlock()
			defer t.active.Done()
			return next(c)
		}
	}
}

func (t *HandlerTracker) StopAccepting() {
	t.mu.Lock()
	t.accepting = false
	t.mu.Unlock()
}

func (t *HandlerTracker) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		t.active.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func senderID(c tele.Context) int64 {
	if c == nil || c.Sender() == nil {
		return 0
	}
	return c.Sender().ID
}

func serializationID(c tele.Context) int64 {
	if c == nil {
		return 0
	}
	if chat := c.Chat(); chat != nil && chat.Type != tele.ChatPrivate {
		return chat.ID
	}
	return senderID(c)
}

func shouldSendBusyNotice(id int64, now time.Time) bool {
	if id == 0 {
		return false
	}
	busyNotices.Lock()
	defer busyNotices.Unlock()
	if previous := busyNotices.last[id]; now.Sub(previous) < 10*time.Second {
		return false
	}
	busyNotices.last[id] = now
	if len(busyNotices.last) > 1024 {
		for key, seenAt := range busyNotices.last {
			if now.Sub(seenAt) >= time.Minute {
				delete(busyNotices.last, key)
			}
		}
	}
	return true
}
