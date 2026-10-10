//go:build integration

package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPersonalAPIWithRuntimePrivileges(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Connect("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	roles := []string{"personal_bot_" + suffix, "personal_admin_" + suffix, "personal_parser_" + suffix, "personal_privacy_" + suffix}
	password := uuid.NewString()
	for _, role := range roles {
		if _, err = db.Exec(`CREATE ROLE ` + role + ` LOGIN PASSWORD '` + password + `'`); err != nil {
			t.Fatal(err)
		}
		defer func() { db.Exec(`DROP OWNED BY ` + role); db.Exec(`DROP ROLE ` + role) }()
	}
	if err = database.ApplyRuntimeGrants(ctx, db, roles[0], roles[1], roles[2], roles[3]); err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	dsn.User = url.UserPassword(roles[1], password)
	limited, err := sqlx.Connect("pgx", dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	user := strconv.FormatInt(time.Now().UnixNano()%1000000000000, 10)
	defer db.Exec(`DELETE FROM users WHERE id=$1`, user)
	defer db.Exec(`DELETE FROM universities WHERE id=$1`, suffix)
	for _, q := range []string{
		`INSERT INTO universities(id,name) VALUES($1,'Personal API')`,
		`INSERT INTO groups(id,university_id,name) VALUES($1,$1,'Group')`,
		`INSERT INTO semesters(id,external_id,university_id,name,start_date,end_date) VALUES($1,$1,$1,'Term','2026-09-01','2026-12-31')`,
		`INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type,room) VALUES($1,$1,$1,$1,1,'09:00','10:00','every','Physics','lecture','100')`,
	} {
		if _, err = db.Exec(q, suffix); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO subscriptions(id,user_id,object_type,object_id) VALUES($1,$2,'group',$1)`, suffix, user); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE users SET default_group_id=$1 WHERE id=$2`, suffix, user); err != nil {
		t.Fatal(err)
	}
	auth := NewAuthManager("synthetic-bot-token", "", false, false)
	auth.UseSessionStore(NewStore(limited))
	server, err := NewServer(NewStore(limited), auth, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	csrf := ""
	request := func(method, path, body string, token bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if token {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}
	raw := signedTelegramInitData(t, "synthetic-bot-token", time.Now(), fmt.Sprintf(`{"id":%s,"first_name":"Synthetic"}`, user))
	body, _ := json.Marshal(map[string]string{"init_data": raw})
	response := request("POST", "/api/personal/auth/telegram", string(body), false)
	if response.Code != 200 {
		t.Fatalf("ordinary login: %d %s", response.Code, response.Body)
	}
	var me struct {
		CSRF  string `json:"csrf_token"`
		Admin bool   `json:"is_admin"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Admin || me.CSRF == "" {
		t.Fatalf("profile: %+v", me)
	}
	csrf = me.CSRF
	cookie = response.Result().Cookies()[0]
	if cookie.Name == adminSessionCookie || !cookie.HttpOnly {
		t.Fatalf("cookie: %+v", cookie)
	}
	response = request("GET", "/api/personal/me", "", false)
	if response.Code != 200 {
		t.Fatalf("persisted login: %d %s", response.Code, response.Body)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("personal data is cacheable")
	}
	response = request("GET", "/api/users", "", false)
	if response.Code != 401 {
		t.Fatalf("personal session accessed admin: %d", response.Code)
	}
	response = request("GET", "/api/personal/schedule?target="+suffix+"&from=2026-09-07&to=2026-09-07", "", false)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Physics") {
		t.Fatalf("read: %d %s", response.Code, response.Body)
	}
	payload := fmt.Sprintf(`{"target_id":%q,"lesson_id":%q,"date":"2026-09-07","scope":"semester","patch":{"room":"Personal"}}`, suffix, suffix)
	response = request("POST", "/api/personal/changes", payload, false)
	if response.Code != 403 {
		t.Fatalf("missing csrf: %d", response.Code)
	}
	response = request("POST", "/api/personal/changes", payload, true)
	if response.Code != 200 {
		t.Fatalf("save under admin DB grants: %d %s", response.Code, response.Body)
	}
	response = request("GET", "/api/personal/schedule?target=foreign&from=2026-09-07&to=2026-09-07", "", false)
	if response.Code != 403 {
		t.Fatalf("foreign target: %d", response.Code)
	}
	dsn.User = url.UserPassword(roles[2], password)
	parserDB, err := sqlx.Connect("pgx", dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	defer parserDB.Close()
	if _, err = parserDB.Exec(`SELECT scheduler_personal_publication_changed($1,ARRAY[$1]::text[])`, suffix); err != nil {
		t.Fatal("parser review privilege", err)
	}
	response = request("GET", "/api/personal/schedule?target="+suffix+"&from=2026-09-07&to=2026-09-07", "", false)
	var personal service.PersonalSchedule
	if err = json.Unmarshal(response.Body.Bytes(), &personal); err != nil || personal.Review == nil {
		t.Fatal("review response", response.Body, err)
	}
	decision, _ := json.Marshal(service.PersonalReviewInput{TargetID: suffix, Publication: personal.Publication, Action: "keep", Versions: map[string]int64{personal.Review.Items[0].ID: personal.Review.Items[0].Version}})
	response = request("POST", "/api/personal/review", string(decision), false)
	if response.Code != 403 {
		t.Fatal("review without CSRF", response.Code)
	}
	response = request("POST", "/api/personal/review", string(decision), true)
	if response.Code != 200 {
		t.Fatal("review with runtime privileges", response.Code, response.Body)
	}
	response = request("POST", "/api/personal/review", string(decision), true)
	if response.Code != 409 {
		t.Fatal("duplicate review", response.Code)
	}
	users, err := NewStore(limited).Users(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range users {
		if item.ID == user {
			found = item.DefaultGroupUniversityName == "Personal API"
		}
	}
	if !found {
		t.Fatal("student university missing")
	}
	dsn.User = url.UserPassword(roles[0], password)
	botDB, err := sqlx.Connect("pgx", dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	defer botDB.Close()
	export, err := repository.NewUserRepository(botDB).ExportUserData(ctx, user)
	if err != nil || len(export.PersonalChanges) != 1 || len(export.PersonalSessions) != 1 {
		t.Fatalf("bot export privileges: %+v %v", export, err)
	}
	var writable bool
	if err = botDB.Get(&writable, `SELECT has_table_privilege(current_user,'personal_schedule_overrides','UPDATE')`); err != nil || writable {
		t.Fatalf("bot can edit: %t %v", writable, err)
	}
	if _, err = db.Exec(`UPDATE users SET is_admin=TRUE,admin_role='support' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	response = request("GET", "/api/personal/me", "", false)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"is_admin":true`) {
		t.Fatalf("admin choice: %d %s", response.Code, response.Body)
	}
	if _, err = db.Exec(`UPDATE users SET bot_blocked=TRUE WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	response = request("GET", "/api/personal/me", "", false)
	if response.Code != 403 {
		t.Fatalf("blocked session: %d", response.Code)
	}
	if _, err = db.Exec(`DELETE FROM users WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	response = request("GET", "/api/personal/me", "", false)
	if response.Code != 401 {
		t.Fatalf("deleted session: %d %s", response.Code, response.Body)
	}
}
