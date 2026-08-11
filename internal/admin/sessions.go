package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) SaveAdminSession(
	ctx context.Context,
	tokenHash string,
	identity AdminIdentity,
	expires time.Time,
	maxSessions int,
) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at<=NOW()`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO admin_sessions (
			token_hash, admin_id, name, auth_method, admin_role, csrf_token, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		tokenHash, identity.ID, identity.Name, identity.AuthMethod, identity.Role, identity.CSRFToken, expires,
	); err != nil {
		return fmt.Errorf("insert admin session: %w", err)
	}
	if maxSessions > 0 {
		if _, err = tx.ExecContext(ctx, `
			DELETE FROM admin_sessions WHERE token_hash IN (
				SELECT token_hash FROM admin_sessions
				ORDER BY created_at DESC OFFSET $1
			)`, maxSessions); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AdminSession(ctx context.Context, tokenHash string) (AdminIdentity, time.Time, error) {
	var row struct {
		AdminIdentity
		Expires time.Time `db:"expires_at"`
	}
	err := s.db.GetContext(ctx, &row, `
		SELECT admin_id AS id, name, auth_method, admin_role AS role,
			csrf_token, expires_at
		FROM admin_sessions
		WHERE token_hash=$1 AND expires_at>NOW()`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminIdentity{}, time.Time{}, ErrUnauthorized
	}
	return row.AdminIdentity, row.Expires, err
}

func (s *Store) DeleteAdminSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash=$1`, tokenHash)
	return err
}

func (s *Store) PurgeAccessKeySessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE auth_method='access_key'`)
	return err
}
