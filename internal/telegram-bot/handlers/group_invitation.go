package handlers

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/miniapp"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleNotificationCenter(c tele.Context) error {
	if c.Callback() != nil {
		_ = c.Respond()
	}
	app, err := miniapp.AppURL(h.AdminPublicURL)
	if err != nil {
		return c.Send("Настройки: /daily — расписание, /reminders — напоминания, /quiet_hours — тихие часы, /updates — новости.")
	}
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(tele.Btn{Text: "Что будет приходить", WebApp: &tele.WebApp{URL: app + "?section=notifications"}}))
	return c.Send("Все уведомления, пауза и ближайшие сообщения — на одной странице.", menu)
}

func (h *Handler) HandleShareGroup(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	state, err := h.readyState(ctx, c.Sender().ID)
	if err != nil {
		return err
	}
	if state == nil || state.Role == domain.RoleTeacher {
		return c.Send("Поделиться можно учебной группой из студенческих подписок.")
	}
	groupID := state.GroupID
	if value, ok := callbackArgument(c); ok && value != "" {
		items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
		if err != nil {
			return err
		}
		groupID = ""
		for _, item := range items {
			if keyboards.GroupToken(item.GroupID) == value && item.IsActive {
				groupID = item.GroupID
			}
		}
	}
	group, err := h.GroupService.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil || !group.IsActive {
		return c.Send("Группа сейчас не публикуется. Поделиться ей можно после восстановления расписания.")
	}
	if c.Bot().Me == nil || c.Bot().Me.Username == "" {
		return c.Send("Не удалось сформировать ссылку. Попробуйте позже.")
	}
	link := "https://t.me/" + c.Bot().Me.Username + "?start=group_" + keyboards.GroupToken(group.ID)
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.URL("Поделиться группой", "https://t.me/share/url?"+url.Values{"url": {link}, "text": {"Расписание группы " + group.Name}}.Encode())))
	if c.Callback() != nil {
		_ = c.Respond()
	}
	return c.Send("Ссылка на группу "+group.Name+":\n"+link+"\n\nПолучатель сам подтвердит группу и выберет подгруппу. Ваши личные правки и заметки не передаются.", menu)
}

func (h *Handler) handleGroupInvitation(c tele.Context, payload string) error {
	ctx, cancel := reqCtx()
	defer cancel()
	group, err := h.GroupService.GetActiveGroupByToken(ctx, strings.TrimPrefix(payload, "group_"))
	if err != nil {
		return err
	}
	if group == nil {
		return c.Send("Эта группа сейчас недоступна или ссылка устарела. Выберите расписание через /start.")
	}
	uni, err := h.UniversityService.GetByID(ctx, group.UniversityID)
	if err != nil {
		return err
	}
	if uni == nil || !uni.IsActive {
		return c.Send("Вуз временно недоступен.")
	}
	userID := fmt.Sprint(c.Sender().ID)
	previous, err := h.UserService.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	user, err := h.UserService.RegisterOrGetUser(ctx, userID, c.Sender().Username)
	if err != nil {
		return err
	}
	if previous == nil {
		if err = h.UserService.SetNotificationsEnabled(ctx, userID, false); err != nil {
			return err
		}
	}
	if err = h.configureMiniAppMenu(ctx, c.Bot(), c.Sender(), user.IsAdmin); err == nil {
		_ = h.UserService.MarkTelegramMenuConfigured(ctx, user.ID, miniapp.MenuFingerprint(h.AdminPublicURL, user.IsAdmin))
	}
	if user.Role == domain.RoleTeacher {
		return c.Send(uni.Name + " · " + group.Name + "\nЭто приглашение для студента. Чтобы подключить группу, сначала выберите роль студента через /role и снова откройте ссылку.")
	}
	state := &dto.UserState{Role: domain.RoleStudent, UniversityID: uni.ID, University: uni.Name, GroupID: group.ID, Query: group.Name, Step: "invite_confirm", FlowNonce: newFlowNonce()}
	h.StateManager.Set(c.Sender().ID, state)
	menu := &tele.ReplyMarkup{}
	rows := []tele.Row{menu.Row(menu.Data("Подключить как основную", "invite_confirm", "primary", state.FlowNonce))}
	if user.DefaultGroupID != "" {
		rows = append(rows, menu.Row(menu.Data("Добавить, сохранив основную", "invite_confirm", "additional", state.FlowNonce)))
	}
	rows = append(rows, menu.Row(menu.Data("Отмена", "invite_confirm", "cancel", state.FlowNonce)))
	menu.Inline(rows...)
	return c.Send(uni.Name+" · группа "+group.Name+"\n\nПодключить это расписание? Настройки уведомлений выбираются отдельно.", menu)
}

func (h *Handler) HandleInvitationConfirm(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) != 2 || !validFlow(state, "invite_confirm", args[1]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	if args[0] == "cancel" {
		h.StateManager.Delete(c.Sender().ID)
		return c.Send("Подключение отменено. Ваши настройки сохранены.")
	}
	if args[0] != "primary" && args[0] != "additional" {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	from := time.Now().In(h.universityLocation(ctx, state.UniversityID))
	data, err := h.ScheduleService.GetScheduleForGroupRange(ctx, state.GroupID, from, from.AddDate(0, 0, 30))
	if err != nil {
		return c.Send("Группа временно недоступна. Откройте ссылку позже.")
	}
	seen := map[int]bool{}
	for _, lessons := range data {
		for _, lesson := range lessons {
			if lesson.Subgroup > 0 && lesson.Subgroup <= 100 {
				seen[lesson.Subgroup] = true
			}
		}
	}
	subgroups := []int{}
	for value := range seen {
		subgroups = append(subgroups, value)
	}
	sort.Ints(subgroups)
	state.InviteSubgroups = subgroups
	state.Step = "invite_subgroup"
	state.FlowNonce = newFlowNonce()
	state.SetSelectedGroupDefault = args[0] == "primary"
	h.StateManager.Set(c.Sender().ID, state)
	return showInvitationSubgroups(c, state, 0)
}

func showInvitationSubgroups(c tele.Context, state *dto.UserState, page int) error {
	if page < 0 || page > len(state.InviteSubgroups)/10 {
		return respondStaleCallback(c)
	}
	menu := &tele.ReplyMarkup{}
	rows := []tele.Row{menu.Row(menu.Data("Все подгруппы", "invite_subgroup", "0", state.FlowNonce))}
	for _, value := range state.InviteSubgroups[page*10 : min((page+1)*10, len(state.InviteSubgroups))] {
		rows = append(rows, menu.Row(menu.Data(fmt.Sprintf("Подгруппа %d", value), "invite_subgroup", strconv.Itoa(value), state.FlowNonce)))
	}
	navigation := []tele.Btn{}
	if page > 0 {
		navigation = append(navigation, menu.Data("←", "invite_subgroup", fmt.Sprintf("page:%d", page-1), state.FlowNonce))
	}
	if (page+1)*10 < len(state.InviteSubgroups) {
		navigation = append(navigation, menu.Data("→", "invite_subgroup", fmt.Sprintf("page:%d", page+1), state.FlowNonce))
	}
	if len(navigation) > 0 {
		rows = append(rows, menu.Row(navigation...))
	}
	rows = append(rows, menu.Row(menu.Data("Отмена", "invite_subgroup", "cancel", state.FlowNonce)))
	menu.Inline(rows...)
	return editOrSend(c, "Выберите подгруппу. Изменить её можно позже в настройках.", menu)
}

func (h *Handler) HandleInvitationSubgroup(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) != 2 || !validFlow(state, "invite_subgroup", args[1]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	if args[0] == "cancel" {
		h.StateManager.Delete(c.Sender().ID)
		return c.Send("Подключение отменено.")
	}
	if pageText, ok := strings.CutPrefix(args[0], "page:"); ok {
		page, err := strconv.Atoi(pageText)
		if err != nil {
			return respondStaleCallback(c)
		}
		return showInvitationSubgroups(c, state, page)
	}
	subgroup, err := strconv.Atoi(args[0])
	if err != nil || subgroup < 0 || subgroup > 100 {
		return respondStaleCallback(c)
	}
	service, ok := h.SubscriptionService.(interface {
		AcceptGroupInvitation(context.Context, string, string, int, bool) error
	})
	if !ok {
		return c.Send("Подключение недоступно. Попробуйте позже.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	if err = service.AcceptGroupInvitation(ctx, fmt.Sprint(c.Sender().ID), state.GroupID, subgroup, state.SetSelectedGroupDefault); err != nil {
		return c.Send("Не удалось подключить группу. Она могла стать неактивной; откройте приглашение заново.")
	}
	h.StateManager.Delete(c.Sender().ID)
	target, err := h.downloadTarget(ctx, c, keyboards.GroupToken(state.GroupID))
	if err != nil {
		return err
	}
	date := time.Now().In(h.universityLocation(ctx, state.UniversityID))
	days, err := h.getScheduleForTarget(ctx, target, date, date)
	if err != nil {
		return err
	}
	if err = h.sendScheduleView(ctx, c, days, target, date, 1, scheduleDayNavigationForTarget(date, target, false), ""); err != nil {
		return err
	}
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Настроить уведомления", "notification_center")), menu.Row(menu.Data("Новости Scheduler", "updates_settings")))
	return c.Send("Группа подключена. При желании настройте сообщения — новые рассылки автоматически не включались.", menu)
}
