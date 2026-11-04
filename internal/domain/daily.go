package domain

import (
	"encoding/json"
	"errors"
)

var ErrEmptyDailySchedule = errors.New("skip empty daily schedule")

type ScheduleMessage struct {
	Text   string          `json:"text,omitempty"`
	PNG    []byte          `json:"png,omitempty"`
	Markup json.RawMessage `json:"markup,omitempty"`
}

type DailyContext struct {
	Date     string `json:"date"`
	Timezone string `json:"timezone"`
}
