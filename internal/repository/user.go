package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, id, username string, isAdmin bool) (string, error) {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, username, is_admin, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, username, isAdmin, now, now)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		`SELECT id, COALESCE(username, '') AS username, is_admin,
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			created_at, updated_at
		 FROM users WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user %s: %w", id, err)
	}
	return &user, nil
}

func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		`SELECT id, COALESCE(username, '') AS username, is_admin,
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			created_at, updated_at
		 FROM users WHERE username = $1`, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by username %q: %w", username, err)
	}
	return &user, nil
}

func (r *UserRepository) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	var users []domain.User
	err := r.db.SelectContext(ctx, &users,
		`SELECT id, COALESCE(username, '') AS username, is_admin,
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			created_at, updated_at FROM users`)
	if err != nil {
		return nil, fmt.Errorf("get all users: %w", err)
	}
	return users, nil
}

func (r *UserRepository) GetUsersPendingMenuSync(
	ctx context.Context,
	afterID string,
	limit int,
	adminFingerprint string,
	regularFingerprint string,
) ([]domain.User, error) {
	if limit <= 0 {
		return []domain.User{}, nil
	}
	var users []domain.User
	if err := r.db.SelectContext(ctx, &users, `
		SELECT id, COALESCE(username, '') AS username, is_admin
		FROM users
		WHERE ($1 = '' OR id > $1)
		  AND telegram_menu_fingerprint IS DISTINCT FROM
		      CASE WHEN is_admin THEN $3 ELSE $4 END
		ORDER BY id
		LIMIT $2`, afterID, limit, adminFingerprint, regularFingerprint); err != nil {
		return nil, fmt.Errorf("get users pending menu sync after %q: %w", afterID, err)
	}
	if users == nil {
		users = []domain.User{}
	}
	return users, nil
}

func (r *UserRepository) MarkMenuConfigured(ctx context.Context, userID, fingerprint string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE users SET telegram_menu_fingerprint=$2 WHERE id=$1`, userID, fingerprint,
	); err != nil {
		return fmt.Errorf("mark Telegram menu configured for user %s: %w", userID, err)
	}
	return nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, id, username string, isAdmin bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = $1, is_admin = $2, updated_at = $3 WHERE id = $4`,
		username, isAdmin, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update user %s: %w", id, err)
	}
	return nil
}

func (r *UserRepository) SetDefaultGroup(ctx context.Context, userID, groupID string) error {
	var value any
	if groupID != "" {
		value = groupID
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET default_group_id = $1, updated_at = NOW() WHERE id = $2`,
		value, userID,
	)
	if err != nil {
		return fmt.Errorf("set default group for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) SetNotificationsEnabled(ctx context.Context, userID string, enabled bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set notifications for user %s: begin: %w", userID, err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx,
		`UPDATE users SET notifications_enabled = $1, updated_at = NOW() WHERE id = $2`,
		enabled, userID,
	)
	if err != nil {
		return fmt.Errorf("set notifications for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if !enabled {
		if _, err = tx.ExecContext(ctx, `
			UPDATE notification_deliveries
			SET status='cancelled', updated_at=NOW()
			WHERE user_id=$1 AND status='pending'`, userID); err != nil {
			return fmt.Errorf("cancel pending notifications for user %s: %w", userID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("set notifications for user %s: commit: %w", userID, err)
	}
	return nil
}

func (r *UserRepository) SetLessonReminder(
	ctx context.Context,
	userID string,
	enabled bool,
	minutes int,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set lesson reminder for user %s: begin: %w", userID, err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET reminder_enabled=$1, reminder_minutes=$2, updated_at=NOW()
		WHERE id=$3`,
		enabled, minutes, userID,
	)
	if err != nil {
		return fmt.Errorf("set lesson reminder for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if !enabled {
		if _, err = tx.ExecContext(ctx, `
			UPDATE bot_outbox
			SET status='cancelled', updated_at=NOW()
			WHERE user_id=$1 AND kind='lesson_reminder' AND status='pending'`,
			userID,
		); err != nil {
			return fmt.Errorf("cancel lesson reminders for user %s: %w", userID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("set lesson reminder for user %s: commit: %w", userID, err)
	}
	return nil
}

func (r *UserRepository) SetQuietHours(
	ctx context.Context,
	userID string,
	enabled bool,
	start string,
	end string,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET quiet_hours_enabled=$1, quiet_hours_start=$2::time,
			quiet_hours_end=$3::time, updated_at=NOW()
		WHERE id=$4`, enabled, start, end, userID)
	if err != nil {
		return fmt.Errorf("set quiet hours for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {
	randomMarker := make([]byte, 16)
	if _, err := rand.Read(randomMarker); err != nil {
		return fmt.Errorf("delete user %s: generate anonymous marker: %w", id, err)
	}
	anonymizedID := "deleted:" + hex.EncodeToString(randomMarker)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete user %s: begin: %w", id, err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM admin_sessions WHERE admin_id=$1`, id); err != nil {
		return fmt.Errorf("delete user %s: revoke admin sessions: %w", id, err)
	}
	statements := []string{
		`UPDATE admin_audit_logs SET actor_id=$2, actor_name='Удалённый пользователь', ip_address='' WHERE actor_id=$1`,
		`UPDATE chat_schedule_profiles SET configured_by=$2 WHERE configured_by=$1`,
		`UPDATE lesson_overrides SET created_by=$2 WHERE created_by=$1`,
		`UPDATE support_requests SET reviewed_by=$2 WHERE reviewed_by=$1`,
		`UPDATE parser_snapshots SET reviewed_by=$2 WHERE reviewed_by=$1`,
		`UPDATE connector_clients SET created_by=$2 WHERE created_by=$1`,
	}
	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement, id, anonymizedID); err != nil {
			return fmt.Errorf("delete user %s: anonymize references: %w", id, err)
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user %s: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("delete user %s: commit: %w", id, err)
	}
	return nil
}

func (r *UserRepository) ExportUserData(ctx context.Context, id string) (*domain.UserDataExport, error) {
	user, err := r.GetUserByID(ctx, id)
	if err != nil || user == nil {
		return nil, err
	}
	result := &domain.UserDataExport{
		ExportedAt:      time.Now().UTC(),
		User:            *user,
		Subscriptions:   []domain.Subscription{},
		SupportRequests: []domain.SupportRequest{},
		AuditRecords:    []domain.PersonalAuditRecord{},
		AdminSessions:   []domain.PersonalAdminSession{},
		References:      []domain.PersonalDataReference{},
	}
	if err = r.db.SelectContext(ctx, &result.Subscriptions, `
		SELECT id, user_id, object_id, object_type, created_at, updated_at
		FROM subscriptions
		WHERE user_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export subscriptions for user %s: %w", id, err)
	}
	if err = r.db.SelectContext(ctx, &result.SupportRequests, `
		SELECT id, user_id, request_type, details, status, review_note,
			reviewed_by, reviewed_at, created_at, updated_at
		FROM support_requests
		WHERE user_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export support requests for user %s: %w", id, err)
	}
	if err = r.db.SelectContext(ctx, &result.AuditRecords, `
		SELECT id, actor_name, action, object_type, object_id, details,
		       ip_address, created_at
		FROM admin_audit_logs
		WHERE actor_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export audit records for user %s: %w", id, err)
	}
	if err = r.db.SelectContext(ctx, &result.AdminSessions, `
		SELECT name, auth_method, admin_role, expires_at, created_at, last_seen_at
		FROM admin_sessions
		WHERE admin_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export admin sessions for user %s: %w", id, err)
	}
	if err = r.db.SelectContext(ctx, &result.References, `
		SELECT category, object_id, relationship, created_at
		FROM (
			SELECT 'admin_audit' AS category, audit.id AS object_id,
				'actor_id' AS relationship, audit.created_at
			FROM admin_audit_logs audit WHERE audit.actor_id=$1
			UNION ALL
			SELECT 'admin_session', '', 'admin_id', session.created_at
			FROM admin_sessions session WHERE session.admin_id=$1
			UNION ALL
			SELECT 'chat_schedule_profile', profile.chat_id, 'configured_by', profile.created_at
			FROM chat_schedule_profiles profile WHERE profile.configured_by=$1
			UNION ALL
			SELECT 'lesson_override', override.id, 'created_by', override.created_at
			FROM lesson_overrides override WHERE override.created_by=$1
			UNION ALL
			SELECT 'support_review', request.id, 'reviewed_by', request.created_at
			FROM support_requests request WHERE request.reviewed_by=$1
			UNION ALL
			SELECT 'parser_snapshot_review', snapshot.id, 'reviewed_by', snapshot.created_at
			FROM parser_snapshots snapshot WHERE snapshot.reviewed_by=$1
			UNION ALL
			SELECT 'connector', connector.id, 'created_by', connector.created_at
			FROM connector_clients connector WHERE connector.created_by=$1
		) personal_references
		ORDER BY created_at, category`, id); err != nil {
		return nil, fmt.Errorf("export references for user %s: %w", id, err)
	}
	return result, nil
}
