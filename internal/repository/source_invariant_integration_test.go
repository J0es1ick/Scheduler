//go:build integration

package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestOnlyOneSourceCanBeActiveForUniversity(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}

	suffix := uuid.NewString()
	universityID := "source-invariant-university-" + suffix
	activeSourceID := "source-invariant-active-" + suffix
	candidateSourceID := "source-invariant-candidate-" + suffix
	connectorID := "source-invariant-connector-" + suffix
	connectorRepo := repository.NewConnectorRepository(db)
	universityRepo := repository.NewUniversityRepository(db)
	dataSourceRepo := repository.NewDataSourceRepository(db)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_clients WHERE id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})

	if _, err = universityRepo.CreateUniversity(
		ctx, universityID, "Source invariant", "Source invariant", "https://example.test", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = dataSourceRepo.CreateDataSource(
		ctx, activeSourceID, universityID, "integration", "{}", 3600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = connectorRepo.Create(ctx, repository.CreateConnectorParams{
		ConnectorID: connectorID, SourceID: candidateSourceID, UniversityID: universityID,
		UniversityName: "Source invariant", DisplayName: "Candidate", KeyID: "key-" + suffix,
		PublicKey: []byte("integration"), CreatedBy: "integration",
		QualityPolicy: domain.DefaultSourceQualityPolicy(),
	}); err != nil {
		t.Fatal(err)
	}

	if err = connectorRepo.UpdateStatus(ctx, connectorID, domain.ConnectorStatusActive); !errors.Is(err, repository.ErrActiveSourceConflict) {
		t.Fatalf("activate competing source error = %v, want ErrActiveSourceConflict", err)
	}
	if _, err = db.ExecContext(ctx, `
		UPDATE data_sources SET lifecycle_status='suspended', is_enabled=FALSE WHERE id=$1`,
		activeSourceID,
	); err != nil {
		t.Fatal(err)
	}
	if err = connectorRepo.UpdateStatus(ctx, connectorID, domain.ConnectorStatusActive); err != nil {
		t.Fatalf("activate source after suspension: %v", err)
	}
	if _, err = db.ExecContext(ctx, `
		UPDATE data_sources SET lifecycle_status='active' WHERE id=$1`, activeSourceID,
	); !repository.IsActiveSourceConflict(err) {
		t.Fatalf("database invariant error = %v, want active-source unique violation", err)
	}
}
