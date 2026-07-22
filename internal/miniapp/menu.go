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

func ConfigureMenu(bot *telegram.Bot, user *telegram.User, publicURL string, isAdmin bool) error {
	if !isAdmin {
		return bot.SetMenuButton(user, telegram.MenuButtonCommands)
	}
	miniAppURL, err := EditorURL(publicURL)
	if err != nil {
		return err
	}
	return bot.SetMenuButton(user, &telegram.MenuButton{
		Type: telegram.MenuButtonWebApp,
		Text: "Админка",
		WebApp: &telegram.WebApp{
			URL: miniAppURL,
		},
	})
}
