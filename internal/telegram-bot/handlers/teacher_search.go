package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) beginTeacherSearch(
	ctx context.Context,
	c tele.Context,
	state *dto.UserState,
	query string,
	origin string,
) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return c.Send("Введите фамилию, имя с отчеством или фамилию с инициалами.")
	}
	names, err := h.ScheduleService.FindTeachers(ctx, state.UniversityID, query)
	if err != nil {
		return c.Send("Не удалось выполнить поиск преподавателя. Попробуйте позже.")
	}
	state.SearchType = dto.SearchTypeTeacher
	state.SearchQuery = query
	state.TeacherSearchOrigin = origin
	state.TeacherCandidates = append([]string(nil), names...)
	state.FlowNonce = newFlowNonce()
	if len(names) == 0 {
		state.Step = "awaiting_search_query"
		h.StateManager.Set(c.Sender().ID, state)
		return c.Send(
			"В расписании основного вуза такой преподаватель не найден. Проверьте фамилию или введите другой вариант.",
			keyboards.CancelButton(state.FlowNonce),
		)
	}
	if len(names) > 1 {
		state.Step = "choosing_teacher"
		h.StateManager.Set(c.Sender().ID, state)
		return c.Send(
			fmt.Sprintf("Найдено несколько преподавателей по запросу «%s». Выберите нужного:", query),
			keyboards.TeacherMatches(names, state.FlowNonce),
		)
	}
	return h.openTeacherSchedule(ctx, c, state, names[0])
}

func (h *Handler) HandleTeacherSelect(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) < 2 || !validFlow(state, "choosing_teacher", args[1]) {
		return respondStaleCallback(c)
	}
	index, err := strconv.Atoi(args[0])
	if err != nil || index < 0 || index >= len(state.TeacherCandidates) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	return h.openTeacherSchedule(ctx, c, state, state.TeacherCandidates[index])
}

func (h *Handler) HandleCancelTeacherSelection(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) < 1 || !validFlow(state, "choosing_teacher", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	if state.TeacherSearchOrigin == "search" {
		state.Step = "awaiting_search_query"
		state.TeacherCandidates = nil
		state.FlowNonce = newFlowNonce()
		h.StateManager.Set(c.Sender().ID, state)
		return editOrSend(c, teacherSearchPrompt(), keyboards.CancelButton(state.FlowNonce))
	}
	ctx, cancel := reqCtx()
	defer cancel()
	restored, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте позже.")
	}
	if restored == nil {
		h.StateManager.Delete(c.Sender().ID)
	} else {
		h.StateManager.Set(c.Sender().ID, restored)
	}
	if err = retireInlineMessage(c, "Поиск отменён."); err != nil {
		return err
	}
	return h.HandleMenu(c)
}

func (h *Handler) HandleSearchTeacherAgain(c tele.Context) error {
	if isGroupChat(c) {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	state, err := h.readyState(ctx, c.Sender().ID)
	if err != nil || state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "Не удалось загрузить профиль", ShowAlert: true})
	}
	state.Step = "awaiting_search_query"
	state.SearchType = dto.SearchTypeTeacher
	state.SearchQuery = ""
	state.TeacherCandidates = nil
	state.TeacherSearchOrigin = "search"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	_ = c.Respond()
	if h.hasTrackedScheduleMessages(c) {
		if err = h.deleteTrackedScheduleMessages(c); err != nil {
			return err
		}
		return c.Send(teacherSearchPrompt(), keyboards.CancelButton(state.FlowNonce))
	}
	return editOrSend(c, teacherSearchPrompt(), keyboards.CancelButton(state.FlowNonce))
}

func (h *Handler) openTeacherSchedule(
	ctx context.Context,
	c tele.Context,
	state *dto.UserState,
	teacher string,
) error {
	target, err := h.teacherScheduleTarget(ctx, c, state.UniversityID, teacher)
	if err != nil {
		return c.Send("Не удалось загрузить настройки расписания. Попробуйте позже.")
	}
	state.Step = "done"
	state.SearchQuery = teacher
	state.TeacherCandidates = nil
	state.TeacherSearchOrigin = ""
	h.StateManager.Set(c.Sender().ID, state)
	return h.sendTargetWeek(ctx, c, target, h.targetNow(ctx, target), 7)
}

func (h *Handler) teacherScheduleTarget(
	ctx context.Context,
	c tele.Context,
	universityID string,
	teacher string,
) (*scheduleTarget, error) {
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}
	university, err := h.UniversityService.GetByID(ctx, universityID)
	if err != nil {
		return nil, err
	}
	if university == nil || !university.IsActive {
		return nil, fmt.Errorf("university not found")
	}
	return &scheduleTarget{
		UniversityID: university.ID,
		University:   university.Name,
		GroupName:    teacher,
		TeacherName:  teacher,
		ViewFormat:   searchScheduleView(user.SearchScheduleView),
	}, nil
}

func teacherSearchPrompt() string {
	return "Введите преподавателя: можно указать только фамилию, часть фамилии, имя и отчество или фамилию с инициалами."
}
