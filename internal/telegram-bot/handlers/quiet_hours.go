package handlers

import (
	"fmt"
	"regexp"
	"strings"

	tele "gopkg.in/telebot.v3"
)

var quietHoursPattern = regexp.MustCompile(`^((?:[01][0-9]|2[0-3]):[0-5][0-9])-((?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

func (h *Handler) HandleQuietHours(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil || user == nil {
		return c.Send("Не удалось загрузить профиль. Используйте /start.")
	}
	input := strings.ToLower(strings.TrimSpace(strings.Join(c.Args(), "")))
	if input != "" {
		enabled, start, end, parseErr := parseQuietHours(input, user.QuietHoursStart, user.QuietHoursEnd)
		if parseErr != nil {
			return c.Send("Формат: /quiet_hours 22:00-07:00 или /quiet_hours off")
		}
		if err = h.UserService.SetQuietHours(ctx, userID, enabled, start, end); err != nil {
			return c.Send("Не удалось сохранить тихие часы.")
		}
		user.QuietHoursEnabled, user.QuietHoursStart, user.QuietHoursEnd = enabled, start, end
	}
	if !user.QuietHoursEnabled {
		return c.Send("Тихие часы выключены.\n\nВключить: /quiet_hours 22:00-07:00")
	}
	return c.Send(fmt.Sprintf("Тихие часы: %s–%s. Уведомления дождутся их окончания.\n\nВыключить: /quiet_hours off", user.QuietHoursStart, user.QuietHoursEnd))
}

func parseQuietHours(input, currentStart, currentEnd string) (bool, string, string, error) {
	if input == "off" || input == "выкл" || input == "0" {
		if currentStart == "" {
			currentStart = "22:00"
		}
		if currentEnd == "" {
			currentEnd = "07:00"
		}
		return false, currentStart, currentEnd, nil
	}
	match := quietHoursPattern.FindStringSubmatch(input)
	if len(match) != 3 || match[1] == match[2] {
		return false, "", "", fmt.Errorf("invalid quiet hours")
	}
	return true, match[1], match[2], nil
}
