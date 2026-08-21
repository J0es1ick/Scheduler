package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

type scheduleTarget struct {
	GroupID      string
	GroupName    string
	UniversityID string
	University   string
	ViewFormat   domain.ScheduleViewFormat
}

func isGroupChat(c tele.Context) bool {
	chat := c.Chat()
	return chat != nil &&
		(chat.Type == tele.ChatGroup || chat.Type == tele.ChatSuperGroup)
}

func (h *Handler) PrivateOnly(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		if isGroupChat(c) {
			return c.Send(
				"Эта команда работает только в личном чате с ботом. " +
					"Команды группового чата: /help",
			)
		}
		return next(c)
	}
}

func (h *Handler) HandleChatSettings(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send("Эта команда предназначена для групповых чатов.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	profile, err := h.ChatProfileService.Get(ctx, strconv.FormatInt(c.Chat().ID, 10))
	if err != nil {
		slog.Error("load chat schedule profile failed", "chat_id", c.Chat().ID, "err", err)
		return c.Send("Не удалось загрузить настройки этого чата.")
	}
	isAdmin, adminErr := chatAdministrator(c)
	if adminErr != nil {
		slog.Debug("check chat administrator for settings panel failed", "chat_id", c.Chat().ID, "err", adminErr)
	}
	if profile == nil {
		text := "Группа расписания для этого чата ещё не выбрана.\n\n" +
			"Администратор чата может настроить её командой:\n" +
			"/set_chat_group isuct 3/147\n" +
			"или /set_chat_group ispu 1-40"
		if c.Callback() != nil {
			return editOrSend(c, text, keyboards.EmptyChatSettings(isAdmin))
		}
		return c.Send(text, keyboards.EmptyChatSettings(isAdmin))
	}
	text := fmt.Sprintf(
		"Расписание группового чата\n\nВуз: %s\nГруппа: %s\n\n"+
			"Доступны /today, /tomorrow, /week, /twoweeks и /date.",
		profile.UniversityName,
		profile.GroupName,
	)
	if c.Callback() != nil {
		return editOrSend(c, text, keyboards.ChatSettings(profile.GroupName, isAdmin))
	}
	return c.Send(text, keyboards.ChatSettings(profile.GroupName, isAdmin))
}

func (h *Handler) HandleChatChangeGroup(c tele.Context) error {
	if !isGroupChat(c) {
		return respondStaleCallback(c)
	}
	isAdmin, err := chatAdministrator(c)
	if err != nil || !isAdmin {
		return c.Respond(&tele.CallbackResponse{Text: "Изменять группу может только администратор чата", ShowAlert: true})
	}
	_ = c.Respond()
	return c.Send(
		"Отправьте команду с вузом и группой:\n" +
			"/set_chat_group isuct 3/147\n" +
			"или /set_chat_group ispu 1-40",
	)
}

func (h *Handler) HandleRequestUnsetChatGroup(c tele.Context) error {
	if !isGroupChat(c) {
		return respondStaleCallback(c)
	}
	isAdmin, err := chatAdministrator(c)
	if err != nil || !isAdmin {
		return c.Respond(&tele.CallbackResponse{Text: "Удалить привязку может только администратор чата", ShowAlert: true})
	}
	_ = c.Respond()
	return editOrSend(
		c,
		"Удалить привязку расписания к этому чату?",
		keyboards.UnsetChatConfirmation(),
	)
}

func (h *Handler) HandleConfirmUnsetChatGroup(c tele.Context) error {
	if !isGroupChat(c) {
		return respondStaleCallback(c)
	}
	isAdmin, err := chatAdministrator(c)
	if err != nil || !isAdmin {
		return c.Respond(&tele.CallbackResponse{Text: "Недостаточно прав", ShowAlert: true})
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	err = h.ChatProfileService.Delete(ctx, strconv.FormatInt(c.Chat().ID, 10))
	if errors.Is(err, sql.ErrNoRows) {
		return editOrSend(c, "Привязка уже удалена.", keyboards.EmptyChatSettings(true))
	}
	if err != nil {
		slog.Error("delete chat schedule profile failed", "chat_id", c.Chat().ID, "err", err)
		return c.Send("Не удалось удалить настройку чата.")
	}
	return editOrSend(c, "Привязка расписания удалена.", keyboards.EmptyChatSettings(true))
}

func (h *Handler) HandleSetChatGroup(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send("Настроить расписание группового чата можно только внутри него.")
	}
	isAdmin, err := chatAdministrator(c)
	if err != nil {
		slog.Error("check Telegram chat administrator failed", "chat_id", c.Chat().ID, "err", err)
		return c.Send("Не удалось проверить права администратора чата. Попробуйте позже.")
	}
	if !isAdmin {
		return c.Send("Изменять группу расписания может только администратор этого чата.")
	}

	universityID, groupName, ok := chatGroupArguments(c.Args())
	if !ok {
		return c.Send(
			"Укажите вуз и группу:\n" +
				"/set_chat_group isuct 3/147\n" +
				"/set_chat_group ispu 1-40",
		)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	if universityID != "" {
		university, loadErr := h.UniversityService.GetByID(ctx, universityID)
		if loadErr != nil {
			return c.Send("Не удалось проверить вуз. Попробуйте позже.")
		}
		if university == nil || !university.IsActive {
			return c.Send("Такого подключённого вуза нет. Доступные идентификаторы: isuct, ispu.")
		}
	}
	groups, err := h.GroupService.FindActiveByName(ctx, universityID, groupName)
	if err != nil {
		slog.Error(
			"find group for chat failed",
			"university_id", universityID,
			"group_name", groupName,
			"err", err,
		)
		return c.Send("Не удалось найти группу. Попробуйте позже.")
	}
	if len(groups) == 0 {
		return c.Send("Группа не найдена в актуальном расписании. Проверьте вуз и написание.")
	}
	if len(groups) > 1 {
		return c.Send(
			"Группа с таким названием есть в нескольких вузах. " +
				"Укажите вуз: /set_chat_group isuct " + groupName,
		)
	}
	group := groups[0]
	university, err := h.UniversityService.GetByID(ctx, group.UniversityID)
	if err != nil || university == nil {
		return c.Send("Не удалось загрузить сведения о вузе.")
	}
	if err = h.ChatProfileService.Set(
		ctx,
		strconv.FormatInt(c.Chat().ID, 10),
		c.Chat().Title,
		group.ID,
		strconv.FormatInt(c.Sender().ID, 10),
	); err != nil {
		slog.Error("save chat schedule profile failed", "chat_id", c.Chat().ID, "err", err)
		return c.Send("Не удалось сохранить группу этого чата.")
	}
	return c.Send(fmt.Sprintf(
		"Расписание чата настроено.\n\nВуз: %s\nГруппа: %s\n\n"+
			"Теперь участники могут использовать /today, /tomorrow, /week и /date.",
		university.Name,
		group.Name,
	))
}

func (h *Handler) HandleUnsetChatGroup(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send("Эта команда предназначена для групповых чатов.")
	}
	isAdmin, err := chatAdministrator(c)
	if err != nil {
		return c.Send("Не удалось проверить права администратора чата.")
	}
	if !isAdmin {
		return c.Send("Удалить настройку может только администратор этого чата.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	err = h.ChatProfileService.Delete(ctx, strconv.FormatInt(c.Chat().ID, 10))
	if errors.Is(err, sql.ErrNoRows) {
		return c.Send("Группа расписания для этого чата не была настроена.")
	}
	if err != nil {
		slog.Error("delete chat schedule profile failed", "chat_id", c.Chat().ID, "err", err)
		return c.Send("Не удалось удалить настройку чата.")
	}
	return c.Send("Привязка расписания удалена.")
}

func (h *Handler) scheduleTarget(
	requestContext context.Context,
	telegramContext tele.Context,
) *scheduleTarget {
	if isGroupChat(telegramContext) {
		profile, err := h.ChatProfileService.Get(
			requestContext,
			strconv.FormatInt(telegramContext.Chat().ID, 10),
		)
		if err != nil {
			slog.Error(
				"load chat schedule target failed",
				"chat_id", telegramContext.Chat().ID,
				"err", err,
			)
			_ = telegramContext.Send("Не удалось загрузить группу этого чата.")
			return nil
		}
		if profile == nil {
			_ = telegramContext.Send(
				"Для этого чата расписание ещё не настроено. " +
					"Администратор может использовать /set_chat_group.",
			)
			return nil
		}
		return &scheduleTarget{
			GroupID:      profile.DefaultGroupID,
			GroupName:    profile.GroupName,
			UniversityID: profile.UniversityID,
			University:   profile.UniversityName,
			ViewFormat:   domain.ScheduleViewCompact,
		}
	}

	state, err := h.readyState(requestContext, telegramContext.Sender().ID)
	if err != nil {
		slog.Error(
			"restore schedule target failed",
			"user_id", telegramContext.Sender().ID,
			"err", err,
		)
		_ = telegramContext.Send("Не удалось загрузить сохранённую группу. Попробуйте позже.")
		return nil
	}
	if state == nil {
		_ = telegramContext.Send("Для начала работы используйте /start")
		return nil
	}
	viewFormat := domain.ScheduleViewCompact
	if subscriptions, loadErr := h.SubscriptionService.GetGroupSubscriptions(
		requestContext,
		fmt.Sprint(telegramContext.Sender().ID),
	); loadErr != nil {
		slog.Warn("load schedule view preference failed", "user_id", telegramContext.Sender().ID, "err", loadErr)
	} else {
		for _, subscription := range subscriptions {
			if subscription.GroupID == state.GroupID {
				viewFormat = subscription.ScheduleViewFormat
				break
			}
		}
	}
	return &scheduleTarget{
		GroupID:      state.GroupID,
		GroupName:    state.Query,
		UniversityID: state.UniversityID,
		University:   state.University,
		ViewFormat:   viewFormat,
	}
}

func chatAdministrator(c tele.Context) (bool, error) {
	member, err := c.Bot().ChatMemberOf(c.Chat(), c.Sender())
	if err != nil {
		return false, err
	}
	return member.Role == tele.Creator || member.Role == tele.Administrator, nil
}

func chatGroupArguments(args []string) (string, string, bool) {
	if len(args) == 0 {
		return "", "", false
	}
	if len(args) == 1 {
		if universityID, groupName, found := strings.Cut(args[0], ":"); found {
			universityID = strings.ToLower(strings.TrimSpace(universityID))
			groupName = strings.TrimSpace(groupName)
			return universityID, groupName, universityID != "" && groupName != ""
		}
		groupName := strings.TrimSpace(args[0])
		return "", groupName, groupName != ""
	}
	universityID := strings.ToLower(strings.TrimSpace(args[0]))
	groupName := strings.TrimSpace(strings.Join(args[1:], " "))
	return universityID, groupName, universityID != "" && groupName != ""
}
