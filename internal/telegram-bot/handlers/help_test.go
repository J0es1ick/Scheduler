package handlers

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestHelpCategoriesCoverEveryRegisteredCommand(t *testing.T) {
	body, err := os.ReadFile("../bot.go")
	if err != nil {
		t.Fatal(err)
	}
	public := helpText(false)
	for _, match := range regexp.MustCompile(`bot.Handle\("(/\w+)"`).FindAllStringSubmatch(string(body), -1) {
		if match[1] == "/admin" || match[1] == "/metrics" {
			continue
		}
		if !strings.Contains(public, match[1]) {
			t.Errorf("command %s has no help category", match[1])
		}
	}
	for _, feature := range []string{"PNG", "JSON", "CSV", "ICS", "подгрупп", "inline", "формат", "пожелан", "от -7 до 7"} {
		if !strings.Contains(public, feature) {
			t.Errorf("feature %s missing", feature)
		}
	}
	for _, topic := range helpTopics {
		if len([]rune(topic.text)) > tgMaxLen {
			t.Errorf("topic %s exceeds Telegram limit", topic.id)
		}
	}
}

func TestHelpTextForRegularUserDoesNotExposeAdminCommands(t *testing.T) {
	text := helpText(false)
	if strings.Contains(text, "/admin") || strings.Contains(text, "/metrics") {
		t.Fatalf("regular help exposes administrator commands:\n%s", text)
	}
}

func TestHelpTextIncludesAllUserCommands(t *testing.T) {
	text := helpText(false)
	for _, command := range []string{
		"/start",
		"/help",
		"/today",
		"/tomorrow",
		"/date",
		"/week",
		"/twoweeks",
		"/search",
		"/change_group",
		"/change_university",
		"/settings",
		"/subscriptions",
		"/reminders",
		"/hotline",
		"/privacy",
		"/my_data",
		"/delete_me",
		"/sources",
		"/chat_settings",
		"/set_chat_group",
		"/unset_chat_group",
	} {
		if !strings.Contains(text, command) {
			t.Errorf("regular help does not contain %s", command)
		}
	}
}

func TestGroupHelpContainsConfigurationCommands(t *testing.T) {
	text := groupHelpText()
	for _, command := range []string{
		"/start",
		"/help",
		"/privacy",
		"/connect_source",
		"/date",
		"/chat_settings",
		"/set_chat_group",
		"/unset_chat_group",
	} {
		if !strings.Contains(text, command) {
			t.Errorf("group help does not contain %s", command)
		}
	}
	if strings.Contains(text, "/metrics") || strings.Contains(text, "/my_data") {
		t.Errorf("group help exposes private commands:\n%s", text)
	}
}

func TestHelpTextForAdminContainsAllAdminCommands(t *testing.T) {
	text := helpText(true)
	for _, command := range []string{"/admin", "/metrics"} {
		if !strings.Contains(text, command) {
			t.Errorf("administrator help does not contain %s", command)
		}
	}
	if !strings.Contains(text, "Команды администратора сервиса:") {
		t.Errorf("administrator help has no dedicated section:\n%s", text)
	}
}
