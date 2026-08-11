package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestJSONLoggerRedactsSecretsAndTelegramTokens(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSONLogger(&output, slog.LevelInfo, "database-password")
	logger.Error("request failed", "err", errors.New("https://api.telegram.org/bot123456:secret-token/getMe database-password"))
	logged := output.String()
	if strings.Contains(logged, "secret-token") || strings.Contains(logged, "database-password") {
		t.Fatalf("secret leaked into log: %s", logged)
	}
	if !strings.Contains(logged, "<redacted>") {
		t.Fatalf("redaction marker missing: %s", logged)
	}
}
