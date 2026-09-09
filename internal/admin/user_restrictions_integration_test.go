//go:build integration

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

func TestUserRestrictionAPIRequiresOwnerAndCSRF(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Connect("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	target, actor := "restricted-"+uuid.NewString(), "moderator-"+uuid.NewString()
	defer db.Exec(`DELETE FROM users WHERE id IN ($1,$2)`, target, actor)
	defer db.Exec(`DELETE FROM admin_audit_logs WHERE actor_id=$1`, actor)
	if _, err = db.Exec(`INSERT INTO users(id,is_admin,admin_role) VALUES($1,FALSE,'none'),($2,TRUE,'owner')`, target, actor); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	auth := NewAuthManager("", "", false, false)
	auth.UseSessionStore(store)
	server := &Server{store: store, auth: auth}
	mux := http.NewServeMux()
	server.protected(mux, "PATCH /api/users/{id}/restrictions", server.handleUserRestrictions)
	request := func(role, body string, csrf bool, id string) *httptest.ResponseRecorder {
		t.Helper()
		if _, err = db.Exec(`UPDATE users SET admin_role=$2 WHERE id=$1`, actor, role); err != nil {
			t.Fatal(err)
		}
		login := httptest.NewRecorder()
		identity, err := auth.IssueSession(login, AdminIdentity{ID: actor, Name: "Synthetic moderator", Role: role, AuthMethod: "telegram"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("PATCH", "/api/users/"+id+"/restrictions", strings.NewReader(body))
		req.AddCookie(login.Result().Cookies()[0])
		if csrf {
			req.Header.Set("X-CSRF-Token", identity.CSRFToken)
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		return response
	}
	for _, role := range []string{"read_only", "support", "editor", "reviewer", "operator"} {
		if response := request(role, `{"bot_blocked":true}`, true, target); response.Code != 403 {
			t.Fatalf("%s status=%d %s", role, response.Code, response.Body.String())
		}
	}
	if response := request("owner", `{"bot_blocked":true}`, false, target); response.Code != 403 {
		t.Fatalf("missing CSRF status=%d", response.Code)
	}
	for _, body := range []string{`{}`, `{"bot_blocked":"true"}`, `{"support_blocked":null}`, `{"bot_blocked":true,"unknown":1}`} {
		if response := request("owner", body, true, target); response.Code != 400 {
			t.Fatalf("invalid %s status=%d", body, response.Code)
		}
	}
	if response := request("owner", `{"bot_blocked":true}`, true, "missing-"+target); response.Code != 404 {
		t.Fatalf("missing user status=%d", response.Code)
	}
	for _, step := range []struct {
		body string
		want domain.UserRestrictions
	}{
		{`{"support_blocked":true}`, domain.UserRestrictions{SupportBlocked: true}},
		{`{"bot_blocked":true}`, domain.UserRestrictions{BotBlocked: true, SupportBlocked: true}},
		{`{"bot_blocked":false}`, domain.UserRestrictions{SupportBlocked: true}},
		{`{"support_blocked":false}`, domain.UserRestrictions{}},
	} {
		response := request("owner", step.body, true, target)
		if response.Code != 200 {
			t.Fatalf("status=%d %s", response.Code, response.Body.String())
		}
		var got domain.UserRestrictions
		if err = json.Unmarshal(response.Body.Bytes(), &got); err != nil || got != step.want {
			t.Fatalf("response=%+v %v", got, err)
		}
		items, err := store.Users(ctx, target, 10)
		if err != nil || len(items) != 1 || items[0].BotBlocked != got.BotBlocked || items[0].SupportBlocked != got.SupportBlocked {
			t.Fatalf("list=%+v %v", items, err)
		}
	}
	var auditCount int
	if err = db.Get(&auditCount, `SELECT count(*) FROM admin_audit_logs WHERE actor_id=$1 AND action='update_user_restrictions' AND details ? 'before' AND details ? 'after'`, actor); err != nil || auditCount != 4 {
		t.Fatalf("audit=%d %v", auditCount, err)
	}

	t.Run("audit failure rolls back restriction", func(t *testing.T) {
		name := "reject_restriction_audit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		quoted := pgx.Identifier{name}.Sanitize()
		if _, err = db.Exec(`CREATE FUNCTION ` + quoted + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='update_user_restrictions' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + quoted + ` BEFORE INSERT ON admin_audit_logs FOR EACH ROW EXECUTE FUNCTION ` + quoted + `() `); err != nil {
			t.Fatal(err)
		}
		defer db.Exec(`DROP TRIGGER ` + quoted + ` ON admin_audit_logs; DROP FUNCTION ` + quoted + `() `)
		value := true
		if _, err = store.UpdateUserRestrictions(ctx, target, UserRestrictionsPatch{BotBlocked: &value}, AdminIdentity{ID: actor}, ""); err == nil {
			t.Fatal("audit failure was ignored")
		}
		flags, err := repository.NewUserRepository(db).Restrictions(ctx, target)
		if err != nil || flags.BotBlocked {
			t.Fatalf("unaudited ban committed: %+v %v", flags, err)
		}
	})

	t.Run("submission waiting for ban cannot slip through", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		var pid int
		if err = tx.GetContext(ctx, &pid, `SELECT pg_backend_pid()`); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "support:"+target); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE users SET support_blocked=TRUE WHERE id=$1`, target); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			done <- repository.NewSupportRequestRepository(db).Create(ctx, "waiting-"+target, target, domain.SupportRequestFeedback, "Synthetic feedback concurrent with a restriction")
		}()
		for {
			var waiting bool
			if err = db.GetContext(ctx, &waiting, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid); err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			select {
			case err := <-done:
				t.Fatalf("submission did not wait: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(10 * time.Millisecond):
			}
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if err = <-done; !errors.Is(err, repository.ErrSupportRequestBlocked) {
			t.Fatalf("submission bypassed ban: %v", err)
		}
	})
}
