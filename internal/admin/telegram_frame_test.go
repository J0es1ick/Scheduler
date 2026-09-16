package admin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

func TestTelegramFrameBrowser(t *testing.T) {
	if os.Getenv("SCHEDULER_BROWSER_TEST") != "1" {
		t.Skip("requires installed Playwright Chromium")
	}
	auth := NewAuthManager("synthetic-token", "", false, true)
	checker := &telegramAdminCheckerStub{isAdmin: true}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><body><main>Scheduler frame</main></body></html>"))
	})
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		identity, err := auth.IssueSession(w, AdminIdentity{ID: "42", AuthMethod: "telegram"})
		if err != nil {
			t.Error(err)
			http.Error(w, "login failed", 500)
			return
		}
		writeJSON(w, http.StatusOK, identity)
	})
	mux.Handle("GET /me", auth.Require(checker, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.Handle("POST /logout", auth.Require(checker, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.Logout(w, r); err != nil {
			t.Error(err)
			http.Error(w, "logout failed", 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	server := (&Server{}).securityHeaders(mux)
	fixture := httptest.NewTLSServer(server)
	defer fixture.Close()
	command := exec.Command("node", "../../web/admin/tests/telegram-frame.mjs")
	command.Env = append(os.Environ(), "SCHEDULER_FRAME_TEST_URL="+fixture.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Telegram frame browser: %v\n%s", err, output)
	}
	t.Log(string(output))
}
