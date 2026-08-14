package connectorapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
	processCtx, cancelProcess := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-processCtx.Done():
				heartbeatDone <- nil
				return
			case <-ticker.C:
				if renewErr := s.repository.RenewClaim(processCtx, run.ID, run.ClaimToken); renewErr != nil {
					if processCtx.Err() != nil {
						heartbeatDone <- nil
						return
					}
					heartbeatDone <- renewErr
					cancelProcess()
					return
				}
			}
		}
	}()
	completion, processErr := s.process(processCtx, run)
	cancelProcess()
	heartbeatErr := <-heartbeatDone
	if heartbeatErr != nil {
		return true, fmt.Errorf("connector ingestion heartbeat: %w", heartbeatErr)
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if processErr != nil {
		retryable := run.Attempts < 5 && !isPermanentIngestionError(processErr)
		if recordErr := s.repository.Fail(ctx, run.ID, run.ClaimToken, processErr, retryable); recordErr != nil {
			slog.Error("connector: failed to record ingestion failure", "run_id", run.ID, "err", recordErr)
			return true, errors.Join(processErr, recordErr)
		}
		return true, processErr
	}
	if completion.finalized {
		return true, nil
	}
	if err = s.repository.Complete(
		ctx, run.ID, run.ClaimToken, completion.status, completion.snapshotID,
		completion.groupCount, completion.lessonCount,
	); err != nil {
		return true, err
	}
	return true, nil
}

type ingestionCompletion struct {
	status      string
	snapshotID  string
	groupCount  int
	lessonCount int
	finalized   bool
}

func (s *Service) process(ctx context.Context, run *domain.ConnectorIngestionRun) (ingestionCompletion, error) {
	var completion ingestionCompletion
	var input connector.Snapshot
	if err := json.Unmarshal(run.Payload, &input); err != nil {
		return completion, permanentError{fmt.Errorf("decode connector snapshot: %w", err)}
	}
	if err := connector.Validate(input); err != nil {
		return completion, permanentError{err}
	}
	client, err := s.repository.Get(ctx, run.ConnectorID)
	if err != nil {
		return completion, err
	}
	if input.Institution.ExternalID != client.UniversityID {
		return completion, permanentError{fmt.Errorf(
			"institution.external_id %q does not match connector university %q",
			input.Institution.ExternalID, client.UniversityID,
		)}
	}
	payload, err := convertSnapshot(client.DataSourceID, client.UniversityID, input)
	if err != nil {
		return completion, permanentError{err}
	}
	snapshot, err := s.parser.IngestClaimedExternalSnapshot(
		ctx, client.DataSourceID, payload, run.ID, run.ClaimToken,
	)
	if err != nil {
		return completion, err
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
	return ingestionCompletion{
		status: status, snapshotID: snapshot.ID,
		groupCount: snapshot.GroupCount, lessonCount: snapshot.LessonCount,
		finalized: snapshot.Status == domain.SnapshotStatusPublished,
	}, nil
}

type permanentError struct{ error }

func isPermanentIngestionError(err error) bool {
	var permanent permanentError
	return errors.As(err, &permanent) || connector.IsValidationError(err)
}
