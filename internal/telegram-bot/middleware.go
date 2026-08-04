package bot

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	tele "gopkg.in/telebot.v3"
)

var ErrBotBusy = errors.New("bot handler capacity exhausted")

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

func LimitConcurrent(maxConcurrent int) tele.MiddlewareFunc {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	slots := make(chan struct{}, maxConcurrent)
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				return next(c)
			default:
				slog.Warn("Telegram update dropped because handler capacity is exhausted", "user_id", senderID(c))
				return ErrBotBusy
			}
		}
	}
}

func SerializeBySender() tele.MiddlewareFunc {
	var registryMu sync.Mutex
	locks := make(map[int64]*senderMutex)

	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			id := senderID(c)
			if id == 0 {
				return next(c)
			}

			registryMu.Lock()
			lock := locks[id]
			if lock == nil {
				lock = &senderMutex{}
				locks[id] = lock
			}
			lock.users++
			registryMu.Unlock()

			lock.mu.Lock()
			defer func() {
				lock.mu.Unlock()
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
	users int
}

func senderID(c tele.Context) int64 {
	if c == nil || c.Sender() == nil {
		return 0
	}
	return c.Sender().ID
}
