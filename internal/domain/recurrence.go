package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

type RecurrenceRule struct {
	CycleLength int        `json:"cycle_length,omitempty"`
	CycleWeeks  []int      `json:"cycle_weeks,omitempty"`
	AnchorDate  *time.Time `json:"anchor_date,omitempty"`
}

func (r RecurrenceRule) IsZero() bool {
	return r.CycleLength == 0 && len(r.CycleWeeks) == 0 && r.AnchorDate == nil
}

func (r RecurrenceRule) Matches(date time.Time, fallbackAnchor *time.Time) bool {
	if r.CycleLength <= 0 || len(r.CycleWeeks) == 0 {
		return true
	}
	anchor := fallbackAnchor
	if r.AnchorDate != nil {
		anchor = r.AnchorDate
	}
	if anchor == nil {
		return false
	}
	start := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, anchor.Location())
	current := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	days := int(current.Sub(start).Hours() / 24)
	if days < 0 {
		return false
	}
	position := (days/7)%r.CycleLength + 1
	return slices.Contains(r.CycleWeeks, position)
}

func (r RecurrenceRule) Value() (driver.Value, error) {
	if r.IsZero() {
		return []byte(`{}`), nil
	}
	return json.Marshal(r)
}

func (r *RecurrenceRule) Scan(value any) error {
	if value == nil {
		*r = RecurrenceRule{}
		return nil
	}
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("scan recurrence: unsupported value %T", value)
	}
	if len(raw) == 0 {
		*r = RecurrenceRule{}
		return nil
	}
	return json.Unmarshal(raw, r)
}
