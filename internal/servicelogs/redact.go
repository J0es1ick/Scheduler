package servicelogs

import (
	"fmt"
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`(?:bot)?[0-9]{5,}:[A-Za-z0-9_-]{15,}`)
var credentialsPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s/@]+:[^\s/@]+@`)
var assignmentPattern = regexp.MustCompile(`(?i)(password|passwd|pwd|token|secret|authorization|cookie|init_data|access_key|user_id|chat_id|telegram_id|admin_id|sender_id)(["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;&]+)`)
var bearerPattern = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[a-z0-9+/_.=-]+`)
var userPathPattern = regexp.MustCompile(`(/api/users/)[0-9]+`)

func RedactText(text string) string {
	text = tokenPattern.ReplaceAllString(text, "<redacted>")
	text = credentialsPattern.ReplaceAllString(text, "${1}<redacted>@")
	text = assignmentPattern.ReplaceAllString(text, "${1}${2}<redacted>")
	text = userPathPattern.ReplaceAllString(text, "${1}<redacted>")
	return bearerPattern.ReplaceAllString(text, "${1} <redacted>")
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	for _, part := range []string{"password", "passwd", "secret", "token", "authorization", "cookie", "initdata", "accesskey", "privatekey", "credential"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	switch key {
	case "userid", "chatid", "telegramid", "adminid", "senderid", "username", "actorid", "actorname", "email", "phone", "ip", "remoteaddr":
		return true
	}
	return false
}

func redactValue(value any) any {
	switch item := value.(type) {
	case string:
		return RedactText(item)
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, value := range item {
			if sensitiveKey(key) {
				result[key] = "<redacted>"
			} else {
				result[key] = redactValue(value)
			}
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, value := range item {
			result[i] = redactValue(value)
		}
		return result
	default:
		return value
	}
}

func fieldString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := fields[key]; ok && value != nil {
			if text := fmt.Sprint(value); text != "" {
				return text
			}
		}
	}
	return ""
}
