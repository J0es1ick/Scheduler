//go:build integration

package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestLogoutRevokesEveryRoleCookie(t *testing.T) {
	db, err := sqlx.Connect("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	for _, role := range []string{"read_only", "support", "editor", "reviewer", "operator", "owner"} {
		t.Run(role, func(t *testing.T) {
			id := "logout-" + uuid.NewString()
			if _, err := db.Exec(`INSERT INTO users(id,username,is_admin,admin_role) VALUES($1,'Synthetic admin',TRUE,$2)`, id, role); err != nil {
				t.Fatal(err)
			}
			defer db.Exec(`DELETE FROM users WHERE id=$1`, id)
			defer db.Exec(`DELETE FROM admin_audit_logs WHERE actor_id=$1`, id)
			auth := NewAuthManager("", "", false, false)
			auth.UseSessionStore(store)
			login := httptest.NewRecorder()
			identity, err := auth.IssueSession(login, AdminIdentity{ID: id, Name: "Synthetic admin", AuthMethod: "telegram", Role: role})
			if err != nil {
				t.Fatal(err)
			}
			cookie := login.Result().Cookies()[0]
			server := &Server{store: store, auth: auth}
			mux := http.NewServeMux()
			server.protected(mux, "POST /api/auth/logout", server.handleLogout)
			for _, validCSRF := range []bool{false, true} {
				request := httptest.NewRequest("POST", "/api/auth/logout", nil)
				request.AddCookie(cookie)
				if validCSRF {
					request.Header.Set("X-CSRF-Token", identity.CSRFToken)
				}
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, request)
				if !validCSRF {
					if response.Code != 403 {
						t.Fatalf("invalid CSRF status=%d", response.Code)
					}
					continue
				}
				if response.Code != http.StatusNoContent {
					t.Fatalf("logout status=%d body=%s", response.Code, response.Body.String())
				}
			}
			request := httptest.NewRequest("GET", "/api/auth/me", nil)
			request.AddCookie(cookie)
			if _, err := auth.identityForRequest(request); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("old cookie remains valid: %v", err)
			}
		})
	}
}

type unavailableSessionStore struct{ adminSessionStore }

func (unavailableSessionStore) AdminSession(context.Context, string) (AdminIdentity, time.Time, error) {
	return AdminIdentity{}, time.Time{}, errors.New("database unavailable")
}
func TestUnavailableSessionStoreIsNotUnauthorized(t *testing.T) {
	auth := NewAuthManager("", "", false, false)
	auth.UseSessionStore(unavailableSessionStore{})
	request := httptest.NewRequest("GET", "/api/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "synthetic-cookie"})
	response := httptest.NewRecorder()
	auth.Require(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unverified request allowed") })).ServeHTTP(response, request)
	if response.Code != 503 {
		t.Fatalf("database outage misclassified as status %d", response.Code)
	}
}
