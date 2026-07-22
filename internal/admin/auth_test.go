package admin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestValidateTelegramInitData(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	const botToken = "123456:test-token"
	raw := signedTelegramInitData(t, botToken, now, `{"id":42,"username":"scheduler_admin","first_name":"Alex"}`)

	user, err := validateTelegramInitData(raw, botToken, now)
	if err != nil {
		t.Fatalf("validate signed init data: %v", err)
	}
	if user.ID != 42 || user.Username != "scheduler_admin" {
		t.Fatalf("unexpected user: %+v", user)
	}

	if _, err = validateTelegramInitData(strings.Replace(raw, "scheduler_admin", "intruder", 1), botToken, now); err == nil {
		t.Fatal("tampered init data must be rejected")
	}
	expired := signedTelegramInitData(t, botToken, now.Add(-16*time.Minute), `{"id":42}`)
	if _, err = validateTelegramInitData(expired, botToken, now); err == nil {
		t.Fatal("expired init data must be rejected")
	}
}

func TestAccessKeyAndSessionCSRF(t *testing.T) {
	auth := NewAuthManager("bot-token", "correct-horse", "https://admin.example.test")
	if _, err := auth.LoginWithAccessKey("wrong"); err != ErrUnauthorized {
		t.Fatalf("wrong access key error = %v", err)
	}
	identity, err := auth.LoginWithAccessKey("correct-horse")
	if err != nil {
		t.Fatalf("login with access key: %v", err)
	}

	recorder := httptest.NewRecorder()
	identity, err = auth.IssueSession(recorder, identity)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	response := recorder.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected session cookie: %+v", cookies)
	}

	protected := auth.Require(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := identityFromContext(r.Context()).ID; got != identity.ID {
			t.Errorf("identity in context = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	getRequest := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	getRequest.AddCookie(cookies[0])
	getRecorder := httptest.NewRecorder()
	protected.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusNoContent {
		t.Fatalf("authorized GET status = %d", getRecorder.Code)
	}

	postRequest := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	postRequest.AddCookie(cookies[0])
	postRecorder := httptest.NewRecorder()
	protected.ServeHTTP(postRecorder, postRequest)
	if postRecorder.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF status = %d", postRecorder.Code)
	}

	postRequest = httptest.NewRequest(http.MethodPost, "/api/test", nil)
	postRequest.AddCookie(cookies[0])
	postRequest.Header.Set("X-CSRF-Token", identity.CSRFToken)
	postRecorder = httptest.NewRecorder()
	protected.ServeHTTP(postRecorder, postRequest)
	if postRecorder.Code != http.StatusNoContent {
		t.Fatalf("POST with CSRF status = %d", postRecorder.Code)
	}
}

type telegramAdminCheckerStub struct {
	isAdmin bool
}

func (s *telegramAdminCheckerStub) TelegramAdmin(_ context.Context, id string) (*UserView, error) {
	return &UserView{ID: id, IsAdmin: s.isAdmin}, nil
}

func TestTelegramSessionRechecksAdminRole(t *testing.T) {
	auth := NewAuthManager("bot-token", "", "https://admin.example.test")
	checker := &telegramAdminCheckerStub{isAdmin: true}
	recorder := httptest.NewRecorder()
	_, err := auth.IssueSession(recorder, AdminIdentity{ID: "42", Name: "@admin", AuthMethod: "telegram"})
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	cookie := recorder.Result().Cookies()[0]
	protected := auth.Require(checker, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("active admin status = %d", response.Code)
	}

	checker.isAdmin = false
	request = httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("revoked admin status = %d", response.Code)
	}
	if cookies := response.Result().Cookies(); len(cookies) == 0 || cookies[0].MaxAge >= 0 {
		t.Fatalf("revoked session cookie was not cleared: %+v", cookies)
	}
}

func signedTelegramInitData(t *testing.T, botToken string, authTime time.Time, user string) string {
	t.Helper()
	values := url.Values{
		"auth_date": {strconv.FormatInt(authTime.Unix(), 10)},
		"query_id":  {"AAE-test-query"},
		"user":      {user},
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secretMAC.Write([]byte(botToken))
	validationMAC := hmac.New(sha256.New, secretMAC.Sum(nil))
	_, _ = validationMAC.Write([]byte(strings.Join(parts, "\n")))
	values.Set("hash", hex.EncodeToString(validationMAC.Sum(nil)))
	return values.Encode()
}
