package logging

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"strings"
)

var telegramTokenPattern = regexp.MustCompile(`bot[0-9]+:[A-Za-z0-9_-]+`)

func NewJSONLogger(output io.Writer, level slog.Level, secrets ...string) *slog.Logger {
	filtered := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if strings.TrimSpace(secret) != "" {
			filtered = append(filtered, secret)
		}
	}
	return slog.New(&redactingHandler{
		next:    slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}),
		secrets: filtered,
	})
}

type redactingHandler struct {
	next    slog.Handler
	secrets []string
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	copyRecord := slog.NewRecord(record.Time, record.Level, h.redact(record.Message), record.PC)
	record.Attrs(func(attribute slog.Attr) bool {
		copyRecord.AddAttrs(h.redactAttr(attribute))
		return true
	})
	return h.next.Handle(ctx, copyRecord)
}

func (h *redactingHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attributes))
	for _, attribute := range attributes {
		redacted = append(redacted, h.redactAttr(attribute))
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted), secrets: h.secrets}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name), secrets: h.secrets}
}

func (h *redactingHandler) redactAttr(attribute slog.Attr) slog.Attr {
	attribute.Value = attribute.Value.Resolve()
	switch attribute.Value.Kind() {
	case slog.KindString:
		attribute.Value = slog.StringValue(h.redact(attribute.Value.String()))
	case slog.KindAny:
		if err, ok := attribute.Value.Any().(error); ok {
			attribute.Value = slog.StringValue(h.redact(err.Error()))
		}
	case slog.KindGroup:
		members := attribute.Value.Group()
		for index := range members {
			members[index] = h.redactAttr(members[index])
		}
		attribute.Value = slog.GroupValue(members...)
	}
	return attribute
}

func (h *redactingHandler) redact(value string) string {
	value = telegramTokenPattern.ReplaceAllString(value, "bot<redacted>")
	for _, secret := range h.secrets {
		value = strings.ReplaceAll(value, secret, "<redacted>")
	}
	return value
}
