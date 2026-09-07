//go:build integration

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestAssignRevokeAndDeleteLeavesNoUserIdentifiers(t *testing.T) {
	db, err := sqlx.Connect("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	target := strconv.FormatInt(time.Now().UnixNano(), 10)
	ownerA := "owner-a-" + uuid.NewString()
	ownerB := "owner-b-" + uuid.NewString()
	defer db.Exec(`DELETE FROM admin_audit_logs WHERE actor_id IN ($1,$2)`, ownerA, ownerB)
	defer db.Exec(`DELETE FROM users WHERE id IN ($1,$2,$3)`, target, ownerA, ownerB)
	for _, id := range []string{target, ownerA, ownerB} {
		role := "owner"
		if id == target {
			role = "none"
		}
		if _, err = db.Exec(`INSERT INTO users(id,is_admin,admin_role) VALUES($1,$2,$3)`, id, role != "none", role); err != nil {
			t.Fatal(err)
		}
	}
	store := NewStore(db)
	auth := NewAuthManager("", "", false, false)
	auth.UseSessionStore(store)
	server := &Server{store: store, auth: auth}
	mux := http.NewServeMux()
	server.protected(mux, "PATCH /api/users/{id}", server.handleUpdateUser)
	for i, owner := range []string{ownerA, ownerB} {
		role := "editor"
		if i == 1 {
			role = "none"
		}
		login := httptest.NewRecorder()
		identity, issueErr := auth.IssueSession(login, AdminIdentity{ID: owner, Name: "Synthetic owner", Role: "owner", AuthMethod: "telegram"})
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		request := httptest.NewRequest("PATCH", "/api/users/"+target, strings.NewReader(`{"admin_role":"`+role+`"}`))
		request.AddCookie(login.Result().Cookies()[0])
		request.Header.Set("X-CSRF-Token", identity.CSRFToken)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("role change status=%d body=%s", response.Code, response.Body.String())
		}
		if i == 0 && repository.NewUserRepository(db).DeleteUser(ctx, target) == nil {
			t.Fatal("active administrator was deleted")
		}
	}
	if err = repository.NewUserRepository(db).DeleteUser(ctx, target); err != nil {
		t.Fatal(err)
	}
	var leftovers, markers int
	if err = db.Get(&leftovers, `SELECT count(*) FROM admin_audit_logs WHERE actor_id IN ($1,$2) AND (object_id=$3 OR details::text LIKE '%'||$3||'%')`, ownerA, ownerB, target); err != nil || leftovers != 0 {
		t.Fatalf("identifiers remain: %d %v", leftovers, err)
	}
	if err = db.Get(&markers, `SELECT count(DISTINCT object_id) FROM admin_audit_logs WHERE actor_id IN ($1,$2) AND action='update_admin_role' AND object_id LIKE 'deleted:%'`, ownerA, ownerB); err != nil || markers != 1 {
		t.Fatalf("deletion marker not shared: %d %v", markers, err)
	}
}
