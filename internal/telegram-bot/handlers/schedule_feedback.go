package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	tele "gopkg.in/telebot.v3"
)

func scheduleReportLink(c tele.Context) string {
	if isGroupChat(c) {
		return ""
	}
	return "\nСообщить об ошибке: /report"
}

func parseReportPayload(payload string) (string, time.Time, int, error) {
	parts := strings.Split(payload, "_")
	if len(parts) != 4 || parts[0] != "report" {
		return "", time.Time{}, 0, fmt.Errorf("invalid report link")
	}
	reference := parts[1]
	hexPart := reference
	if len(reference) == 17 && (reference[0] == 't' || reference[0] == 'p') {
		hexPart = reference[1:]
	}
	if len(hexPart) != 16 {
		return "", time.Time{}, 0, fmt.Errorf("invalid schedule reference")
	}
	if _, err := strconv.ParseUint(hexPart, 16, 64); err != nil {
		return "", time.Time{}, 0, err
	}
	date, err := time.Parse("20060102", parts[2])
	if err != nil {
		return "", time.Time{}, 0, err
	}
	subgroup, err := strconv.Atoi(parts[3])
	if err != nil || subgroup < 0 || subgroup > 100 {
		return "", time.Time{}, 0, fmt.Errorf("invalid subgroup")
	}
	return reference, date, subgroup, nil
}

func (h *Handler) HandleReport(c tele.Context) error {
	return h.openScheduleReport(c, nil, time.Time{})
}

func (h *Handler) handleReportLink(c tele.Context, payload string) error {
	reference, date, subgroup, err := parseReportPayload(payload)
	if err != nil {
		return c.Send("Ссылка на расписание повреждена. Опишите ошибку через /report.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	target, err := h.downloadTarget(ctx, c, reference)
	if err != nil || target == nil {
		return c.Send("Это расписание больше недоступно. Опишите ошибку через /report и укажите группу или преподавателя.")
	}
	target.Subgroup = subgroup
	return h.openScheduleReport(c, target, date)
}

func (h *Handler) openScheduleReport(c tele.Context, target *scheduleTarget, date time.Time) error {
	ctx, cancel := reqCtx()
	defer cancel()
	if _, err := h.UserService.RegisterOrGetUser(ctx, fmt.Sprint(c.Sender().ID), c.Sender().Username); err != nil {
		return c.Send("Не удалось открыть обращение. Попробуйте позже.")
	}
	context := ""
	if target != nil {
		subgroup := "все"
		if target.Subgroup > 0 {
			subgroup = strconv.Itoa(target.Subgroup)
		}
		context = fmt.Sprintf("Вуз: %s\nРасписание: %s\nДата: %s\nПодгруппа: %s\n\n", target.University, target.displayName(), date.Format("02.01.2006"), subgroup)
	}
	current := &dto.UserState{Step: "awaiting_hotline_submission", HotlineType: domain.SupportRequestUpdateExisting, HotlineContext: context, FlowNonce: newFlowNonce()}
	h.StateManager.Set(c.Sender().ID, current)
	prompt := "Опишите ошибку одним сообщением. Этот контекст будет добавлен к вашему сообщению."
	if target == nil {
		prompt = "Укажите вуз, группу или преподавателя, дату и опишите ошибку одним сообщением."
	}
	return c.Send("Сообщить об ошибке\n\n"+context+prompt+"\n\nОбращение отправится только после отправки вами текста.", hotlineCancelButton(current.FlowNonce))
}
