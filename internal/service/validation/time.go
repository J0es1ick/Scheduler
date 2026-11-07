package validation

import (
	"fmt"
	"strings"
	"time"
)

func ParseDailyTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := time.Parse("15:04", value)
	if err != nil || len(value) != 5 {
		return "", fmt.Errorf("укажите время в формате ЧЧ:ММ, например 06:00")
	}
	return parsed.Format("15:04"), nil
}
