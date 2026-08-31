package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type ChatProfileRepository struct {
	db *sqlx.DB
}

func NewChatProfileRepository(db *sqlx.DB) *ChatProfileRepository {
	return &ChatProfileRepository{db: db}
}

func (r *ChatProfileRepository) Upsert(
	ctx context.Context,
	chatID string,
	title string,
	groupID string,
	configuredBy string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("upsert chat schedule profile: begin: %w", err)
	}
	defer tx.Rollback()
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return err
	}
	if err = lockActiveGroup(ctx, tx, groupID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO chat_schedule_profiles
			(chat_id, title, default_group_id, configured_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id) DO UPDATE SET
			title=EXCLUDED.title,
			default_group_id=EXCLUDED.default_group_id,
			configured_by=EXCLUDED.configured_by,
			updated_at=NOW()`,
		chatID, title, groupID, configuredBy,
	); err != nil {
		return fmt.Errorf("upsert chat schedule profile %s: %w", chatID, err)
	}
	return tx.Commit()
}

func (r *ChatProfileRepository) Get(
	ctx context.Context,
	chatID string,
) (*domain.ChatScheduleProfile, error) {
	var profile domain.ChatScheduleProfile
	err := r.db.GetContext(ctx, &profile, `
		SELECT p.chat_id, p.title, p.default_group_id,
			g.name AS group_name, g.university_id,
			u.name AS university_name, p.schedule_view_format, p.configured_by,
			p.created_at, p.updated_at, NOT (g.is_active AND u.is_active) AS unavailable
		FROM chat_schedule_profiles p
		JOIN groups g ON g.id=p.default_group_id
		JOIN universities u ON u.id=g.university_id
		WHERE p.chat_id=$1`, chatID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get chat schedule profile %s: %w", chatID, err)
	}
	return &profile, nil
}

func (r *ChatProfileRepository) SetScheduleView(
	ctx context.Context,
	chatID string,
	format domain.ScheduleViewFormat,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE chat_schedule_profiles
		SET schedule_view_format=$2, updated_at=NOW()
		WHERE chat_id=$1`, chatID, format)
	if err != nil {
		return fmt.Errorf("set chat schedule view %s: %w", chatID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ChatProfileRepository) Delete(ctx context.Context, chatID string) error {
	result, err := r.db.ExecContext(
		ctx,
		`DELETE FROM chat_schedule_profiles WHERE chat_id=$1`,
		chatID,
	)
	if err != nil {
		return fmt.Errorf("delete chat schedule profile %s: %w", chatID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
