package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

var ErrSupportRequestLimit = errors.New("support request limit reached")

type SupportRequestRepository struct {
	db *sqlx.DB
}

func NewSupportRequestRepository(db *sqlx.DB) *SupportRequestRepository {
	return &SupportRequestRepository{db: db}
}

func (r *SupportRequestRepository) Create(
	ctx context.Context,
	id, userID, requestType, details string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create support request: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "support:"+userID); err != nil {
		return fmt.Errorf("create support request: lock user: %w", err)
	}
	var open int
	if err = tx.GetContext(ctx, &open,
		`SELECT COUNT(*) FROM support_requests WHERE user_id=$1 AND status='pending'`, userID,
	); err != nil {
		return fmt.Errorf("create support request: count open: %w", err)
	}
	if open >= 3 {
		return ErrSupportRequestLimit
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO support_requests (id, user_id, request_type, details)
		VALUES ($1, $2, $3, $4)`, id, userID, requestType, details); err != nil {
		return fmt.Errorf("create support request: insert: %w", err)
	}

	adminMessage := supportAdminMessage(id, requestType, userID, details)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO bot_outbox (id, user_id, request_id, kind, body)
		SELECT $1 || ':admin:' || id, id, $1, 'support_request', $2
		FROM users
		WHERE is_admin
		ON CONFLICT (id) DO NOTHING`, id, adminMessage); err != nil {
		return fmt.Errorf("create support request: notify admins: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("create support request: commit: %w", err)
	}
	return nil
}

func supportAdminMessage(id, requestType, userID, details string) string {
	typeLabel := "Обновление существующего расписания"
	if requestType == domain.SupportRequestNewInstitution {
		typeLabel = "Новое учебное заведение"
	}
	return fmt.Sprintf(
		"📨 Новое обращение в горячую линию\n\n%s\nЗаявка: %s\nПользователь: %s\n\n%s\n\nРассмотреть: раздел «Обращения» в админке.",
		typeLabel, id, userID, truncateSupportMessage(details, 3400),
	)
}

func truncateSupportMessage(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n…полный текст сохранён в админке"
}
