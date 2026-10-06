package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

var ErrBroadcastConflict = errors.New("рассылка уже изменена или отправлена; обновите страницу")

type BroadcastRepository struct{ db *sqlx.DB }

func NewBroadcastRepository(db *sqlx.DB) *BroadcastRepository { return &BroadcastRepository{db: db} }

const broadcastColumns = `id,COALESCE(author_id,'') AS author_id,document,body,audience_mode,status,version,created_at,updated_at,sent_at,completed_at`
const attachmentColumns = `id,broadcast_id,filename,media_type,content_type,size,position,telegram_file_id`

type BroadcastInput struct {
	Document      json.RawMessage `json:"document"`
	AudienceMode  string          `json:"audience_mode"`
	UserIDs       []string        `json:"user_ids"`
	AttachmentIDs []string        `json:"attachment_ids"`
	Version       int             `json:"version"`
}

type BroadcastAudience struct {
	Recipients []domain.BroadcastRecipient `json:"recipients"`
	Eligible   int                         `json:"eligible"`
	Total      int                         `json:"total"`
}

func (r *BroadcastRepository) Create(ctx context.Context, authorID string) (string, error) {
	id := uuid.NewString()
	_, err := r.db.ExecContext(ctx, `INSERT INTO broadcasts(id,author_id) VALUES($1,(SELECT id FROM users WHERE id=$2))`, id, authorID)
	return id, err
}

func (r *BroadcastRepository) List(ctx context.Context, offset int) ([]domain.Broadcast, error) {
	items := []domain.Broadcast{}
	err := r.db.SelectContext(ctx, &items, `SELECT `+broadcastColumns+` FROM broadcasts ORDER BY created_at DESC,id LIMIT 50 OFFSET $1`, offset)
	return items, err
}

func (r *BroadcastRepository) Get(ctx context.Context, id string) (*domain.Broadcast, error) {
	var b domain.Broadcast
	if err := r.db.GetContext(ctx, &b, `SELECT `+broadcastColumns+` FROM broadcasts WHERE id=$1`, id); err != nil {
		return nil, err
	}
	b.Attachments = []domain.BroadcastAttachment{}
	if err := r.db.SelectContext(ctx, &b.Attachments, `SELECT `+attachmentColumns+` FROM broadcast_attachments WHERE broadcast_id=$1 ORDER BY position`, id); err != nil {
		return nil, err
	}
	b.Recipients = []domain.BroadcastRecipient{}
	if err := r.db.SelectContext(ctx, &b.Recipients, `SELECT r.user_id,COALESCE(u.username,'') AS username,
		CASE WHEN o.status='delivered' THEN 'delivered' WHEN COALESCE(jsonb_array_length(o.broadcast_message_ids),0)>0 AND o.status<>'pending' THEN 'partial'
		WHEN o.status='pending' THEN 'pending' WHEN o.status='failed' THEN 'failed' WHEN o.status='cancelled' THEN 'skipped'
		WHEN u.bot_blocked THEN 'blocked' WHEN u.service_updates_consent IS NOT TRUE THEN 'no_consent' ELSE 'ready' END AS status,
		COALESCE(NULLIF(o.last_error,''),o.cancel_reason,'') AS reason,COALESCE(jsonb_array_length(o.broadcast_message_ids),0) AS parts
		FROM broadcast_recipients r JOIN users u ON u.id=r.user_id LEFT JOIN bot_outbox o ON o.broadcast_id=r.broadcast_id AND o.user_id=r.user_id
		WHERE r.broadcast_id=$1 ORDER BY r.user_id`, id); err != nil {
		return nil, err
	}
	b.Counts = map[string]int{}
	for _, recipient := range b.Recipients {
		b.Counts[recipient.Status]++
	}
	return &b, nil
}

func NormalizeBroadcastUsernames(input string) ([]string, error) {
	parts := strings.FieldsFunc(input, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == ';' })
	if len(parts) > 1000 {
		return nil, fmt.Errorf("можно указать не более 1000 ников")
	}
	seen := map[string]bool{}
	result := []string{}
	for _, part := range parts {
		name := strings.ToLower(strings.TrimPrefix(part, "@"))
		if len(name) < 1 || len(name) > 32 {
			return nil, fmt.Errorf("неверный ник: %s", part)
		}
		for _, c := range name {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
				return nil, fmt.Errorf("неверный ник: %s", part)
			}
		}
		if !seen[name] {
			result = append(result, name)
			seen[name] = true
		}
	}
	return result, nil
}

func (r *BroadcastRepository) Audience(ctx context.Context, mode, input string) (BroadcastAudience, error) {
	result := BroadcastAudience{Recipients: []domain.BroadcastRecipient{}}
	if mode == "all" {
		err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE service_updates_consent IS TRUE AND NOT bot_blocked) FROM users`).Scan(&result.Total, &result.Eligible)
		return result, err
	}
	if mode != "selected" {
		return result, fmt.Errorf("неверный режим получателей")
	}
	names, err := NormalizeBroadcastUsernames(input)
	if err != nil {
		return result, err
	}
	for _, name := range names {
		matches := []domain.BroadcastRecipient{}
		err = r.db.SelectContext(ctx, &matches, `SELECT id AS user_id,COALESCE(username,'') AS username,
			CASE WHEN bot_blocked THEN 'blocked' WHEN service_updates_consent IS NOT TRUE THEN 'no_consent' ELSE 'ready' END AS status
			FROM users WHERE lower(username)=$1 ORDER BY id`, name)
		if err != nil {
			return result, err
		}
		if len(matches) == 0 {
			matches = append(matches, domain.BroadcastRecipient{Username: name, Status: "unknown"})
		}
		if len(matches) > 1 {
			for i := range matches {
				matches[i].Reason = "ambiguous"
			}
		}
		for _, m := range matches {
			result.Total++
			if m.Status == "ready" {
				result.Eligible++
			}
			result.Recipients = append(result.Recipients, m)
		}
	}
	return result, nil
}

func lockBroadcast(ctx context.Context, tx *sqlx.Tx, id string, version int) (domain.Broadcast, error) {
	var b domain.Broadcast
	err := tx.GetContext(ctx, &b, `SELECT `+broadcastColumns+` FROM broadcasts WHERE id=$1 FOR UPDATE`, id)
	if err == nil && (b.Status != "draft" || b.Version != version) {
		err = ErrBroadcastConflict
	}
	return b, err
}

func (r *BroadcastRepository) Save(ctx context.Context, id string, input BroadcastInput) error {
	body, err := domain.RenderBroadcastDocument(input.Document)
	if err != nil {
		return err
	}
	if input.AudienceMode != "all" && input.AudienceMode != "selected" {
		return fmt.Errorf("неверный режим получателей")
	}
	if len(input.UserIDs) > 1000 {
		return fmt.Errorf("слишком много получателей")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = lockBroadcast(ctx, tx, id, input.Version); err != nil {
		return err
	}
	var attachments []string
	if err = tx.SelectContext(ctx, &attachments, `SELECT id FROM broadcast_attachments WHERE broadcast_id=$1 ORDER BY position`, id); err != nil {
		return err
	}
	if len(attachments) != len(input.AttachmentIDs) {
		return ErrBroadcastConflict
	}
	set := map[string]bool{}
	for _, a := range attachments {
		set[a] = true
	}
	for _, a := range input.AttachmentIDs {
		if !set[a] {
			return ErrBroadcastConflict
		}
		delete(set, a)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE broadcast_attachments SET position=-position-1 WHERE broadcast_id=$1`, id); err != nil {
		return err
	}
	for index, a := range input.AttachmentIDs {
		if _, err = tx.ExecContext(ctx, `UPDATE broadcast_attachments SET position=$2 WHERE id=$1`, a, index); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM broadcast_recipients WHERE broadcast_id=$1`, id); err != nil {
		return err
	}
	if input.AudienceMode == "selected" {
		for _, userID := range input.UserIDs {
			if _, err = tx.ExecContext(ctx, `INSERT INTO broadcast_recipients(broadcast_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, userID); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE broadcasts SET document=$2,body=$3,audience_mode=$4,version=version+1,updated_at=NOW() WHERE id=$1`, id, input.Document, body, input.AudienceMode); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *BroadcastRepository) AddAttachment(ctx context.Context, id string, version int, a domain.BroadcastAttachment) error {
	if len(a.Data) == 0 || len(a.Data) > 10000000 {
		return fmt.Errorf("размер вложения должен быть от 1 байта до 10 МБ")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = lockBroadcast(ctx, tx, id, version); err != nil {
		return err
	}
	var count, total, position int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(size),0),COALESCE(MAX(position),-1)+1 FROM broadcast_attachments WHERE broadcast_id=$1`, id).Scan(&count, &total, &position); err != nil {
		return err
	}
	if count >= 10 || total+len(a.Data) > 50000000 {
		return fmt.Errorf("допускается до 10 вложений и до 50 МБ суммарно")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO broadcast_attachments(id,broadcast_id,filename,media_type,content_type,size,data,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.NewString(), id, a.Filename, a.MediaType, a.ContentType, len(a.Data), a.Data, position)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE broadcasts SET version=version+1,updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *BroadcastRepository) RemoveAttachment(ctx context.Context, id, attachment string, version int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = lockBroadcast(ctx, tx, id, version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM broadcast_attachments WHERE id=$1 AND broadcast_id=$2`, attachment, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE broadcasts SET version=version+1,updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *BroadcastRepository) Attachment(ctx context.Context, id, attachment string) (domain.BroadcastAttachment, error) {
	var a domain.BroadcastAttachment
	err := r.db.GetContext(ctx, &a, `SELECT `+attachmentColumns+`,data FROM broadcast_attachments WHERE broadcast_id=$1 AND id=$2`, id, attachment)
	return a, err
}

func (r *BroadcastRepository) Send(ctx context.Context, id, key string, version int) error {
	if _, err := uuid.Parse(key); err != nil {
		return fmt.Errorf("неверный ключ отправки")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var b domain.Broadcast
	var existingKey sql.NullString
	if err = tx.GetContext(ctx, &b, `SELECT `+broadcastColumns+` FROM broadcasts WHERE id=$1 FOR UPDATE`, id); err != nil {
		return err
	}
	if err = tx.GetContext(ctx, &existingKey, `SELECT send_key FROM broadcasts WHERE id=$1`, id); err != nil {
		return err
	}
	if existingKey.Valid && existingKey.String == key {
		return nil
	}
	if b.Status != "draft" || b.Version != version {
		return ErrBroadcastConflict
	}
	if _, err = domain.RenderBroadcastDocument(b.Document); err != nil {
		return err
	}
	if b.AudienceMode == "all" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO broadcast_recipients(broadcast_id,user_id) SELECT $1,id FROM users WHERE service_updates_consent IS TRUE AND NOT bot_blocked ON CONFLICT DO NOTHING`, id); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO bot_outbox(id,user_id,kind,body,broadcast_id)
		SELECT 'broadcast:'||r.broadcast_id||':'||r.user_id,r.user_id,'service_update',$2,r.broadcast_id
		FROM broadcast_recipients r JOIN users u ON u.id=r.user_id WHERE r.broadcast_id=$1 AND u.service_updates_consent IS TRUE AND NOT u.bot_blocked
		ON CONFLICT DO NOTHING`, id, b.Body)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("нет получателей, согласившихся на рассылку")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE broadcasts SET status='sending',send_key=$2,sent_at=NOW(),updated_at=NOW(),version=version+1 WHERE id=$1`, id, key); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_notify('scheduler_outbox_ready','')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *BroadcastRepository) Stop(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE broadcasts SET status='cancelled',completed_at=NOW(),updated_at=NOW(),version=version+1 WHERE id=$1 AND status='sending'`, id)
	return err
}

func (r *BroadcastRepository) DeleteDraft(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM broadcasts WHERE id=$1 AND status='draft'`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrBroadcastConflict
	}
	return err
}
