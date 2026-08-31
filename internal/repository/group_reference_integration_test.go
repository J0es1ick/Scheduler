//go:build integration

package repository_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
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
	for _, invalid := range []string{"", "not-a-group-token", strings.Repeat("0", 16), "' OR 1=1 --"} {
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

func TestConcurrentSubscriptionProfileChanges(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	universities := repository.NewUniversityRepository(db)
	groups := repository.NewGroupRepository(db)
	users := repository.NewUserRepository(db)
	subscriptions := repository.NewSubscriptionRepository(db)
	if _, err := universities.CreateUniversity(ctx, "u", "U", "University", "", true); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		if _, err := groups.CreateGroup(ctx, id, "u", id, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := users.CreateUser(ctx, "user", "user", false); err != nil {
		t.Fatal(err)
	}
	for round := range 20 {
		start := make(chan struct{})
		failures := make(chan error, 6)
		var wg sync.WaitGroup
		for _, id := range []string{"a", "b", "c"} {
			wg.Go(func() {
				<-start
				failures <- subscriptions.SubscribeAndSetDefault(ctx, uuid.NewString(), "user", id)
			})
		}
		wg.Go(func() { <-start; failures <- subscriptions.SetDefaultSubscribedGroup(ctx, "user", "a") })
		wg.Go(func() {
			<-start
			_, err := subscriptions.UnsubscribeAndSelectDefault(ctx, "user", "b")
			failures <- err
		})
		wg.Go(func() {
			<-start
			failures <- subscriptions.UpsertActiveGroupSubscription(ctx, uuid.NewString(), "user", "c")
		})
		close(start)
		wg.Wait()
		close(failures)
		for err := range failures {
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		if err := database.CheckSubscriptionIntegrity(ctx, db.DB); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSubscriptionRejectsUniversityDisabledDuringRequest(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO universities (id,name,full_name,schedule_url,is_active) VALUES ('u','U','U','',TRUE);
		INSERT INTO groups (id,university_id,name,is_active) VALUES ('g','u','G',TRUE);
		INSERT INTO users (id) VALUES ('user');`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE universities SET is_active=FALSE WHERE id='u'`); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		finished <- repository.NewSubscriptionRepository(db).SubscribeAndSetDefault(ctx, "sub", "user", "g")
	}()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-finished; !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("disabled university accepted: %v", err)
	}
	if err = database.CheckSubscriptionIntegrity(ctx, db.DB); err != nil {
		t.Fatal(err)
	}
}

func TestInactiveChatGroupPreservesProfile(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO universities (id,name,full_name,schedule_url,is_active) VALUES ('u','U','U','',TRUE);
		INSERT INTO groups (id,university_id,name,is_active) VALUES ('g','u','G',TRUE);`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewChatProfileRepository(db)
	if err := repo.Upsert(ctx, "chat", "Chat", "g", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE universities SET is_active=FALSE WHERE id='u'`); err != nil {
		t.Fatal(err)
	}
	profile, err := repo.Get(ctx, "chat")
	if err != nil || profile == nil || !profile.Unavailable || profile.DefaultGroupID != "g" {
		t.Fatalf("inactive profile=%+v error=%v", profile, err)
	}
}

func TestSubscriptionsWithoutDefaultRemainReadable(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO universities (id,name,full_name,schedule_url,is_active) VALUES ('u','U','U','',TRUE);
		INSERT INTO groups (id,university_id,name,is_active) VALUES ('g','u','G',TRUE);
		INSERT INTO users (id) VALUES ('user');
		INSERT INTO subscriptions (id,user_id,object_id,object_type) VALUES ('sub','user','g','group');`); err != nil {
		t.Fatal(err)
	}
	items, err := repository.NewSubscriptionRepository(db).GetGroupSubscriptions(ctx, "user")
	if err != nil || len(items) != 1 || items[0].IsDefault {
		t.Fatalf("subscriptions without default: %+v %v", items, err)
	}
}
