//go:build integration

package repository_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

func TestPublicGroupReferenceOnlyResolvesActiveUniversityAndGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := openRepositoryIntegrationDB(t, ctx)
	universityID := "group-reference-" + uuid.NewString()
	cleanupRepositoryUniversity(t, db, universityID)
	unis := repository.NewUniversityRepository(db)
	groups := repository.NewGroupRepository(db)
	if _, err := unis.CreateUniversity(ctx, universityID, "ИГХТУ", "Университет", "", true); err != nil {
		t.Fatal(err)
	}
	groupID := universityID + ":группа:4/147"
	if _, err := groups.CreateGroup(ctx, groupID, universityID, "4/147", true); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(groupID))
	token := fmt.Sprintf("%x", digest[:8])
	if group, err := groups.GetActiveGroupByToken(ctx, token); err != nil || group == nil || group.ID != groupID {
		t.Fatalf("UTF-8 group token did not round-trip: %v %v", group, err)
	}
	for _, invalid := range []string{"", "not-a-group-token", "0123456789abcdef", "' OR 1=1 --"} {
		if group, err := groups.GetActiveGroupByToken(ctx, invalid); err != nil || group != nil {
			t.Fatalf("invalid token %q resolved group: %v %v", invalid, group, err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE groups SET is_active=FALSE WHERE id=$1`, groupID); err != nil {
		t.Fatal(err)
	}
	if group, err := groups.GetActiveGroupByToken(ctx, token); err != nil || group != nil {
		t.Fatalf("inactive group is visible: %v %v", group, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE groups SET is_active=TRUE WHERE id=$1`, groupID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE universities SET is_active=FALSE WHERE id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if group, err := groups.GetActiveGroupByToken(ctx, token); err != nil || group != nil {
		t.Fatalf("disabled university is visible: %v %v", group, err)
	}
}
