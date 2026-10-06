package handlers

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) mainMenu(c tele.Context) *tele.ReplyMarkup {
	if h.StateManager == nil || c.Sender() == nil {
		return keyboards.MainMenu()
	}
	role := domain.RoleStudent
	if state := h.StateManager.Get(c.Sender().ID); state != nil {
		role = state.Role
	}
	return keyboards.MainMenu(role)
}

func (h *Handler) HandleRole(c tele.Context) error {
	state := &dto.UserState{Step: "choosing_role", FlowNonce: newFlowNonce()}
	h.StateManager.Set(c.Sender().ID, state)
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Студент", "select_role", "student", state.FlowNonce), menu.Data("Преподаватель", "select_role", "teacher", state.FlowNonce)), menu.Row(menu.Data("Отмена", "cancel_role", state.FlowNonce)))
	if c.Callback() != nil {
		_ = c.Respond()
	}
	return c.Send("Выберите роль:", menu)
}

func (h *Handler) HandleSelectRole(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) != 2 || !validFlow(state, "choosing_role", args[1]) {
		return respondStaleCallback(c)
	}
	role := domain.UserRole(args[0])
	if role != domain.RoleStudent && role != domain.RoleTeacher {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil || user == nil {
		return c.Send("Не удалось загрузить профиль. Используйте /start.")
	}
	if (role == domain.RoleTeacher && user.TeacherID != "") || (role == domain.RoleStudent && user.DefaultGroupID != "") {
		if err = h.ProfileService.SetRole(ctx, user.ID, role); err != nil {
			return c.Send("Не удалось сменить роль.")
		}
		return h.finishProfileSetup(c)
	}
	state.Role = role
	return h.chooseProfileUniversity(ctx, c, state)
}

func (h *Handler) chooseProfileUniversity(ctx context.Context, c tele.Context, state *dto.UserState) error {
	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		return c.Send("Не удалось загрузить список вузов.")
	}
	if len(universities) == 0 {
		return c.Send("В системе пока нет вузов с актуальным расписанием.")
	}
	state.Step = "choosing_university"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	return c.Send("Выберите вуз:", keyboards.UniversitySelector(universities, state.FlowNonce))
}

func (h *Handler) HandleCancelRole(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) != 1 || !validFlow(h.StateManager.Get(c.Sender().ID), "choosing_role", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	return h.HandleMenu(c)
}

func (h *Handler) confirmTeacher(c tele.Context, state *dto.UserState, name string) error {
	state.TeacherName = name
	state.Step = "confirming_teacher"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Подтвердить ФИО", "confirm_teacher", "save", state.FlowNonce)), menu.Row(menu.Data("Выбрать другое ФИО", "confirm_teacher", "back", state.FlowNonce)), menu.Row(menu.Data("Отмена", "cancel_profile_binding", state.FlowNonce)))
	return c.Send(fmt.Sprintf("Ваше расписание: %s · %s\n\nПодтвердите ФИО.", state.University, name), menu)
}

func (h *Handler) HandleConfirmTeacher(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) != 2 || !validFlow(state, "confirming_teacher", args[1]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	if args[0] == "back" {
		return h.promptOwnTeacher(c, state)
	}
	if args[0] != "save" {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	names, err := h.ScheduleService.FindTeachers(ctx, state.UniversityID, "")
	if err != nil {
		return c.Send("Не удалось проверить ФИО. Попробуйте позже.")
	}
	found := false
	for _, name := range names {
		if searchtext.TokenKey(name) == searchtext.TokenKey(state.TeacherName) {
			state.TeacherName = name
			found = true
			break
		}
	}
	if !found {
		return h.promptOwnTeacher(c, state)
	}
	if err = h.ProfileService.SetTeacher(ctx, fmt.Sprint(c.Sender().ID), state.UniversityID, state.TeacherName); err != nil {
		return c.Send("Не удалось сохранить ФИО. Попробуйте ещё раз.")
	}
	return h.finishProfileSetup(c)
}

func (h *Handler) promptOwnTeacher(c tele.Context, state *dto.UserState) error {
	state.Role = domain.RoleTeacher
	state.Step = "awaiting_own_teacher"
	state.TeacherSearchOrigin = "profile"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Другой вуз", "back_profile_university", state.FlowNonce)), menu.Row(menu.Data("Отмена", "cancel_profile_binding", state.FlowNonce)))
	return c.Send("Введите свою фамилию или ФИО в выбранном вузе. "+teacherSearchPrompt(), menu)
}

func (h *Handler) HandleBackProfileUniversity(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) != 1 || !validFlow(state, "awaiting_own_teacher", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	return h.chooseProfileUniversity(ctx, c, state)
}

func (h *Handler) teacherProfile(c tele.Context) (bool, error) {
	if h.UserService == nil {
		return false, nil
	}
	ctx, cancel := reqCtx()
	defer cancel()
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	return user != nil && user.Role == domain.RoleTeacher, err
}

func (h *Handler) showTeacherSettings(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	state, user, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil || state == nil || user.Role != domain.RoleTeacher {
		return c.Send("Не удалось загрузить ваше расписание. Используйте /start.")
	}
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Заменить ФИО", "change_teacher"), menu.Data("Сменить вуз", "change_profile_university")),
		menu.Row(menu.Data("Визуальная таблица", "teacher_view", "visual"), menu.Data("Компактный текст", "teacher_view", "compact")),
		menu.Row(menu.Data("Обновления сервиса", "updates_settings")), menu.Row(menu.Data("Ежедневное расписание", "daily_settings")), menu.Row(menu.Data("Напоминания перед занятиями", "show_reminder_settings", "0")),
		menu.Row(menu.Data("Уведомления об изменениях", "toggle_notifications")), menu.Row(menu.Data("Сменить роль", "role_settings")), menu.Row(menu.Data("Главное меню", "open_main_menu")))
	status := ""
	if !state.GroupActive {
		status = "\nРасписание временно недоступно; привязка сохранена."
	}
	return editOrSend(c, fmt.Sprintf("Моё расписание: %s · %s%s\nУведомления об изменениях: %s\nФормат: %s", state.University, state.TeacherName, status, onOff(user.NotificationsEnabled), chatViewLabel(user.TeacherScheduleView)), menu)
}

func onOff(value bool) string {
	if value {
		return "включены"
	}
	return "выключены"
}

func (h *Handler) HandleChangeTeacher(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	state, err := h.readyState(ctx, c.Sender().ID)
	if err != nil || state == nil || state.Role != domain.RoleTeacher {
		return respondStaleCallback(c)
	}
	if c.Callback() != nil {
		_ = c.Respond()
	}
	return h.promptOwnTeacher(c, state)
}

func (h *Handler) HandleTeacherView(c tele.Context) error {
	args := callbackArguments(c)
	teacher, err := h.teacherProfile(c)
	if err != nil || !teacher || len(args) != 1 {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	if err = h.ProfileService.SetTeacherView(ctx, fmt.Sprint(c.Sender().ID), domain.ScheduleViewFormat(args[0])); err != nil {
		return c.Send("Не удалось изменить формат.")
	}
	_ = c.Respond()
	return h.showTeacherSettings(c)
}

func (h *Handler) HandleCancelProfileBinding(c tele.Context) error {
	args := callbackArguments(c)
	current := h.StateManager.Get(c.Sender().ID)
	if len(args) != 1 || current == nil || (current.Step != "confirming_teacher" && current.Step != "awaiting_own_teacher") || !validFlow(current, current.Step, args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	restored, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль. Используйте /start.")
	}
	if restored == nil {
		return h.HandleRole(c)
	}
	return h.HandleSettings(c)
}
