package connectorapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
)

type Service struct {
	repository *repository.ConnectorRepository
	parser     *service.ParserService
}

func NewService(repository *repository.ConnectorRepository, parser *service.ParserService) *Service {
	return &Service{repository: repository, parser: parser}
}

func (s *Service) ProcessNext(ctx context.Context) (bool, error) {
	run, err := s.repository.ClaimNext(ctx)
	if err != nil || run == nil {
		return false, err
	}
	if err = s.process(ctx, run); err != nil {
		retryable := run.Attempts < 5 && !isPermanentIngestionError(err)
		if recordErr := s.repository.Fail(ctx, run.ID, err, retryable); recordErr != nil {
			slog.Error("connector: failed to record ingestion failure", "run_id", run.ID, "err", recordErr)
		}
		return true, err
	}
	return true, nil
}

func (s *Service) process(ctx context.Context, run *domain.ConnectorIngestionRun) error {
	var input connector.Snapshot
	if err := json.Unmarshal(run.Payload, &input); err != nil {
		return permanentError{fmt.Errorf("decode connector snapshot: %w", err)}
	}
	if err := connector.Validate(input); err != nil {
		return permanentError{err}
	}
	client, err := s.repository.Get(ctx, run.ConnectorID)
	if err != nil {
		return err
	}
	if input.Institution.ExternalID != client.UniversityID {
		return permanentError{fmt.Errorf(
			"institution.external_id %q does not match connector university %q",
			input.Institution.ExternalID, client.UniversityID,
		)}
	}
	payload, err := convertSnapshot(client.DataSourceID, client.UniversityID, input)
	if err != nil {
		return permanentError{err}
	}
	snapshot, err := s.parser.IngestExternalSnapshot(ctx, client.DataSourceID, payload)
	if err != nil {
		return err
	}
	status := domain.IngestionStatusStaged
	switch snapshot.Status {
	case domain.SnapshotStatusPublished:
		status = domain.IngestionStatusPublished
	case domain.SnapshotStatusQuarantined:
		status = domain.IngestionStatusQuarantined
	case domain.SnapshotStatusRejected:
		status = domain.IngestionStatusRejected
	}
	return s.repository.Complete(
		ctx, run.ID, status, snapshot.ID, snapshot.GroupCount, snapshot.LessonCount,
	)
}

type permanentError struct{ error }

func isPermanentIngestionError(err error) bool {
	var permanent permanentError
	return errors.As(err, &permanent) || connector.IsValidationError(err)
}
