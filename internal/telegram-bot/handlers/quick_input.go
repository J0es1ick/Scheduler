package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

type quickInputKind string

const (
	quickInputNone    quickInputKind = ""
	quickInputDate    quickInputKind = "date"
	quickInputOffset  quickInputKind = "offset"
	quickInputWeekday quickInputKind = "weekday"
	quickInputPeriod  quickInputKind = "period"
	quickInputGroup   quickInputKind = "group"
	quickInputTeacher quickInputKind = "teacher"
)

type quickInput struct {
	kind  quickInputKind
	value string
}

var (
	quickDatePattern    = regexp.MustCompile(`^\d{2}\.\d{2}\.(?:\d{2}|\d{4})$`)
	quickOffsetPattern  = regexp.MustCompile(`^-?[0-7]$`)
	quickIntegerPattern = regexp.MustCompile(`^-?\d+$`)
	quickGroupPattern   = regexp.MustCompile(`^[\p{L}\p{N}/\\№_. –—-]+$`)
	quickWeekdays       = map[string]int{
		"понедельник": 1, "пн": 1,
		"вторник": 2, "вт": 2,
		"среда": 3, "ср": 3,
		"четверг": 4, "чт": 4,
		"пятница": 5, "пт": 5,
		"суббота": 6, "сб": 6,
		"воскресенье": 7, "вс": 7,
	}
)

func parseQuickInput(value string) quickInput {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	switch lower {
	case "сегодня":
		return quickInput{kind: quickInputOffset, value: "0"}
	case "завтра":
		return quickInput{kind: quickInputOffset, value: "1"}
	case "неделя":
		return quickInput{kind: quickInputPeriod, value: "7"}
	case "две недели":
		return quickInput{kind: quickInputPeriod, value: "14"}
	}
	if quickOffsetPattern.MatchString(lower) {
		offset, _ := strconv.Atoi(lower)
		return quickInput{kind: quickInputOffset, value: strconv.Itoa(offset)}
	}
	if quickIntegerPattern.MatchString(lower) {
		return quickInput{}
	}
	if quickDatePattern.MatchString(lower) {
		return quickInput{kind: quickInputDate, value: lower}
	}
	if weekday := quickWeekdays[lower]; weekday > 0 {
		return quickInput{kind: quickInputWeekday, value: strconv.Itoa(weekday)}
	}
	if strings.HasPrefix(lower, "поиск ") {
		query := strings.TrimSpace(value[len("поиск "):])
		if query != "" {
			return quickInput{kind: quickInputTeacher, value: query}
		}
	}
	if looksLikeTeacherQuery(value) {
		return quickInput{kind: quickInputTeacher, value: value}
	}
	if len([]rune(value)) <= 80 && strings.IndexFunc(value, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 &&
		strings.IndexFunc(value, func(r rune) bool { return unicode.IsLetter(r) || strings.ContainsRune("/\\-–— _.", r) }) >= 0 &&
		quickGroupPattern.MatchString(value) {
		return quickInput{kind: quickInputGroup, value: value}
	}
	return quickInput{}
}

func looksLikeTeacherQuery(value string) bool {
	if strings.EqualFold(strings.TrimSpace(value), "поиск") {
		return false
	}
	fields := strings.Fields(value)
	if len(fields) < 1 || len(fields) > 3 {
		return false
	}
	if len(fields) > 1 {
		first, _ := utf8.DecodeRuneInString(fields[0])
		if !unicode.IsUpper(first) {
			return false
		}
	}
	letters := 0
	cyrillic := 0
	for _, character := range value {
		if unicode.IsLetter(character) {
			letters++
			if unicode.In(character, unicode.Cyrillic) {
				cyrillic++
			}
			continue
		}
		if !unicode.IsSpace(character) && character != '.' && character != '-' && character != '–' && character != '—' {
			return false
		}
	}
	return letters >= 3 && cyrillic > 0
}

func (h *Handler) handleQuickTextInput(c tele.Context, input string) (bool, error) {
	request := parseQuickInput(input)
	if request.kind == quickInputNone {
		return false, nil
	}
	ctx, cancel := reqCtx()
	defer cancel()
	state, err := h.readyState(ctx, c.Sender().ID)
	if err != nil {
		return true, c.Send("Не удалось загрузить основную группу. Попробуйте позже.")
	}
	if state == nil {
		return true, c.Send("Сначала выберите доступную основную группу: /start")
	}
	if request.kind == quickInputGroup {
		return true, h.selectQuickPrimaryGroup(ctx, c, state, request.value)
	}
	if !state.GroupActive {
		return true, c.Send("Основная группа временно недоступна. Выберите другую группу в «Мои группы».")
	}
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return true, nil
	}
	location := h.universityLocation(ctx, target.UniversityID)
	now := dateAtLocation(time.Now().In(location), location)
	switch request.kind {
	case quickInputDate:
		date, parseErr := parseScheduleDate(request.value, location)
		if parseErr != nil {
			return true, c.Send("Не удалось распознать дату. Пример: 08.01.2002")
		}
		return true, h.sendTargetDate(ctx, c, target, date)
	case quickInputOffset:
		offset, _ := strconv.Atoi(request.value)
		return true, h.sendTargetDate(ctx, c, target, now.AddDate(0, 0, offset))
	case quickInputWeekday:
		weekday, _ := strconv.Atoi(request.value)
		selected := now
		for weekdayNumber(selected) != weekday {
			selected = selected.AddDate(0, 0, 1)
		}
		return true, h.sendTargetDate(ctx, c, target, selected)
	case quickInputPeriod:
		daysCount, _ := strconv.Atoi(request.value)
		return true, h.sendTargetWeek(ctx, c, target, now, daysCount)
	case quickInputTeacher:
		return true, h.beginTeacherSearch(ctx, c, state, request.value, "quick")
	default:
		return false, nil
	}
}

func (h *Handler) selectQuickPrimaryGroup(
	ctx context.Context,
	c tele.Context,
	state *dto.UserState,
	query string,
) error {
	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		return c.Send("Не удалось загрузить список вузов. Попробуйте позже.")
	}
	university, groupQuery, err := resolveGroupInput(query, state.UniversityID, false, universities)
	if err != nil {
		return c.Send(err.Error())
	}
	var group *domain.Group
	for _, variant := range groupQueryVariants(groupQuery) {
		group, err = h.GroupService.GetGroupByName(ctx, university.ID, variant)
		if err != nil {
			return c.Send("Не удалось проверить группу. Попробуйте позже.")
		}
		if group != nil {
			break
		}
	}
	if group == nil {
		return c.Send(university.Name + ": такой активной группы нет.")
	}
	if err = h.SubscriptionService.SubscribeAndSetDefault(ctx, fmt.Sprint(c.Sender().ID), group.ID); err != nil {
		return c.Send("Не удалось сменить основную группу. Попробуйте позже.")
	}
	if _, _, err = h.restoreProfile(ctx, c.Sender().ID); err != nil {
		return c.Send("Группа сохранена, но профиль не удалось обновить. Используйте /start.")
	}
	return c.Send("Основная группа изменена: "+university.Name+" · "+group.Name, keyboards.MainMenu())
}
