package domain

import (
	"encoding/json"
	"time"
)

const (
	SupportRequestFeedback       = "feedback"
	SupportRequestUpdateExisting = "update_existing"
	SupportRequestNewInstitution = "new_institution"
)

type SupportRequest struct {
	ID          string     `db:"id" json:"id"`
	UserID      string     `db:"user_id" json:"user_id"`
	RequestType string     `db:"request_type" json:"request_type"`
	Details     string     `db:"details" json:"details"`
	Status      string     `db:"status" json:"status"`
	ReviewNote  string     `db:"review_note" json:"review_note"`
	ReviewedBy  string     `db:"reviewed_by" json:"reviewed_by"`
	ReviewedAt  *time.Time `db:"reviewed_at" json:"reviewed_at"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}

type BotOutboxDelivery struct {
	BroadcastID         string          `db:"broadcast_id"`
	BroadcastMessageIDs json.RawMessage `db:"broadcast_message_ids"`
	TeacherID           string          `db:"teacher_id"`
	ScheduleContext     json.RawMessage `db:"schedule_context"`
	ScheduleMessages    json.RawMessage `db:"schedule_messages"`
	DeliveredParts      int             `db:"delivered_parts"`
	GroupID             string          `db:"group_id"`
	ExpiresAt           *time.Time      `db:"expires_at"`
	ReminderContext     json.RawMessage `db:"reminder_context"`
	ID                  string          `db:"id"`
	UserID              string          `db:"user_id"`
	RequestID           string          `db:"request_id"`
	Kind                string          `db:"kind"`
	Body                string          `db:"body"`
	Attempts            int             `db:"attempts"`
	ClaimToken          string          `db:"claim_token"`
	LeaseExpiresAt      *time.Time      `db:"lease_expires_at"`
}
