package bot

import (
	"strings"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

var interruptingTextActions = map[string]struct{}{
	"сегодня":         {},
	"завтра":          {},
	"неделя":          {},
	"две недели":      {},
	"выбрать дату":    {},
	"по дню недели":   {},
	"поиск":           {},
	"мои группы":      {},
	"ещё":             {},
	"на сегодня":      {},
	"на завтра":       {},
	"на неделю":       {},
	"сменить группу":  {},
	"добавить группу": {},
	"настройки":       {},
	"горячая линия":   {},
}

func CommandStatePolicy(manager *state.Manager, preservedCommands ...string) tele.MiddlewareFunc {
	preserved := make(map[string]struct{}, len(preservedCommands))
	for _, command := range preservedCommands {
		normalized := normalizeCommand(command)
		if normalized == "" {
			normalized = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(command), "/"))
		}
		if normalized != "" {
			preserved[normalized] = struct{}{}
		}
	}
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(current tele.Context) error {
			if manager != nil && shouldResetCommandState(current, preserved) {
				if manager.Get(current.Sender().ID) != nil {
					manager.Delete(current.Sender().ID)
				}
			}
			return next(current)
		}
	}
}

func shouldResetCommandState(current tele.Context, preserved map[string]struct{}) bool {
	if current == nil || current.Sender() == nil || current.Chat() == nil || current.Chat().Type != tele.ChatPrivate {
		return false
	}
	text := strings.TrimSpace(current.Text())
	if _, exists := interruptingTextActions[strings.ToLower(text)]; exists {
		return true
	}
	command := normalizeCommand(text)
	if command == "" {
		return false
	}
	_, keep := preserved[command]
	return !keep
}

func normalizeCommand(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return ""
	}
	command := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	if index := strings.IndexByte(command, '@'); index >= 0 {
		command = command[:index]
	}
	return command
}
