package bot

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

type UserRestrictionsReader interface {
	Restrictions(context.Context, string) (domain.UserRestrictions, error)
}

func EnforceUserRestrictions(parent context.Context, reader UserRestrictionsReader, manager *state.Manager) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(current tele.Context) error {
			blocked, err := updateRestricted(parent, current, reader, manager)
			if err != nil || blocked {
				return err
			}
			return next(current)
		}
	}
}

func updateRestricted(parent context.Context, current tele.Context, reader UserRestrictionsReader, manager *state.Manager) (bool, error) {
	if reader == nil || current == nil || current.Sender() == nil {
		return false, nil
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	restrictions, err := reader.Restrictions(ctx, strconv.FormatInt(current.Sender().ID, 10))
	if err != nil {
		return false, err
	}
	if restrictions.BotBlocked {
		return true, nil
	}
	return restrictions.SupportBlocked && isSupportUpdate(current, manager), nil
}

func isSupportUpdate(current tele.Context, manager *state.Manager) bool {
	if callback := current.Callback(); callback != nil {
		name := callback.Unique
		if name == "" {
			name, _, _ = strings.Cut(strings.TrimPrefix(callback.Data, "\f"), "|")
		}
		switch name {
		case "select_hotline_type", "cancel_hotline_type", "cancel_hotline", "open_hotline", "schedule_feedback":
			return true
		}
		return false
	}
	text := strings.TrimSpace(current.Text())
	command := normalizeCommand(text)
	if command == "hotline" || command == "report" || strings.EqualFold(text, "Горячая линия") {
		return true
	}
	if command == "start" {
		fields := strings.Fields(text)
		message := current.Message()
		return (len(fields) > 1 && strings.HasPrefix(fields[1], "report_")) || (message != nil && strings.HasPrefix(message.Payload, "report_"))
	}
	if command != "" {
		return false
	}
	if _, exists := interruptingTextActions[strings.ToLower(text)]; exists {
		return false
	}
	if manager != nil && current.Message() != nil {
		flow := manager.Get(current.Sender().ID)
		return flow != nil && (flow.Step == "awaiting_hotline_submission" || flow.Step == "choosing_hotline_type")
	}
	return false
}
