package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
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
	query := `INSERT INTO users (id, username, created_at, updated_at) VALUES ($1, $2, $3, $4)`
	args := []any{id, username, now, now}
	if isAdmin {
		query = `INSERT INTO users (id, username, created_at, updated_at, is_admin, admin_role) VALUES ($1,$2,$3,$4,TRUE,'owner')`
	}
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		`SELECT id, COALESCE(username, '') AS username, is_admin,
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled, bot_blocked, support_blocked,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			search_schedule_view_format,
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
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled, bot_blocked, support_blocked,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			search_schedule_view_format,
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
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled, bot_blocked, support_blocked,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			search_schedule_view_format,
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
		WHERE NOT bot_blocked AND ($1 = '' OR id > $1)
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

func (r *UserRepository) UpdateUsername(ctx context.Context, id, username string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username=$1, updated_at=$2 WHERE id=$3`,
		username, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update username for user %s: %w", id, err)
	}
	return nil
}

func (r *UserRepository) SetDefaultGroup(ctx context.Context, userID, groupID string) error {
	if groupID != "" {
		return NewSubscriptionRepository(r.db).SetDefaultSubscribedGroup(ctx, userID, groupID)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set default group for user %s: begin: %w", userID, err)
	}
	defer tx.Rollback()
	var previous sql.NullString
	if err = tx.GetContext(ctx, &previous, `
		SELECT default_group_id FROM users WHERE id=$1 FOR UPDATE`, userID); errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	} else if err != nil {
		return fmt.Errorf("set default group for user %s: load current group: %w", userID, err)
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE users SET default_group_id = NULL, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("set default group for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if previous.Valid {
		if _, err = tx.ExecContext(ctx, `
			SELECT scheduler_request_outbox_cancellation(
				$1, 'lesson_reminder', $2, 'default_group_changed'
			)`, userID, previous.String); err != nil {
			return fmt.Errorf("set default group for user %s: cancel reminders: %w", userID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("set default group for user %s: commit: %w", userID, err)
	}
	return nil
}

func (r *UserRepository) SetNotificationsEnabled(ctx context.Context, userID string, enabled bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set notifications for user %s: begin: %w", userID, err)
	}
	defer tx.Rollback()
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return err
	}
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
			SELECT scheduler_request_notification_cancellation(
				$1, NULL, 'notifications_disabled'
			)`, userID); err != nil {
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
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return err
	}
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
			SELECT scheduler_request_outbox_cancellation(
				$1, 'lesson_reminder', NULL, 'reminder_disabled'
			)`,
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

func (r *UserRepository) SetSearchScheduleView(
	ctx context.Context,
	userID string,
	format domain.ScheduleViewFormat,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET search_schedule_view_format=$1, updated_at=NOW()
		WHERE id=$2`, format, userID)
	if err != nil {
		return fmt.Errorf("set search schedule view for user %s: %w", userID, err)
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
	var deleted bool
	if err := r.db.GetContext(ctx, &deleted, `SELECT execute_privacy_deletion($1, $2)`, id, anonymizedID); err != nil {
		return fmt.Errorf("delete user %s: %w", id, err)
	}
	if !deleted {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) EnqueuePrivacyDeletion(ctx context.Context, id string) (string, error) {
	return enqueuePrivacyDeletion(ctx, r.db, id)
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
		SELECT id, user_id, object_id, object_type, schedule_view_format, subgroup,
			created_at, updated_at
		FROM subscriptions
		WHERE user_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export subscriptions for user %s: %w", id, err)
	}
	if err = r.db.SelectContext(ctx, &result.SupportRequests, `
		SELECT id, user_id, request_type, details, status, review_note,
			CASE WHEN reviewed_by=$1 THEN reviewed_by ELSE '' END AS reviewed_by, reviewed_at, created_at, updated_at
		FROM support_requests
		WHERE user_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export support requests for user %s: %w", id, err)
	}
	if err = r.db.SelectContext(ctx, &result.AuditRecords, `
		SELECT id, CASE WHEN actor_id=$1 THEN actor_name ELSE '' END AS actor_name,
               action, object_type,
               CASE WHEN object_type='user' AND object_id<>$1 THEN '' ELSE object_id END AS object_id,
               jsonb_strip_nulls(jsonb_build_object('role',details->'role','admin_role',details->'admin_role','status',details->'status')) AS details,
               CASE WHEN actor_id=$1 THEN ip_address ELSE '' END AS ip_address, created_at
        FROM admin_audit_logs
        WHERE actor_id=$1 OR (object_type='user' AND object_id=$1)
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
