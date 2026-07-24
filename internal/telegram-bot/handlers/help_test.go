package handlers

import (
	"strings"
	"testing"
)

func TestHelpTextForRegularUserDoesNotExposeAdminCommands(t *testing.T) {
	text := helpText(false)
	if strings.Contains(text, "/admin") || strings.Contains(text, "/metrics") {
		t.Fatalf("regular help exposes administrator commands:\n%s", text)
	}
}

func TestHelpTextForAdminContainsAllAdminCommands(t *testing.T) {
	text := helpText(true)
	for _, command := range []string{"/admin", "/metrics"} {
		if !strings.Contains(text, command) {
			t.Errorf("administrator help does not contain %s", command)
		}
	}
	if !strings.Contains(text, "Команды администратора:") {
		t.Errorf("administrator help has no dedicated section:\n%s", text)
	}
}
