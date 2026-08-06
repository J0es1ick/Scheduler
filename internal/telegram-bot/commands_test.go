package bot

import (
	"testing"

	tele "gopkg.in/telebot.v3"
)

func TestPrivateCommandMenuContainsOnlyPrimaryCommands(t *testing.T) {
	commands := privateCommands()
	if len(commands) > 10 {
		t.Fatalf("private command picker is cluttered: %d commands", len(commands))
	}
	for _, hidden := range []string{"admin", "metrics", "hotline", "privacy", "delete_me"} {
		if containsCommand(commands, hidden) {
			t.Errorf("advanced command %q must not clutter the primary picker", hidden)
		}
	}
	for _, primary := range []string{"start", "today", "tomorrow", "week", "date", "search", "settings", "help"} {
		if !containsCommand(commands, primary) {
			t.Errorf("primary command %q is missing", primary)
		}
	}
}

func TestAdminCommandMenuAddsOperationalCommands(t *testing.T) {
	commands := adminPrivateCommands()
	for _, command := range []string{"admin", "metrics"} {
		if !containsCommand(commands, command) {
			t.Errorf("administrator command %q is missing", command)
		}
	}
}

func containsCommand(commands []tele.Command, target string) bool {
	for _, command := range commands {
		if command.Text == target {
			return true
		}
	}
	return false
}
