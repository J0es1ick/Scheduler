package domain

import "time"

type PrivacyDeletionRequest struct {
	ID             string     `db:"id"`
	UserID         string     `db:"user_id"`
	Attempts       int        `db:"attempts"`
	ClaimToken     string     `db:"claim_token"`
	LeaseExpiresAt *time.Time `db:"lease_expires_at"`
}
