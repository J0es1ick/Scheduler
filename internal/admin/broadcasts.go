package admin

import (
	"bytes"
	"database/sql"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Server) broadcastError(w http.ResponseWriter, err error) {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeAPIError(w, 404, "Рассылка или вложение не найдены")
	case errors.Is(err, repository.ErrBroadcastConflict):
		writeAPIError(w, 409, err.Error())
	case errors.As(err, &pg):
		slog.Error("broadcast database request failed", "err", err)
		writeAPIError(w, 500, "Не удалось сохранить рассылку")
	default:
		writeAPIError(w, 400, err.Error())
	}
}
func (s *Server) handleBroadcastList(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	offset = max(0, offset)
	items, err := repository.NewBroadcastRepository(s.store.db).List(r.Context(), offset)
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) handleBroadcastCreate(w http.ResponseWriter, r *http.Request) {
	id, err := repository.NewBroadcastRepository(s.store.db).Create(r.Context(), identityFromContext(r.Context()).ID)
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (s *Server) handleBroadcastGet(w http.ResponseWriter, r *http.Request) {
	b, err := repository.NewBroadcastRepository(s.store.db).Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) handleBroadcastSave(w http.ResponseWriter, r *http.Request) {
	var input repository.BroadcastInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, 400, "Некорректный документ")
		return
	}
	if err := repository.NewBroadcastRepository(s.store.db).Save(r.Context(), r.PathValue("id"), input); err != nil {
		s.broadcastError(w, err)
		return
	}
	s.handleBroadcastGet(w, r)
}
func (s *Server) handleBroadcastAudience(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Mode      string `json:"mode"`
		Usernames string `json:"usernames"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, 400, "Некорректный список получателей")
		return
	}
	audience, err := repository.NewBroadcastRepository(s.store.db).Audience(r.Context(), input.Mode, input.Usernames)
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	writeJSON(w, 200, audience)
}
func (s *Server) handleBroadcastPreview(w http.ResponseWriter, r *http.Request) {
	repo := repository.NewBroadcastRepository(s.store.db)
	b, err := repo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	body, err := domain.RenderBroadcastDocument(b.Document)
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	eligible := b.Counts["ready"]
	if b.AudienceMode == "all" {
		audience, err := repo.Audience(r.Context(), "all", "")
		if err != nil {
			s.broadcastError(w, err)
			return
		}
		eligible = audience.Eligible
	}
	writeJSON(w, 200, map[string]any{"broadcast": b, "body": body, "eligible": eligible})
}
func (s *Server) handleBroadcastSend(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int    `json:"version"`
		Key     string `json:"key"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, 400, "Некорректный запрос отправки")
		return
	}
	if err := repository.NewBroadcastRepository(s.store.db).Send(r.Context(), r.PathValue("id"), input.Key, input.Version); err != nil {
		s.broadcastError(w, err)
		return
	}
	s.handleBroadcastGet(w, r)
}
func (s *Server) handleBroadcastStop(w http.ResponseWriter, r *http.Request) {
	if err := repository.NewBroadcastRepository(s.store.db).Stop(r.Context(), r.PathValue("id")); err != nil {
		s.broadcastError(w, err)
		return
	}
	s.handleBroadcastGet(w, r)
}
func (s *Server) handleBroadcastDelete(w http.ResponseWriter, r *http.Request) {
	if err := repository.NewBroadcastRepository(s.store.db).DeleteDraft(r.Context(), r.PathValue("id")); err != nil {
		s.broadcastError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleBroadcastUpload(w http.ResponseWriter, r *http.Request) {
	version, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil {
		writeAPIError(w, 400, "Укажите версию черновика")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10000000+65536)
	reader, err := r.MultipartReader()
	if err != nil {
		writeAPIError(w, 400, "Ожидается файл")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FileName() == "" {
		writeAPIError(w, 400, "Ожидается файл")
		return
	}
	data, err := io.ReadAll(io.LimitReader(part, 10000001))
	part.Close()
	if err != nil || len(data) == 0 || len(data) > 10000000 {
		writeAPIError(w, 400, "Размер вложения должен быть от 1 байта до 10 МБ")
		return
	}
	if _, err = reader.NextPart(); !errors.Is(err, io.EOF) {
		writeAPIError(w, 400, "Загружайте по одному файлу")
		return
	}
	name := filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
	if len(name) > 240 || strings.ContainsAny(name, "\r\n\x00") {
		writeAPIError(w, 400, "Некорректное имя файла")
		return
	}
	kind := r.URL.Query().Get("type")
	if kind != "photo" && kind != "document" {
		writeAPIError(w, 400, "Выберите фото или документ")
		return
	}
	contentType := http.DetectContentType(data)
	if kind == "photo" {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || (format != "jpeg" && format != "png") || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width+cfg.Height > 10000 || cfg.Width > 20*cfg.Height || cfg.Height > 20*cfg.Width {
			writeAPIError(w, 400, "Это изображение нельзя отправить как фото. Прикрепите его как документ.")
			return
		}
	}
	err = repository.NewBroadcastRepository(s.store.db).AddAttachment(r.Context(), r.PathValue("id"), version, domain.BroadcastAttachment{Filename: name, MediaType: kind, ContentType: contentType, Data: data})
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	s.handleBroadcastGet(w, r)
}
func (s *Server) handleBroadcastAttachment(w http.ResponseWriter, r *http.Request) {
	a, err := repository.NewBroadcastRepository(s.store.db).Attachment(r.Context(), r.PathValue("id"), r.PathValue("attachment"))
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	disposition := "attachment"
	contentType := "application/octet-stream"
	if a.MediaType == "photo" {
		disposition = "inline"
		contentType = a.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Filename}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(a.Data)
}
func (s *Server) handleBroadcastAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	version, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil {
		writeAPIError(w, 400, "Укажите версию черновика")
		return
	}
	err = repository.NewBroadcastRepository(s.store.db).RemoveAttachment(r.Context(), r.PathValue("id"), r.PathValue("attachment"), version)
	if err != nil {
		s.broadcastError(w, err)
		return
	}
	s.handleBroadcastGet(w, r)
}
