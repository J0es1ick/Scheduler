package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
)

type UserRestrictionsPatch struct {
	BotBlocked     *bool `json:"bot_blocked"`
	SupportBlocked *bool `json:"support_blocked"`
}

func (s *Store) UpdateUserRestrictions(ctx context.Context, userID string, patch UserRestrictionsPatch, actor AdminIdentity, ip string) (domain.UserRestrictions, error) {
	var before domain.UserRestrictions
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return before, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "support:"+userID); err != nil {
		return before, err
	}
	if err = tx.GetContext(ctx, &before, `SELECT bot_blocked, support_blocked FROM users WHERE id=$1 FOR UPDATE`, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return before, ErrNotFound
		}
		return before, err
	}
	after := before
	if patch.BotBlocked != nil {
		after.BotBlocked = *patch.BotBlocked
	}
	if patch.SupportBlocked != nil {
		after.SupportBlocked = *patch.SupportBlocked
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET bot_blocked=$2, support_blocked=$3, updated_at=NOW() WHERE id=$1`, userID, after.BotBlocked, after.SupportBlocked); err != nil {
		return before, err
	}
	details, err := json.Marshal(map[string]any{"before": before, "after": after})
	if err != nil {
		return before, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_logs (id,actor_id,actor_name,action,object_type,object_id,details,ip_address,created_at)
		VALUES($1,$2,$3,'update_user_restrictions','user',$4,$5::jsonb,$6,NOW())`, uuid.NewString(), actor.ID, actor.Name, userID, details, ip); err != nil {
		return before, fmt.Errorf("audit user restrictions: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return before, err
	}
	return after, nil
}

func (s *Server) handleUserRestrictions(w http.ResponseWriter, r *http.Request) {
	var patch UserRestrictionsPatch
	if err := decodeJSON(w, r, &patch); err != nil || (patch.BotBlocked == nil && patch.SupportBlocked == nil) {
		writeAPIError(w, http.StatusBadRequest, "Укажите ограничение и его новое состояние")
		return
	}
	result, err := s.store.UpdateUserRestrictions(r.Context(), r.PathValue("id"), patch, identityFromContext(r.Context()), s.requestIP(r))
	if errors.Is(err, ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось изменить ограничения пользователя")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
