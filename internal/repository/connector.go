package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

var (
	ErrConnectorNotFound    = errors.New("connector not found")
	ErrConnectorReplay      = errors.New("connector request replayed")
	ErrConnectorRateLimit   = errors.New("connector rate limit exceeded")
	ErrIngestionDuplicate   = errors.New("ingestion already exists")
	ErrActiveSourceConflict = errors.New("another source is already active for this university")
)

type ConnectorRepository struct {
	db *sqlx.DB
}

type CreateConnectorParams struct {
	ConnectorID        string
	SourceID           string
	UniversityID       string
	UniversityName     string
	UniversityFullName string
	ScheduleURL        string
	Timezone           string
	Locale             string
	DisplayName        string
	Description        string
	MaintainerName     string
	MaintainerURL      string
	KeyID              string
	PublicKey          []byte
	CreatedBy          string
	QualityPolicy      domain.SourceQualityPolicy
	IntegrationMode    string
	ParserID           string
	AdapterType        string
	SourceConfig       string
	UpdateInterval     int
}

func NewConnectorRepository(db *sqlx.DB) *ConnectorRepository {
	return &ConnectorRepository{db: db}
}

func (r *ConnectorRepository) Create(ctx context.Context, params CreateConnectorParams) (*domain.ConnectorClient, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("create connector: begin: %w", err)
	}
	defer tx.Rollback()

	if params.Timezone == "" {
		params.Timezone = "Europe/Moscow"
	}
	if params.Locale == "" {
		params.Locale = "ru-RU"
	}
	if params.IntegrationMode == "" {
		params.IntegrationMode = domain.IntegrationModeExternalPush
	}
	if params.AdapterType == "" {
		params.AdapterType = domain.IntegrationModeExternalPush
	}
	if params.SourceConfig == "" {
		params.SourceConfig = "{}"
	}
	if params.UpdateInterval <= 0 {
		params.UpdateInterval = 3600
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO universities (
			id, name, full_name, schedule_url, timezone, locale, is_active, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,FALSE,NOW(),NOW())
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name,
			full_name=CASE WHEN EXCLUDED.full_name='' THEN universities.full_name ELSE EXCLUDED.full_name END,
			schedule_url=CASE WHEN EXCLUDED.schedule_url='' THEN universities.schedule_url ELSE EXCLUDED.schedule_url END,
			timezone=EXCLUDED.timezone,
			locale=EXCLUDED.locale,
			updated_at=NOW()`,
		params.UniversityID, params.UniversityName, params.UniversityFullName,
		params.ScheduleURL, params.Timezone, params.Locale,
	); err != nil {
		return nil, fmt.Errorf("create connector university: %w", err)
	}
	policy, err := json.Marshal(params.QualityPolicy)
	if err != nil {
		return nil, fmt.Errorf("create connector policy: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO data_sources (
			id, university_id, adapter_type, config, is_enabled, update_interval,
			last_error, lifecycle_status, quality_policy, created_at, updated_at
		) VALUES ($1,$2,$3,$4,FALSE,$5,'','draft',$6::jsonb,NOW(),NOW())`,
		params.SourceID, params.UniversityID, params.AdapterType, params.SourceConfig,
		params.UpdateInterval, policy,
	); err != nil {
		return nil, fmt.Errorf("create connector source: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO connector_clients (
			id, data_source_id, display_name, description, maintainer_name,
			maintainer_url, key_id, public_key, status, created_by,
			integration_mode, parser_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'draft',$9,$10,$11)`,
		params.ConnectorID, params.SourceID, params.DisplayName, params.Description,
		params.MaintainerName, params.MaintainerURL, params.KeyID, params.PublicKey, params.CreatedBy,
		params.IntegrationMode, params.ParserID,
	); err != nil {
		return nil, fmt.Errorf("create connector client: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("create connector: commit: %w", err)
	}
	return r.Get(ctx, params.ConnectorID)
}

const connectorColumns = `
	c.id, c.data_source_id, ds.university_id, u.name AS university_name,
	c.display_name, c.description, c.maintainer_name, c.maintainer_url,
	c.integration_mode, c.parser_id,
	c.key_id, c.public_key, c.status, c.rate_limit_per_minute,
	c.max_payload_bytes,
	COALESCE(c.last_seen_at, ds.last_run_at) AS last_seen_at,
	COALESCE(c.last_snapshot_at, (
		SELECT MAX(ps.created_at) FROM parser_snapshots ps WHERE ps.data_source_id=ds.id
	)) AS last_snapshot_at,
	c.created_by, c.created_at, c.updated_at, ds.quality_policy`

func (r *ConnectorRepository) Get(ctx context.Context, connectorID string) (*domain.ConnectorClient, error) {
	var connector domain.ConnectorClient
	err := r.db.GetContext(ctx, &connector, `SELECT `+connectorColumns+`
		FROM connector_clients c
		JOIN data_sources ds ON ds.id=c.data_source_id
		JOIN universities u ON u.id=ds.university_id
		WHERE c.id=$1`, connectorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConnectorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get connector %s: %w", connectorID, err)
	}
	return &connector, nil
}

func (r *ConnectorRepository) List(ctx context.Context) ([]domain.ConnectorClient, error) {
	items := []domain.ConnectorClient{}
	if err := r.db.SelectContext(ctx, &items, `SELECT `+connectorColumns+`
		FROM connector_clients c
		JOIN data_sources ds ON ds.id=c.data_source_id
		JOIN universities u ON u.id=ds.university_id
		ORDER BY c.created_at DESC`); err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}
	return items, nil
}

func (r *ConnectorRepository) UseNonce(ctx context.Context, connectorID, nonce string, expiresAt time.Time, limit int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM connector_request_nonces WHERE expires_at<NOW()`); err != nil {
		return err
	}
	var recent int
	if err = tx.GetContext(ctx, &recent, `
		SELECT COUNT(*)::int FROM connector_request_nonces
		WHERE connector_id=$1 AND created_at>NOW()-INTERVAL '1 minute'`, connectorID); err != nil {
		return err
	}
	if recent >= limit {
		return ErrConnectorRateLimit
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO connector_request_nonces (connector_id, nonce, expires_at)
		VALUES ($1,$2,$3)`, connectorID, nonce, expiresAt); err != nil {
		if isUniqueViolation(err) {
			return ErrConnectorReplay
		}
		return fmt.Errorf("store connector nonce: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE connector_clients SET last_seen_at=NOW() WHERE id=$1`, connectorID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ConnectorRepository) Enqueue(
	ctx context.Context,
	connectorID, externalSnapshotID, schemaVersion, idempotencyKey, digest string,
	payload json.RawMessage,
) (*domain.ConnectorIngestionRun, bool, error) {
	id := uuid.NewString()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_ingestion_runs (
			id, connector_id, external_snapshot_id, schema_version,
			idempotency_key, payload_sha256, payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`,
		id, connectorID, externalSnapshotID, schemaVersion, idempotencyKey, digest, payload,
	)
	if err != nil {
		if !isUniqueViolation(err) {
			return nil, false, fmt.Errorf("enqueue connector snapshot: %w", err)
		}
		var existingID string
		if getErr := r.db.GetContext(ctx, &existingID, `
			SELECT id FROM connector_ingestion_runs
			WHERE connector_id=$1 AND (external_snapshot_id=$2 OR idempotency_key=$3)
			ORDER BY received_at DESC LIMIT 1`, connectorID, externalSnapshotID, idempotencyKey); getErr != nil {
			return nil, false, fmt.Errorf("load duplicate ingestion: %w", getErr)
		}
		run, getErr := r.Run(ctx, connectorID, existingID)
		return run, true, getErr
	}
	run, err := r.Run(ctx, connectorID, id)
	return run, false, err
}

const ingestionColumns = `
	r.id, r.connector_id, c.data_source_id, r.external_snapshot_id,
	r.schema_version, r.idempotency_key, r.payload_sha256, r.payload,
	r.status, r.attempts, r.error_message,
	COALESCE(r.parser_snapshot_id, '') AS parser_snapshot_id,
	r.group_count, r.lesson_count, r.next_attempt_at, r.claimed_at,
	r.received_at, r.completed_at`

func (r *ConnectorRepository) Run(ctx context.Context, connectorID, runID string) (*domain.ConnectorIngestionRun, error) {
	var run domain.ConnectorIngestionRun
	err := r.db.GetContext(ctx, &run, `SELECT `+ingestionColumns+`
		FROM connector_ingestion_runs r
		JOIN connector_clients c ON c.id=r.connector_id
		WHERE r.connector_id=$1 AND r.id=$2`, connectorID, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConnectorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get connector ingestion %s: %w", runID, err)
	}
	return &run, nil
}

func (r *ConnectorRepository) ListRuns(ctx context.Context, connectorID string, limit int) ([]domain.ConnectorIngestionRun, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	items := []domain.ConnectorIngestionRun{}
	if err := r.db.SelectContext(ctx, &items, `SELECT `+ingestionColumns+`
		FROM connector_ingestion_runs r
		JOIN connector_clients c ON c.id=r.connector_id
		WHERE ($1='' OR r.connector_id=$1)
		ORDER BY r.received_at DESC LIMIT $2`, connectorID, limit); err != nil {
		return nil, fmt.Errorf("list connector ingestions: %w", err)
	}
	for index := range items {
		items[index].Payload = nil
	}
	return items, nil
}

func (r *ConnectorRepository) ClaimNext(ctx context.Context) (*domain.ConnectorIngestionRun, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `
		UPDATE connector_ingestion_runs
		SET status='received', claimed_at=NULL, next_attempt_at=NOW()
		WHERE status='processing' AND claimed_at<NOW()-INTERVAL '10 minutes'`); err != nil {
		return nil, err
	}
	var id string
	err = tx.GetContext(ctx, &id, `
		SELECT r.id
		FROM connector_ingestion_runs r
		JOIN connector_clients c ON c.id=r.connector_id
		WHERE r.status='received' AND r.next_attempt_at<=NOW()
		  AND c.status IN ('testing','pending_review','active')
		ORDER BY r.received_at
		FOR UPDATE OF r SKIP LOCKED
		LIMIT 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE connector_ingestion_runs
		SET status='processing', claimed_at=NOW(), attempts=attempts+1
		WHERE id=$1`, id); err != nil {
		return nil, err
	}
	var run domain.ConnectorIngestionRun
	if err = tx.GetContext(ctx, &run, `SELECT `+ingestionColumns+`
		FROM connector_ingestion_runs r
		JOIN connector_clients c ON c.id=r.connector_id
		WHERE r.id=$1`, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *ConnectorRepository) Complete(
	ctx context.Context,
	runID, status, parserSnapshotID string,
	groupCount, lessonCount int,
) error {
	_, err := r.db.ExecContext(ctx, `
		WITH updated AS (
			UPDATE connector_ingestion_runs
			SET status=$2, parser_snapshot_id=NULLIF($3,''), group_count=$4,
				lesson_count=$5, error_message='', completed_at=NOW(), claimed_at=NULL
			WHERE id=$1
			RETURNING connector_id
		)
		UPDATE connector_clients c
		SET last_snapshot_at=NOW(), updated_at=NOW()
		FROM updated WHERE c.id=updated.connector_id`,
		runID, status, parserSnapshotID, groupCount, lessonCount)
	if err != nil {
		return fmt.Errorf("complete connector ingestion %s: %w", runID, err)
	}
	return nil
}

func (r *ConnectorRepository) Fail(ctx context.Context, runID string, runErr error, retryable bool) error {
	message := runErr.Error()
	if len(message) > 4000 {
		message = message[:4000]
	}
	status := domain.IngestionStatusFailed
	if retryable {
		status = domain.IngestionStatusReceived
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE connector_ingestion_runs
		SET status=$2, error_message=$3, claimed_at=NULL,
			next_attempt_at=CASE WHEN $2='received'
				THEN NOW()+LEAST(3600, 15*POWER(2, LEAST(attempts, 8))) * INTERVAL '1 second'
				ELSE next_attempt_at END,
			completed_at=CASE WHEN $2='failed' THEN NOW() ELSE NULL END
		WHERE id=$1`, runID, status, message)
	if err != nil {
		return fmt.Errorf("fail connector ingestion %s: %w", runID, err)
	}
	return nil
}

func (r *ConnectorRepository) UpdateStatus(ctx context.Context, connectorID, status string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var source struct {
		ID           string `db:"id"`
		UniversityID string `db:"university_id"`
	}
	if err = tx.GetContext(ctx, &source, `
		SELECT ds.id, ds.university_id
		FROM connector_clients c
		JOIN data_sources ds ON ds.id=c.data_source_id
		WHERE c.id=$1
		FOR UPDATE OF c, ds`, connectorID); errors.Is(err, sql.ErrNoRows) {
		return ErrConnectorNotFound
	} else if err != nil {
		return fmt.Errorf("load connector source: %w", err)
	}
	if status == domain.ConnectorStatusActive {
		if _, err = tx.ExecContext(ctx,
			`SELECT pg_advisory_xact_lock(hashtext('scheduler-source-activation'), hashtext($1))`,
			source.UniversityID,
		); err != nil {
			return fmt.Errorf("lock connector university: %w", err)
		}
		var competingSource string
		err = tx.GetContext(ctx, &competingSource, `
			SELECT id FROM data_sources
			WHERE university_id=$1 AND lifecycle_status='active' AND id<>$2
			LIMIT 1`, source.UniversityID, source.ID)
		if err == nil {
			return fmt.Errorf("%w: %s", ErrActiveSourceConflict, competingSource)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check active university source: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE connector_clients SET status=$2, updated_at=NOW() WHERE id=$1`, connectorID, status)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrConnectorNotFound
	}
	enabled := status == domain.ConnectorStatusActive
	lifecycle := status
	if _, err = tx.ExecContext(ctx, `
		UPDATE data_sources ds SET lifecycle_status=$2, is_enabled=$3,
			archived_at=CASE WHEN $2='archived' THEN NOW() ELSE NULL END,
			updated_at=NOW()
		FROM connector_clients c WHERE c.id=$1 AND ds.id=c.data_source_id`, connectorID, lifecycle, enabled); err != nil {
		if IsActiveSourceConflict(err) {
			return ErrActiveSourceConflict
		}
		return err
	}
	if enabled {
		if _, err = tx.ExecContext(ctx, `
			UPDATE universities u SET is_active=TRUE, updated_at=NOW()
			FROM data_sources ds JOIN connector_clients c ON c.data_source_id=ds.id
			WHERE c.id=$1 AND u.id=ds.university_id`, connectorID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *ConnectorRepository) RotateKey(ctx context.Context, connectorID, keyID string, publicKey []byte) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE connector_clients
		SET key_id=$2, public_key=$3, updated_at=NOW()
		WHERE id=$1 AND status<>'archived'`, connectorID, keyID, publicKey)
	if err != nil {
		return fmt.Errorf("rotate connector key: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrConnectorNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	type sqlState interface{ SQLState() string }
	var state sqlState
	return errors.As(err, &state) && state.SQLState() == "23505"
}

func IsActiveSourceConflict(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) &&
		postgresError.Code == "23505" &&
		postgresError.ConstraintName == "idx_data_sources_one_active_per_university"
}
