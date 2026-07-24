package domain

import "time"

const (
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
	ID        string `db:"id"`
	UserID    string `db:"user_id"`
	RequestID string `db:"request_id"`
	Kind      string `db:"kind"`
	Body      string `db:"body"`
	Attempts  int    `db:"attempts"`
}
