package handlers

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func qualifiedGroupPrompt() string {
	return "Отправьте аббревиатуру вуза и группу одним сообщением.\n\n" +
		"Например:\nИГХТУ 4/147\nИГХТУ 4 147\nИГХТУ 4 курс 147 группа\n\n" +
		"Для ИГЭУ: ИГЭУ 1-ЭЭ-В.\nРегистр не важен. Названия подключённых вузов — /sources. " +
		"Вместо аббревиатуры можно указать код вуза, например isuct."
}

func resolveGroupInput(input, defaultUniversityID string, requireUniversity bool, universities []domain.University) (*domain.University, string, error) {
	input = strings.Join(strings.Fields(input), " ")
	lower := strings.ToLower(input)
	bestLength := 0
	var matches []domain.University
	query := ""
	for _, university := range universities {
		if !university.IsActive {
			continue
		}
		for _, alias := range []string{university.ID, university.Name, university.FullName} {
			alias = strings.ToLower(strings.Join(strings.Fields(alias), " "))
			if alias == "" || !strings.HasPrefix(lower, alias) {
				continue
			}
			remainder := strings.TrimPrefix(lower, alias)
			if remainder != "" && !strings.HasPrefix(remainder, " ") && !strings.HasPrefix(remainder, ":") {
				continue
			}
			if len(alias) > bestLength {
				bestLength = len(alias)
				matches = nil
				query = strings.TrimSpace(strings.TrimLeft(remainder, " :"))
			}
			if len(alias) == bestLength && !slices.ContainsFunc(matches, func(item domain.University) bool { return item.ID == university.ID }) {
				matches = append(matches, university)
			}
		}
	}
	if len(matches) > 1 {
		codes := make([]string, 0, len(matches))
		for _, university := range matches {
			codes = append(codes, university.ID)
		}
		return nil, "", fmt.Errorf("У нескольких вузов такая аббревиатура. Укажите код перед группой: %s.", strings.Join(codes, ", "))
	}
	if len(matches) == 1 {
		if query == "" {
			return nil, "", fmt.Errorf("После названия вуза укажите группу, например «%s 4/147».", matches[0].Name)
		}
		return &matches[0], query, nil
	}
	if !requireUniversity && defaultUniversityID != "" {
		for _, university := range universities {
			if university.ID == defaultUniversityID && university.IsActive {
				return &university, input, nil
			}
		}
	}
	return nil, "", fmt.Errorf("Не удалось определить вуз. %s", qualifiedGroupPrompt())
}

func groupQueryVariants(query string) []string {
	query = strings.Join(strings.Fields(query), " ")
	variants := []string{query}
	parts := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return unicode.IsSpace(r) || r == '/' || r == '\\' || r == '-' || r == '–' || r == '—'
	})
	parts = slices.DeleteFunc(parts, func(part string) bool {
		return slices.Contains([]string{"курс", "курса", "группа", "группы", "гр", "гр."}, part)
	})
	if len(parts) < 2 {
		return variants
	}
	separator := "-"
	if len(parts) == 2 && digitsOnly(parts[0]) && digitsOnly(parts[1]) {
		separator = "/"
	}
	canonical := strings.Join(parts, separator)
	if !strings.EqualFold(query, canonical) {
		variants = append(variants, canonical)
	}
	return variants
}

func digitsOnly(value string) bool {
	return value != "" && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) == -1
}

func groupInputBack(state *dto.UserState) *tele.ReplyMarkup {
	if state.GroupChangeDestination == "university" {
		return keyboards.BackButton("back_university_selection", state.FlowNonce)
	}
	if state.GroupChangeDestination == "subscriptions" {
		return keyboards.BackButton("cancel_group_change", "subscriptions", fmt.Sprint(state.GroupChangePage), state.FlowNonce)
	}
	return keyboards.BackButton("cancel_group_change", "main", state.FlowNonce)
}
