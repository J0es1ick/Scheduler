package bot

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func ConfigureIdentity(bot *tele.Bot, configuredUsername, publicURL string) (string, error) {
	if bot == nil {
		return "", errors.New("bot is nil")
	}

	username := normalizeUsername(configuredUsername)
	if username == "" {
		username = usernameFromPublicURL(publicURL)
	}
	if username == "" {
		return "", errors.New("BOT_USERNAME is empty and cannot be inferred from BOT_PUBLIC_URL")
	}
	if err := validateUsername(username); err != nil {
		return "", fmt.Errorf("invalid BOT_USERNAME: %w", err)
	}

	if bot.Me == nil {
		bot.Me = &tele.User{}
	}
	bot.Me.Username = username
	return username, nil
}

func usernameFromPublicURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	if !strings.EqualFold(parsed.Hostname(), "t.me") &&
		!strings.EqualFold(parsed.Hostname(), "telegram.me") {
		return ""
	}
	return normalizeUsername(strings.Trim(parsed.Path, "/"))
}

func normalizeUsername(username string) string {
	return strings.TrimPrefix(strings.TrimSpace(username), "@")
}

func validateUsername(username string) error {
	if len(username) < 5 || len(username) > 32 {
		return errors.New("must contain from 5 to 32 characters")
	}
	for _, char := range username {
		isLetter := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
		isDigit := char >= '0' && char <= '9'
		if char != '_' && !isLetter && !isDigit {
			return errors.New("must contain only letters, digits, and underscores")
		}
	}
	if !strings.HasSuffix(strings.ToLower(username), "bot") {
		return errors.New("must end with 'bot'")
	}
	return nil
}
