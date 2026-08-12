package connectorapi

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

const defaultMaxPayload = 16 << 20

type Server struct {
	repository *repository.ConnectorRepository
	handler    http.Handler
}

func NewServer(repo *repository.ConnectorRepository) *Server {
	server := &Server{repository: repo}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/connector-spec", server.handleSpec)
	mux.HandleFunc("POST /api/v1/connectors/{id}/snapshots", server.handleSnapshot)
	mux.HandleFunc("GET /api/v1/connectors/{id}/runs/{runID}", server.handleRun)
	mux.HandleFunc("POST /api/v1/connectors/{id}/heartbeat", server.handleHeartbeat)
	server.handler = mux
	return server
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) handleSpec(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":              "Scheduler Connector API",
		"schema_version":    connector.SchemaVersion,
		"authentication":    "Ed25519",
		"snapshot_endpoint": "/api/v1/connectors/{connector_id}/snapshots",
		"documentation":     "/docs/connector-api.md",
	})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	client, rawBody, ok := s.authenticate(w, r, true)
	if !ok {
		return
	}
	if client.Status != domain.ConnectorStatusTesting &&
		client.Status != domain.ConnectorStatusPendingReview &&
		client.Status != domain.ConnectorStatusActive {
		writeError(w, http.StatusConflict, "Коннектор не принимает снимки в текущем состоянии")
		return
	}
	body, err := decodeBody(rawBody, r.Header.Get("Content-Encoding"), client.MaxPayloadBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var snapshot connector.Snapshot
	if err = json.Unmarshal(body, &snapshot); err != nil {
		writeError(w, http.StatusBadRequest, "Тело запроса не является корректным JSON")
		return
	}
	if err = connector.Validate(snapshot); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if snapshot.Institution.ExternalID != client.UniversityID {
		writeError(w, http.StatusForbidden, "Снимок относится к другому учебному заведению")
		return
	}
	idempotency := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotency == "" || len(idempotency) > 200 {
		writeError(w, http.StatusBadRequest, "Требуется корректный заголовок Idempotency-Key")
		return
	}
	if idempotency != snapshot.SnapshotID {
		writeError(w, http.StatusBadRequest, "Idempotency-Key должен совпадать с snapshot_id")
		return
	}
	run, duplicate, err := s.repository.Enqueue(
		r.Context(), client.ID, snapshot.SnapshotID, snapshot.SchemaVersion,
		idempotency, connector.PayloadDigest(body), body,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось поставить снимок в очередь")
		return
	}
	statusCode := http.StatusAccepted
	if duplicate {
		statusCode = http.StatusOK
	}
	writeJSON(w, statusCode, connector.SubmissionResponse{
		RunID: run.ID, Status: run.Status, SnapshotID: snapshot.SnapshotID,
		StatusURL: fmt.Sprintf("/api/v1/connectors/%s/runs/%s", client.ID, run.ID),
		Duplicate: duplicate,
	})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	client, _, ok := s.authenticate(w, r, false)
	if !ok {
		return
	}
	run, err := s.repository.Run(r.Context(), client.ID, r.PathValue("runID"))
	if errors.Is(err, repository.ErrConnectorNotFound) {
		writeError(w, http.StatusNotFound, "Загрузка не найдена")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось получить статус загрузки")
		return
	}
	writeJSON(w, http.StatusOK, connector.RunStatus{
		RunID: run.ID, ConnectorID: run.ConnectorID,
		ExternalSnapshot: run.ExternalSnapshotID, Status: run.Status,
		GroupCount: run.GroupCount, LessonCount: run.LessonCount,
		Error: run.ErrorMessage, ParserSnapshotID: run.ParserSnapshotID,
		ReceivedAt: run.ReceivedAt, CompletedAt: run.CompletedAt,
	})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	client, _, ok := s.authenticate(w, r, true)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "connector_id": client.ID, "server_time": time.Now().UTC(),
	})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request, readBody bool) (*domain.ConnectorClient, []byte, bool) {
	connectorID := r.PathValue("id")
	client, err := s.repository.Get(r.Context(), connectorID)
	if errors.Is(err, repository.ErrConnectorNotFound) {
		writeError(w, http.StatusUnauthorized, "Неизвестный коннектор")
		return nil, nil, false
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Не удалось проверить коннектор")
		return nil, nil, false
	}
	if client.IntegrationMode != domain.IntegrationModeExternalPush {
		writeError(w, http.StatusUnauthorized, "Эта интеграция не использует внешний Connector API")
		return nil, nil, false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(connector.HeaderKeyID)), []byte(client.KeyID)) != 1 {
		writeError(w, http.StatusUnauthorized, "Неизвестный ключ коннектора")
		return nil, nil, false
	}
	timestampRaw := r.Header.Get(connector.HeaderTimestamp)
	timestamp, err := time.Parse(time.RFC3339, timestampRaw)
	if err != nil || absDuration(time.Since(timestamp)) > 5*time.Minute {
		writeError(w, http.StatusUnauthorized, "Недействительное время подписи")
		return nil, nil, false
	}
	nonce := strings.TrimSpace(r.Header.Get(connector.HeaderNonce))
	if len(nonce) < 16 || len(nonce) > 200 {
		writeError(w, http.StatusUnauthorized, "Недействительный nonce")
		return nil, nil, false
	}
	limit := client.MaxPayloadBytes
	if limit <= 0 {
		limit = defaultMaxPayload
	}
	var body []byte
	if readBody {
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, int64(limit)+1))
		if err != nil || len(body) > limit {
			writeError(w, http.StatusRequestEntityTooLarge, "Тело запроса превышает лимит коннектора")
			return nil, nil, false
		}
	}
	digest := connector.PayloadDigest(body)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(r.Header.Get(connector.HeaderPayload))), []byte(digest)) != 1 {
		writeError(w, http.StatusUnauthorized, "Контрольная сумма запроса не совпадает")
		return nil, nil, false
	}
	publicKey := ed25519.PublicKey(client.PublicKey)
	if len(publicKey) != ed25519.PublicKeySize || !connector.VerifyRequest(
		publicKey, r.Method, r.URL.EscapedPath(), timestampRaw, nonce, digest,
		r.Header.Get(connector.HeaderSignature),
	) {
		writeError(w, http.StatusUnauthorized, "Подпись запроса не прошла проверку")
		return nil, nil, false
	}
	if err = s.repository.UseNonce(
		r.Context(), client.ID, nonce, timestamp.Add(10*time.Minute), client.RateLimitPerMinute,
	); errors.Is(err, repository.ErrConnectorReplay) {
		writeError(w, http.StatusConflict, "Запрос уже был обработан")
		return nil, nil, false
	} else if errors.Is(err, repository.ErrConnectorRateLimit) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "Превышен лимит запросов коннектора")
		return nil, nil, false
	} else if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Не удалось проверить уникальность запроса")
		return nil, nil, false
	}
	return client, body, true
}

func decodeBody(raw []byte, encoding string, limit int) ([]byte, error) {
	if strings.TrimSpace(strings.ToLower(encoding)) == "" {
		return raw, nil
	}
	if strings.ToLower(strings.TrimSpace(encoding)) != "gzip" {
		return nil, errors.New("поддерживается только Content-Encoding: gzip")
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("не удалось распаковать gzip")
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil || len(decoded) > limit {
		return nil, errors.New("распакованное тело превышает лимит коннектора")
	}
	return decoded, nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
