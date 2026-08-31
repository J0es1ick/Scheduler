package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func ProductionPreflight(getenv func(string) string) error {
	var failures []error
	require := func(ok bool, message string) {
		if !ok {
			failures = append(failures, errors.New(message))
		}
	}
	require(getenv("DEPLOYMENT_ENV") == "production", "DEPLOYMENT_ENV must be production")
	require(getenv("DATABASE_SSLMODE") == "verify-full", "DATABASE_SSLMODE must be verify-full")
	require(getenv("ADMIN_COOKIE_SECURE") == "true", "ADMIN_COOKIE_SECURE must be true")
	require(getenv("ADMIN_ACCESS_LOGIN_ENABLED") == "false", "emergency access-key login must be disabled")
	require(getenv("BOT_TELEGRAM_API_ALLOW_INSECURE") != "true", "insecure Telegram API must be disabled")
	for _, key := range []string{"ADMIN_PUBLIC_URL", "SITE_PUBLIC_URL"} {
		parsed, err := url.Parse(getenv(key))
		require(err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil, key+" must be an absolute HTTPS URL without credentials")
	}
	for _, key := range []string{"BOT_TOKEN", "ADMIN_METRICS_TOKEN"} {
		require(len(getenv(key)) >= 32 && !isPlaceholderSecret(getenv(key)), key+" must contain a real secret of at least 32 characters")
	}
	roles, passwords := map[string]bool{}, map[string]bool{}
	for _, role := range []string{"MIGRATOR", "BOT", "ADMIN", "SITE", "BACKUP", "RESTORE"} {
		userKey, passwordKey := "DATABASE_"+role+"_USER", "DATABASE_"+role+"_PASSWORD"
		user, password := strings.TrimSpace(getenv(userKey)), getenv(passwordKey)
		require(user != "" && user != "postgres" && !roles[user], userKey+" must be a separate non-superuser role")
		require(len(password) >= 24 && !isPlaceholderSecret(password) && !passwords[password], passwordKey+" must be a distinct secret of at least 24 characters")
		roles[user], passwords[password] = true, true
	}
	require(getenv("DATABASE_HOST") != "" && getenv("DATABASE_NAME") != "", "database host and name are required")
	if err := validatePort("DATABASE_PORT", getenv("DATABASE_PORT")); err != nil {
		failures = append(failures, err)
	}
	ca, err := os.ReadFile(getenv("PGSSLROOTCERT"))
	pool := x509.NewCertPool()
	require(err == nil && pool.AppendCertsFromPEM(ca), "PGSSLROOTCERT must point to a readable PEM CA certificate")
	require(getenv("BACKUP_REQUIRE_OFFSITE") == "true", "BACKUP_REQUIRE_OFFSITE must be true")
	destination, err := os.Stat(getenv("BACKUP_OFFSITE_DIRECTORY"))
	require(err == nil && destination.IsDir(), "offsite backup directory must be mounted")
	require(regexp.MustCompile(`^age1[0-9a-z]{58}$`).MatchString(getenv("BACKUP_AGE_RECIPIENT")), "BACKUP_AGE_RECIPIENT must contain an age public recipient")
	interval, err := strconv.Atoi(getenv("BACKUP_INTERVAL_SECONDS"))
	require(err == nil && interval >= 60 && interval <= 86400, "backup interval must be between 60 and 86400 seconds")
	for _, key := range []string{"BACKUP_MAX_AGE_SECONDS", "BACKUP_OFFSITE_MAX_AGE_SECONDS"} {
		value, err := strconv.Atoi(getenv(key))
		require(err == nil && value > interval && value <= 172800, key+" must exceed the interval and not exceed 172800 seconds")
	}
	if len(failures) != 0 {
		return fmt.Errorf("production preflight: %w", errors.Join(failures...))
	}
	return nil
}
