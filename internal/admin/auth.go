package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)

const (
	adminSessionCookie = "scheduler_admin_session"
	maxAdminSessions   = 10_000
)

type session struct {
	identity AdminIdentity
	expires  time.Time
}

type identityContextKey struct{}

type telegramAdminChecker interface {
	TelegramAdmin(context.Context, string) (*UserView, error)
}

type adminSessionStore interface {
	SaveAdminSession(context.Context, string, AdminIdentity, time.Time, int) error
	AdminSession(context.Context, string) (AdminIdentity, time.Time, error)
	DeleteAdminSession(context.Context, string) error
}

type AuthManager struct {
	botToken       string
	accessToken    string
	accessKeyLogin bool
	cookieSecure   bool
	ttl            time.Duration
	maxSessions    int
	persistence    adminSessionStore

	mu       sync.Mutex
	sessions map[string]session
}

func (a *AuthManager) UseSessionStore(store adminSessionStore) {
	a.persistence = store
}

func NewAuthManager(botToken, accessToken string, accessKeyLogin, cookieSecure bool) *AuthManager {
	return &AuthManager{
		botToken:       botToken,
		accessToken:    accessToken,
		accessKeyLogin: accessKeyLogin,
		cookieSecure:   cookieSecure,
		ttl:            12 * time.Hour,
		maxSessions:    maxAdminSessions,
		sessions:       make(map[string]session),
	}
}

func (a *AuthManager) AccessKeyEnabled() bool {
	return a.accessKeyLogin && a.accessToken != ""
}

func (a *AuthManager) LoginWithAccessKey(key string) (AdminIdentity, error) {
	if !a.AccessKeyEnabled() {
		return AdminIdentity{}, ErrForbidden
	}
	expected := sha256.Sum256([]byte(a.accessToken))
	actual := sha256.Sum256([]byte(key))
	if subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
		return AdminIdentity{}, ErrUnauthorized
	}
	return AdminIdentity{ID: "local-admin", Name: "Администратор", AuthMethod: "access_key", Role: "owner"}, nil
}

func (a *AuthManager) LoginWithTelegram(ctx context.Context, store telegramAdminChecker, initData string) (AdminIdentity, error) {
	telegramUser, err := validateTelegramInitData(initData, a.botToken, time.Now())
	if err != nil {
		return AdminIdentity{}, ErrUnauthorized
	}
	user, err := store.TelegramAdmin(ctx, strconv.FormatInt(telegramUser.ID, 10))
	if err != nil || user == nil || !user.IsAdmin {
		return AdminIdentity{}, ErrForbidden
	}
	name := strings.TrimSpace(user.Username)
	if name == "" {
		name = telegramUser.Username
	}
	if name == "" {
		name = telegramUser.FirstName
	}
	if name == "" {
		name = strconv.FormatInt(telegramUser.ID, 10)
	}
	if !strings.HasPrefix(name, "@") && (user.Username != "" || telegramUser.Username != "") {
		name = "@" + name
	}
	return AdminIdentity{ID: user.ID, Name: name, AuthMethod: "telegram", Role: user.AdminRole}, nil
}

func (a *AuthManager) IssueSession(w http.ResponseWriter, identity AdminIdentity) (AdminIdentity, error) {
	token, err := randomToken(32)
	if err != nil {
		return AdminIdentity{}, err
	}
	csrf, err := randomToken(24)
	if err != nil {
		return AdminIdentity{}, err
	}
	identity.CSRFToken = csrf
	expires := time.Now().Add(a.ttl)
	if a.persistence != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = a.persistence.SaveAdminSession(ctx, tokenHash(token), identity, expires, a.maxSessions); err != nil {
			return AdminIdentity{}, fmt.Errorf("save admin session: %w", err)
		}
	} else {
		a.mu.Lock()
		a.cleanupExpiredLocked(time.Now())
		if len(a.sessions) >= a.maxSessions {
			a.evictEarliestSessionLocked()
		}
		a.sessions[token] = session{identity: identity, expires: expires}
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(a.ttl.Seconds()),
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	return identity, nil
}

func (a *AuthManager) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminSessionCookie); err == nil {
		if a.persistence != nil {
			_ = a.persistence.DeleteAdminSession(r.Context(), tokenHash(cookie.Value))
		} else {
			a.mu.Lock()
			delete(a.sessions, cookie.Value)
			a.mu.Unlock()
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *AuthManager) Require(store telegramAdminChecker, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := a.identityForRequest(r)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "Требуется вход в админку")
			return
		}
		if identity.AuthMethod == "telegram" {
			user, err := store.TelegramAdmin(r.Context(), identity.ID)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && (user == nil || !user.IsAdmin)) {
				a.Logout(w, r)
				writeAPIError(w, http.StatusForbidden, "Права администратора отозваны")
				return
			}
			if err != nil {
				writeAPIError(w, http.StatusServiceUnavailable, "Не удалось проверить права администратора")
				return
			}
			identity.Role = user.AdminRole
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			provided := r.Header.Get("X-CSRF-Token")
			if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(identity.CSRFToken)) != 1 {
				writeAPIError(w, http.StatusForbidden, "Проверка безопасности запроса не пройдена")
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		r.Header.Set(internalAdminIDHeader, identity.ID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityContextKey{}, identity)))
	})
}

func (a *AuthManager) identityForRequest(r *http.Request) (AdminIdentity, bool) {
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil || cookie.Value == "" {
		return AdminIdentity{}, false
	}
	if a.persistence != nil {
		identity, expires, loadErr := a.persistence.AdminSession(r.Context(), tokenHash(cookie.Value))
		return identity, loadErr == nil && expires.After(time.Now())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	a.cleanupExpiredLocked(now)
	current, ok := a.sessions[cookie.Value]
	if !ok || current.expires.Before(now) {
		return AdminIdentity{}, false
	}
	return current.identity, true
}

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (a *AuthManager) cleanupExpiredLocked(now time.Time) {
	for token, current := range a.sessions {
		if current.expires.Before(now) {
			delete(a.sessions, token)
		}
	}
}

func (a *AuthManager) evictEarliestSessionLocked() {
	var earliestToken string
	var earliestExpiry time.Time
	for token, current := range a.sessions {
		if earliestToken == "" || current.expires.Before(earliestExpiry) {
			earliestToken = token
			earliestExpiry = current.expires
		}
	}
	if earliestToken != "" {
		delete(a.sessions, earliestToken)
	}
}

func identityFromContext(ctx context.Context) AdminIdentity {
	identity, _ := ctx.Value(identityContextKey{}).(AdminIdentity)
	return identity
}

type telegramWebAppUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

func validateTelegramInitData(raw, botToken string, now time.Time) (telegramWebAppUser, error) {
	if raw == "" || botToken == "" {
		return telegramWebAppUser{}, ErrUnauthorized
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return telegramWebAppUser{}, err
	}
	providedHash, err := hex.DecodeString(values.Get("hash"))
	if err != nil || len(providedHash) == 0 {
		return telegramWebAppUser{}, ErrUnauthorized
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "hash" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	dataCheckString := strings.Join(parts, "\n")
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secretMAC.Write([]byte(botToken))
	validationMAC := hmac.New(sha256.New, secretMAC.Sum(nil))
	_, _ = validationMAC.Write([]byte(dataCheckString))
	if !hmac.Equal(validationMAC.Sum(nil), providedHash) {
		return telegramWebAppUser{}, ErrUnauthorized
	}
	authUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return telegramWebAppUser{}, ErrUnauthorized
	}
	authTime := time.Unix(authUnix, 0)
	if authTime.After(now.Add(time.Minute)) || now.Sub(authTime) > 15*time.Minute {
		return telegramWebAppUser{}, ErrUnauthorized
	}
	var user telegramWebAppUser
	if err = json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID == 0 {
		return telegramWebAppUser{}, ErrUnauthorized
	}
	return user, nil
}

func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate secure token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
