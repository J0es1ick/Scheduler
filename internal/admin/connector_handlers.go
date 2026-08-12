package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/declarative"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

var connectorSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

type createConnectorRequest struct {
	IntegrationMode    string `json:"integration_mode"`
	ParserID           string `json:"parser_id"`
	DeclarativeURL     string `json:"declarative_url"`
	UpdateInterval     int    `json:"update_interval"`
	UniversityID       string `json:"university_id"`
	UniversityName     string `json:"university_name"`
	UniversityFullName string `json:"university_full_name"`
	ScheduleURL        string `json:"schedule_url"`
	Timezone           string `json:"timezone"`
	Locale             string `json:"locale"`
	DisplayName        string `json:"display_name"`
	Description        string `json:"description"`
	MaintainerName     string `json:"maintainer_name"`
	MaintainerURL      string `json:"maintainer_url"`
}

type connectorCredentials struct {
	ConnectorID string `json:"connector_id"`
	KeyID       string `json:"key_id"`
	PrivateKey  string `json:"private_key"`
	SubmitPath  string `json:"submit_path"`
}

func (s *Server) handleConnectors(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Connectors(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить коннекторы")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleConnectorCatalog(w http.ResponseWriter, r *http.Request) {
	connectors, err := s.store.Connectors(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить каталог парсеров")
		return
	}
	linked := make(map[string]domain.ConnectorClient)
	for _, item := range connectors {
		if item.ParserID != "" && item.Status != domain.ConnectorStatusArchived {
			linked[item.ParserID] = item
		}
	}
	items := make([]map[string]any, 0, len(s.managedParsers))
	for _, manifest := range s.managedParsers {
		item := map[string]any{"manifest": manifest, "connected": false}
		if current, ok := linked[manifest.ParserID]; ok {
			item["connected"] = true
			item["connector_id"] = current.ID
			item["status"] = current.Status
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateConnector(w http.ResponseWriter, r *http.Request) {
	var request createConnectorRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректные параметры коннектора")
		return
	}
	request.IntegrationMode = strings.TrimSpace(request.IntegrationMode)
	if request.IntegrationMode == "" {
		request.IntegrationMode = domain.IntegrationModeExternalPush
	}
	request.ParserID = strings.TrimSpace(request.ParserID)
	request.DeclarativeURL = strings.TrimSpace(request.DeclarativeURL)
	if request.UpdateInterval == 0 {
		request.UpdateInterval = 3600
	}
	if request.UpdateInterval < 300 || request.UpdateInterval > 604800 {
		writeAPIError(w, http.StatusUnprocessableEntity, "Интервал должен быть от 5 минут до 7 дней")
		return
	}
	if request.IntegrationMode == domain.IntegrationModeManagedParser {
		manifest, ok := s.managedParsers[request.ParserID]
		if !ok {
			writeAPIError(w, http.StatusUnprocessableEntity, "Выбранный управляемый парсер не установлен в этой версии Scheduler")
			return
		}
		request.UniversityID = manifest.Institution.ExternalID
		request.UniversityName = manifest.Institution.Name
		request.UniversityFullName = manifest.Institution.FullName
		request.ScheduleURL = manifest.Institution.ScheduleURL
		request.Timezone = manifest.Institution.Timezone
		request.Locale = manifest.Institution.Locale
		request.DisplayName = manifest.DisplayName
		request.Description = manifest.Description
		request.MaintainerName = manifest.MaintainerName
		request.MaintainerURL = manifest.MaintainerURL
		request.UpdateInterval = manifest.UpdateSeconds
	}
	request.UniversityID = strings.ToLower(strings.TrimSpace(request.UniversityID))
	request.UniversityName = strings.TrimSpace(request.UniversityName)
	request.UniversityFullName = strings.TrimSpace(request.UniversityFullName)
	request.ScheduleURL = strings.TrimSpace(request.ScheduleURL)
	request.Timezone = strings.TrimSpace(request.Timezone)
	request.Locale = strings.TrimSpace(request.Locale)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Description = strings.TrimSpace(request.Description)
	request.MaintainerName = strings.TrimSpace(request.MaintainerName)
	request.MaintainerURL = strings.TrimSpace(request.MaintainerURL)
	if !connectorSlugPattern.MatchString(request.UniversityID) ||
		request.UniversityName == "" || request.DisplayName == "" {
		writeAPIError(w, http.StatusUnprocessableEntity, "Укажите корректный slug, название вуза и название коннектора")
		return
	}
	if request.Timezone == "" {
		request.Timezone = "Europe/Moscow"
	}
	if _, err := time.LoadLocation(request.Timezone); err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "Неизвестный часовой пояс IANA")
		return
	}
	if request.Locale == "" {
		request.Locale = "ru-RU"
	}
	if !optionalHTTPURL(request.ScheduleURL) || !optionalHTTPURL(request.MaintainerURL) {
		writeAPIError(w, http.StatusUnprocessableEntity, "Ссылки должны использовать абсолютный HTTP(S) URL")
		return
	}
	adapterType := domain.IntegrationModeExternalPush
	sourceConfig := "{}"
	switch request.IntegrationMode {
	case domain.IntegrationModeManagedParser:
		adapterType = "managed:" + request.ParserID
	case domain.IntegrationModeDeclarative:
		config := declarative.Config{
			URL:                request.DeclarativeURL,
			UniversityID:       request.UniversityID,
			UniversityName:     request.UniversityName,
			UniversityFullName: request.UniversityFullName,
			ScheduleURL:        request.ScheduleURL,
			Timezone:           request.Timezone,
			Locale:             request.Locale,
		}
		if err := declarative.ValidateConfig(config); err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		encoded, _ := json.Marshal(config)
		sourceConfig = string(encoded)
		adapterType = declarative.AdapterType
	case domain.IntegrationModeExternalPush:
	default:
		writeAPIError(w, http.StatusUnprocessableEntity, "Неизвестный способ интеграции")
		return
	}
	publicEncoded, privateEncoded, err := connector.GenerateKeyPair()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось создать ключ коннектора")
		return
	}
	publicKey, _ := connector.DecodePublicKey(publicEncoded)
	connectorID := "connector-" + uuid.NewString()
	sourceID := "integration-" + uuid.NewString()
	keyID := "key-" + uuid.NewString()
	identity := identityFromContext(r.Context())
	created, err := s.store.CreateConnector(r.Context(), repository.CreateConnectorParams{
		ConnectorID: connectorID, SourceID: sourceID,
		UniversityID: request.UniversityID, UniversityName: request.UniversityName,
		UniversityFullName: request.UniversityFullName, ScheduleURL: request.ScheduleURL,
		Timezone: request.Timezone, Locale: request.Locale,
		DisplayName: request.DisplayName, Description: request.Description,
		MaintainerName: request.MaintainerName, MaintainerURL: request.MaintainerURL,
		KeyID: keyID, PublicKey: publicKey, CreatedBy: identity.ID,
		QualityPolicy:   domain.DefaultSourceQualityPolicy(),
		IntegrationMode: request.IntegrationMode, ParserID: request.ParserID,
		AdapterType: adapterType, SourceConfig: sourceConfig, UpdateInterval: request.UpdateInterval,
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeAPIError(w, http.StatusConflict, "Такой вуз или источник уже зарегистрирован")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Не удалось создать коннектор")
		return
	}
	s.writeAudit(r.Context(), identity, "connector_created", "connector", connectorID,
		map[string]any{"university_id": request.UniversityID, "source_id": sourceID}, s.requestIP(r))
	response := map[string]any{
		"connector":           created,
		"credentials_warning": "",
	}
	if request.IntegrationMode == domain.IntegrationModeExternalPush {
		response["credentials"] = connectorCredentials{
			ConnectorID: connectorID, KeyID: keyID, PrivateKey: privateEncoded,
			SubmitPath: "/api/v1/connectors/" + connectorID + "/snapshots",
		}
		response["credentials_warning"] = "Закрытый ключ показывается только один раз"
	}
	writeJSON(w, http.StatusCreated, response)
}

type updateConnectorRequest struct {
	Status        string                      `json:"status"`
	QualityPolicy *domain.SourceQualityPolicy `json:"quality_policy"`
}

func (s *Server) handleUpdateConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.store.Connector(r.Context(), id)
	if errors.Is(err, repository.ErrConnectorNotFound) {
		writeAPIError(w, http.StatusNotFound, "Коннектор не найден")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить коннектор")
		return
	}
	var request updateConnectorRequest
	if err = decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректные параметры")
		return
	}
	identity := identityFromContext(r.Context())
	if request.QualityPolicy != nil {
		if err = validateQualityPolicy(*request.QualityPolicy); err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if err = s.store.UpdateConnectorPolicy(r.Context(), id, *request.QualityPolicy); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "Не удалось сохранить правила качества")
			return
		}
	}
	if request.Status != "" && request.Status != current.Status {
		if !connectorTransitionAllowed(current.Status, request.Status) {
			writeAPIError(w, http.StatusConflict, "Недопустимый переход состояния коннектора")
			return
		}
		if request.Status == domain.ConnectorStatusActive {
			runID, snapshotID, snapshotStatus, candidateErr := s.store.ConnectorActivationCandidate(r.Context(), id)
			if errors.Is(candidateErr, sql.ErrNoRows) || candidateErr != nil && snapshotID == "" {
				writeAPIError(w, http.StatusConflict, "Перед активацией отправьте и проверьте тестовый снимок")
				return
			}
			if candidateErr != nil {
				writeAPIError(w, http.StatusInternalServerError, "Не удалось проверить тестовый снимок")
				return
			}
			if snapshotStatus != domain.SnapshotStatusPublished {
				writeAPIError(w, http.StatusConflict, "Сначала откройте тестовый снимок в разделе источников и подтвердите его публикацию")
				return
			}
			candidate, loadErr := repository.NewParserSnapshotRepository(s.store.db).Get(r.Context(), snapshotID)
			if loadErr != nil || candidate == nil {
				writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить проверенный снимок")
				return
			}
			if current.IntegrationMode == domain.IntegrationModeExternalPush && runID != "" {
				_ = s.store.MarkConnectorRunPublished(
					r.Context(), runID, snapshotID, candidate.GroupCount, candidate.LessonCount,
				)
			}
		}
		if err = s.store.UpdateConnectorStatus(r.Context(), id, request.Status); errors.Is(err, repository.ErrActiveSourceConflict) {
			writeAPIError(w, http.StatusConflict, "У этого вуза уже есть активный источник. Сначала приостановите его")
			return
		} else if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "Не удалось изменить состояние коннектора")
			return
		}
	}
	s.writeAudit(r.Context(), identity, "connector_updated", "connector", id,
		map[string]any{"status": request.Status, "quality_policy": request.QualityPolicy}, s.requestIP(r))
	updated, _ := s.store.Connector(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"connector": updated})
}

func (s *Server) handleRotateConnectorKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.store.Connector(r.Context(), id)
	if errors.Is(err, repository.ErrConnectorNotFound) {
		writeAPIError(w, http.StatusNotFound, "Коннектор не найден")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить коннектор")
		return
	}
	if current.IntegrationMode != domain.IntegrationModeExternalPush {
		writeAPIError(w, http.StatusConflict, "Ключи используются только внешними push-коннекторами")
		return
	}
	publicEncoded, privateEncoded, err := connector.GenerateKeyPair()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось создать ключ")
		return
	}
	publicKey, _ := connector.DecodePublicKey(publicEncoded)
	keyID := "key-" + uuid.NewString()
	if err = s.store.RotateConnectorKey(r.Context(), id, keyID, publicKey); errors.Is(err, repository.ErrConnectorNotFound) {
		writeAPIError(w, http.StatusNotFound, "Коннектор не найден")
		return
	} else if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось заменить ключ")
		return
	}
	identity := identityFromContext(r.Context())
	s.writeAudit(r.Context(), identity, "connector_key_rotated", "connector", id,
		map[string]any{"key_id": keyID}, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"credentials": connectorCredentials{ConnectorID: id, KeyID: keyID, PrivateKey: privateEncoded,
			SubmitPath: "/api/v1/connectors/" + id + "/snapshots"},
		"credentials_warning": "Старый ключ отозван. Новый закрытый ключ показывается только один раз",
	})
}

func (s *Server) handleConnectorRuns(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ConnectorRuns(r.Context(), r.PathValue("id"), 100)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить отправки коннектора")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func optionalHTTPURL(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func validateQualityPolicy(policy domain.SourceQualityPolicy) error {
	if policy.MinimumGroups < 0 || policy.MinimumLessons < 0 {
		return errors.New("Минимальные количества не могут быть отрицательными")
	}
	for _, value := range []float64{
		policy.MaximumGroupDropRatio, policy.MaximumGroupGrowthRatio,
		policy.MaximumLessonDropRatio, policy.MaximumLessonGrowthRatio,
	} {
		if value < 0 || value > 10 {
			return errors.New("Порог изменения должен находиться между 0 и 10")
		}
	}
	return nil
}

func connectorTransitionAllowed(from, to string) bool {
	allowed := map[string]map[string]bool{
		domain.ConnectorStatusDraft:         {domain.ConnectorStatusTesting: true, domain.ConnectorStatusArchived: true},
		domain.ConnectorStatusTesting:       {domain.ConnectorStatusPendingReview: true, domain.ConnectorStatusDraft: true, domain.ConnectorStatusSuspended: true},
		domain.ConnectorStatusPendingReview: {domain.ConnectorStatusTesting: true, domain.ConnectorStatusActive: true, domain.ConnectorStatusSuspended: true},
		domain.ConnectorStatusActive:        {domain.ConnectorStatusSuspended: true},
		domain.ConnectorStatusSuspended:     {domain.ConnectorStatusTesting: true, domain.ConnectorStatusActive: true, domain.ConnectorStatusArchived: true},
		domain.ConnectorStatusArchived:      {domain.ConnectorStatusDraft: true},
	}
	return allowed[from][to]
}
