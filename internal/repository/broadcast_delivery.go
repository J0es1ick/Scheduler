package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func (r *NotificationRepository) BroadcastAttachments(ctx context.Context, id string) ([]domain.BroadcastAttachment, error) {
	items := []domain.BroadcastAttachment{}
	err := r.db.SelectContext(ctx, &items, `SELECT `+attachmentColumns+` FROM broadcast_attachments WHERE broadcast_id=$1 ORDER BY position`, id)
	return items, err
}

func (r *NotificationRepository) BroadcastAttachmentData(ctx context.Context, id string) ([]byte, error) {
	var data []byte
	err := r.db.GetContext(ctx, &data, `SELECT data FROM broadcast_attachments WHERE id=$1`, id)
	return data, err
}

func (r *NotificationRepository) SaveBroadcastPart(ctx context.Context, id, claim string, part, messageID int, attachmentID, fileID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var messages json.RawMessage
	err = tx.GetContext(ctx, &messages, `SELECT broadcast_message_ids FROM bot_outbox WHERE id=$1 AND claim_token=$2 AND status='pending' AND lease_expires_at>clock_timestamp() FOR UPDATE`, id, claim)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationClaimLost, err)
	}
	var ids []int
	if err = json.Unmarshal(messages, &ids); err != nil {
		return err
	}
	if len(ids) != part {
		return fmt.Errorf("unexpected broadcast progress")
	}
	ids = append(ids, messageID)
	messages, err = json.Marshal(ids)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bot_outbox SET broadcast_message_ids=$3,updated_at=NOW() WHERE id=$1 AND claim_token=$2`, id, claim, messages); err != nil {
		return err
	}
	if attachmentID != "" && fileID != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE broadcast_attachments SET telegram_file_id=$2 WHERE id=$1`, attachmentID, fileID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *NotificationRepository) CompleteBroadcasts(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE broadcasts b SET status='completed',completed_at=NOW(),updated_at=NOW()
		WHERE b.status='sending' AND NOT EXISTS(SELECT 1 FROM bot_outbox o WHERE o.broadcast_id=b.id AND o.status='pending')`)
	return err
}

func (r *NotificationRepository) ListenOutbox(ctx context.Context, wake chan<- struct{}) error {
	borrowed, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	var config *pgx.ConnConfig
	err = borrowed.Raw(func(raw any) error {
		pg, ok := raw.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("outbox notifications require pgx")
		}
		config = pg.Conn().Config().Copy()
		return nil
	})
	_ = borrowed.Close()
	if err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err = conn.Exec(ctx, `LISTEN scheduler_outbox_ready`); err != nil {
		return err
	}
	select {
	case wake <- struct{}{}:
	default:
	}
	for {
		if _, err = conn.WaitForNotification(ctx); err != nil {
			return err
		}
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}
