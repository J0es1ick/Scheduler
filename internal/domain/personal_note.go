package domain

import (
	"encoding/json"
	"time"
)

type PersonalNote struct {
	ID         string          `db:"id" json:"id"`
	UserID     string          `db:"user_id" json:"-"`
	Role       UserRole        `db:"role" json:"role"`
	TargetID   string          `db:"target_id" json:"target_id"`
	LessonID   string          `db:"lesson_id" json:"lesson_id"`
	LessonDate time.Time       `db:"lesson_date" json:"lesson_date"`
	Basis      json.RawMessage `db:"basis" json:"basis"`
	Body       string          `db:"body" json:"body"`
	DueDate    *time.Time      `db:"due_date" json:"due_date"`
	Completed  bool            `db:"completed" json:"completed"`
	Version    int64           `db:"version" json:"version"`
	CreatedAt  time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updated_at"`
}
