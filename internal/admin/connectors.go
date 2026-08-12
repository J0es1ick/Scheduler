package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

func (s *Store) Connectors(ctx context.Context) ([]domain.ConnectorClient, error) {
	return repository.NewConnectorRepository(s.db).List(ctx)
}

func (s *Store) ConnectorRuns(ctx context.Context, connectorID string, limit int) ([]domain.ConnectorIngestionRun, error) {
	return repository.NewConnectorRepository(s.db).ListRuns(ctx, connectorID, limit)
}

func (s *Store) CreateConnector(ctx context.Context, params repository.CreateConnectorParams) (*domain.ConnectorClient, error) {
	return repository.NewConnectorRepository(s.db).Create(ctx, params)
}

func (s *Store) Connector(ctx context.Context, id string) (*domain.ConnectorClient, error) {
	return repository.NewConnectorRepository(s.db).Get(ctx, id)
}

func (s *Store) UpdateConnectorStatus(ctx context.Context, id, status string) error {
	return repository.NewConnectorRepository(s.db).UpdateStatus(ctx, id, status)
}

func (s *Store) RotateConnectorKey(ctx context.Context, id, keyID string, publicKey []byte) error {
	return repository.NewConnectorRepository(s.db).RotateKey(ctx, id, keyID, publicKey)
}

func (s *Store) UpdateConnectorPolicy(ctx context.Context, connectorID string, policy domain.SourceQualityPolicy) error {
	payload, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE data_sources ds SET quality_policy=$2::jsonb, updated_at=NOW()
		FROM connector_clients c
		WHERE c.id=$1 AND ds.id=c.data_source_id`, connectorID, payload)
	if err != nil {
		return fmt.Errorf("update connector quality policy: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ConnectorActivationCandidate(ctx context.Context, connectorID string) (string, string, string, error) {
	connector, err := repository.NewConnectorRepository(s.db).Get(ctx, connectorID)
	if err != nil {
		return "", "", "", err
	}
	var candidate struct {
		RunID      string `db:"run_id"`
		SnapshotID string `db:"snapshot_id"`
		Status     string `db:"status"`
	}
	if connector.IntegrationMode != domain.IntegrationModeExternalPush {
		err = s.db.GetContext(ctx, &candidate, `
			SELECT '' AS run_id, ps.id AS snapshot_id, ps.status
			FROM parser_snapshots ps
			WHERE ps.data_source_id=$1 AND ps.publishable AND ps.status='approved'
			ORDER BY ps.created_at DESC LIMIT 1`, connector.DataSourceID)
		if err != nil {
			return "", "", "", err
		}
		return candidate.RunID, candidate.SnapshotID, candidate.Status, nil
	}
	err = s.db.GetContext(ctx, &candidate, `
		SELECT r.id AS run_id, r.parser_snapshot_id AS snapshot_id, ps.status
		FROM connector_ingestion_runs r
		JOIN parser_snapshots ps ON ps.id=r.parser_snapshot_id
		WHERE r.connector_id=$1
		  AND r.status='staged'
		  AND ps.publishable
		  AND ps.status='approved'
		ORDER BY r.received_at DESC LIMIT 1`, connectorID)
	if errors.Is(err, repository.ErrConnectorNotFound) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", err
	}
	return candidate.RunID, candidate.SnapshotID, candidate.Status, nil
}
