package miniapp

import (
	"fmt"
	"net/url"
	"strings"

	telegram "gopkg.in/telebot.v3"
)

func EditorURL(publicURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("ADMIN_PUBLIC_URL must be a public HTTPS URL")
	}
	parsed.Fragment = "/editor"
	return parsed.String(), nil
}

func AppURL(publicURL string) (string, error) {
	editor, err := EditorURL(publicURL)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(editor)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/app"
	parsed.Fragment = ""
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func ConfigureMenu(bot *telegram.Bot, user *telegram.User, publicURL string, isAdmin bool) error {
	miniAppURL, err := AppURL(publicURL)
	if err != nil {
		if !isAdmin {
			return bot.SetMenuButton(user, telegram.MenuButtonCommands)
		}
		return err
	}
	return bot.SetMenuButton(user, &telegram.MenuButton{Type: telegram.MenuButtonWebApp, Text: "Расписание", WebApp: &telegram.WebApp{URL: miniAppURL}})
}

func MenuFingerprint(publicURL string, isAdmin bool) string {
	appURL, err := AppURL(publicURL)
	if err != nil {
		return "commands:v1"
	}
	return fmt.Sprintf("personal:v1:%t:%s", isAdmin, appURL)
}
