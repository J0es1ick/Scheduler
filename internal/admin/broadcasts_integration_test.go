//go:build integration

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestBroadcastAPIConsentPermissionsAndAttachments(t *testing.T) {
	db, err := sqlx.Connect("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	actor, recipient, declined := uuid.NewString(), uuid.NewString(), uuid.NewString()
	defer db.Exec(`DELETE FROM users WHERE id IN ($1,$2,$3)`, actor, recipient, declined)
	defer db.Exec(`DELETE FROM admin_audit_logs WHERE actor_id=$1`, actor)
	if _, err = db.Exec(`INSERT INTO users(id,is_admin,admin_role,service_updates_consent) VALUES($1,TRUE,'owner',FALSE),($2,FALSE,'none',TRUE),($3,FALSE,'none',FALSE)`, actor, recipient, declined); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	auth := NewAuthManager("", "", false, false)
	auth.UseSessionStore(store)
	server, err := NewServer(store, auth, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	var csrf string
	login := func(role string) {
		t.Helper()
		if _, err = db.Exec(`UPDATE users SET admin_role=$2 WHERE id=$1`, actor, role); err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		identity, err := auth.IssueSession(response, AdminIdentity{ID: actor, Name: "Synthetic author", Role: role, AuthMethod: "telegram"})
		if err != nil {
			t.Fatal(err)
		}
		cookie = response.Result().Cookies()[0]
		csrf = identity.CSRFToken
	}
	request := func(method, path string, body []byte, contentType string, withCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.AddCookie(cookie)
		req.Header.Set("Content-Type", contentType)
		if withCSRF {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, req)
		return response
	}
	routes := []string{"GET /api/broadcasts", "POST /api/broadcasts", "POST /api/broadcasts/audience", "GET /api/broadcasts/test", "PUT /api/broadcasts/test", "DELETE /api/broadcasts/test", "POST /api/broadcasts/test/preview", "POST /api/broadcasts/test/send", "POST /api/broadcasts/test/stop", "POST /api/broadcasts/test/attachments", "GET /api/broadcasts/test/attachments/file", "DELETE /api/broadcasts/test/attachments/file"}
	for _, role := range []string{"read_only", "support", "editor", "reviewer", "operator"} {
		login(role)
		for _, route := range routes {
			method, path, _ := strings.Cut(route, " ")
			response := request(method, path, []byte(`{}`), "application/json", true)
			if response.Code != 403 {
				t.Fatalf("%s %s: %d %s", role, route, response.Code, response.Body.String())
			}
		}
	}
	login("owner")
	for _, route := range routes {
		method, path, _ := strings.Cut(route, " ")
		if method == "GET" {
			continue
		}
		if response := request(method, path, []byte(`{}`), "application/json", false); response.Code != 403 {
			t.Fatalf("CSRF %s: %d", route, response.Code)
		}
	}
	response := request("POST", "/api/broadcasts", nil, "application/json", true)
	if response.Code != 201 {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM broadcasts WHERE id=$1`, created.ID)
	base := "/api/broadcasts/" + created.ID
	upload := func(kind string, version int, data []byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "<test>.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		return request("POST", fmt.Sprintf("%s/attachments?type=%s&version=%d", base, kind, version), body.Bytes(), writer.FormDataContentType(), true)
	}
	if response = upload("photo", 1, []byte("not an image")); response.Code != 400 {
		t.Fatalf("invalid photo: %d", response.Code)
	}
	if response = upload("document", 1, bytes.Repeat([]byte("a"), 10000001)); response.Code != 400 {
		t.Fatalf("oversized file: %d", response.Code)
	}
	response = upload("document", 1, []byte("<script>alert(1)</script>"))
	if response.Code != 200 {
		t.Fatalf("upload: %d %s", response.Code, response.Body.String())
	}
	var b domain.Broadcast
	if err = json.Unmarshal(response.Body.Bytes(), &b); err != nil || len(b.Attachments) != 1 {
		t.Fatalf("upload body: %+v %v", b, err)
	}
	attachment := b.Attachments[0].ID
	response = request("GET", base+"/attachments/"+attachment, nil, "", true)
	if response.Code != 200 || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "attachment;") || response.Header().Get("Content-Type") != "application/octet-stream" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe document response: %d %+v", response.Code, response.Header())
	}
	response = request("GET", "/api/broadcasts/other/attachments/"+attachment, nil, "", true)
	if response.Code != 404 {
		t.Fatalf("cross-broadcast file: %d", response.Code)
	}
	payload := fmt.Sprintf(`{"version":2,"audience_mode":"selected","user_ids":[%q,%q],"attachment_ids":[%q],"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello <world> 🎉","marks":[{"type":"bold"}]}]}]}}`, recipient, declined, attachment)
	response = request("PUT", base, []byte(payload), "application/json", true)
	if response.Code != 200 {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	if response = request("PUT", base, []byte(payload), "application/json", true); response.Code != 409 {
		t.Fatalf("stale save: %d", response.Code)
	}
	response = request("POST", base+"/preview", nil, "application/json", true)
	var preview struct {
		Body     string `json:"body"`
		Eligible int    `json:"eligible"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &preview); err != nil || response.Code != 200 || preview.Eligible != 1 || preview.Body != "<b>Hello &lt;world&gt; 🎉</b>" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	send := []byte(fmt.Sprintf(`{"version":3,"key":%q}`, uuid.NewString()))
	for range 2 {
		response = request("POST", base+"/send", send, "application/json", true)
		if response.Code != 200 {
			t.Fatalf("send: %d %s", response.Code, response.Body.String())
		}
	}
	var count int
	if err = db.Get(&count, `SELECT COUNT(*) FROM bot_outbox WHERE broadcast_id=$1 AND user_id=$2`, created.ID, recipient); err != nil || count != 1 {
		t.Fatalf("delivery count: %d %v", count, err)
	}
	if err = db.Get(&count, `SELECT COUNT(*) FROM bot_outbox WHERE broadcast_id=$1 AND user_id=$2`, created.ID, declined); err != nil || count != 0 {
		t.Fatalf("refusal ignored: %d %v", count, err)
	}
	if response = upload("document", 4, []byte("late file")); response.Code != 409 {
		t.Fatalf("sent draft changed: %d", response.Code)
	}
	response = request("POST", base+"/stop", nil, "application/json", true)
	if response.Code != 200 {
		t.Fatalf("stop: %d %s", response.Code, response.Body.String())
	}
	var decision string
	if err = db.Get(&decision, `SELECT policy_decision FROM notification_queue_eligibility WHERE id=$1`, "broadcast:"+created.ID+":"+recipient); err != nil || decision != "cancel" {
		t.Fatalf("stop decision: %s %v", decision, err)
	}
}
