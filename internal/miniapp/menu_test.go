package miniapp

import "testing"

func TestPersonalMenuURLAndFingerprint(t *testing.T) {
	for _, raw := range []string{"https://example.com", "https://example.com/", "https://example.com/#/editor"} {
		got, err := AppURL(raw)
		if err != nil || got != "https://example.com/app" {
			t.Fatalf("%s: %s %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "http://example.com", "javascript:alert(1)"} {
		if _, err := AppURL(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if MenuFingerprint("https://example.com", false) == "commands:v1" {
		t.Fatal("ordinary user has no app")
	}
	if MenuFingerprint("https://example.com", true) == MenuFingerprint("https://example.com", false) {
		t.Fatal("rights change did not invalidate menu")
	}
	if got, _ := EditorURL("https://example.com"); got != "https://example.com#/editor" {
		t.Fatalf("admin shortcut: %s", got)
	}
}
