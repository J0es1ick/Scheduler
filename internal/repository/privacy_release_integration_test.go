//go:build integration

package repository_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

func TestDeletionRedactsActorTargetAndNestedIDs(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	deleted := "deleted:" + uuid.NewString()
	users := repository.NewUserRepository(db)
	if _, err := users.CreateUser(ctx, id, "synthetic-person", false); err != nil {
		t.Fatal(err)
	}
	auditID := uuid.NewString()
	targetAuditID := uuid.NewString()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM admin_audit_logs WHERE id IN ($1,$2)`, auditID, targetAuditID)
		db.Exec(`DELETE FROM users WHERE id=$1`, id)
	})
	for _, entry := range []struct{ id, actor, object string }{{auditID, id, "another-user"}, {targetAuditID, "another-owner", id}} {
		if _, err := db.Exec(`INSERT INTO admin_audit_logs(id,actor_id,actor_name,action,object_type,object_id,details,ip_address) VALUES($1,$2,'Actor','set_admin_role','user',$3,jsonb_build_object('user_id',$4::text,'nested',jsonb_build_array(jsonb_build_object('target',$4::text,'numeric_target',$4::numeric)),'path','/api/users/'||$4::text,'admin_role','none'),'192.0.2.1')`, entry.id, entry.actor, entry.object, id); err != nil {
			t.Fatal(err)
		}
	}
	exported, err := users.ExportUserData(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(exported)
	if strings.Contains(string(encoded), "another-owner") || strings.Contains(string(encoded), "another-user") {
		t.Fatal("export contains third-party identifiers")
	}
	var result bool
	if err = db.Get(&result, `SELECT execute_privacy_deletion($1,$2)`, id, deleted); err != nil || !result {
		t.Fatalf("delete=%t err=%v", result, err)
	}
	var remaining int
	if err = db.Get(&remaining, `SELECT count(*) FROM admin_audit_logs WHERE id IN ($1,$2) AND (actor_id=$3 OR object_id=$3 OR details::text LIKE '%'||$3||'%')`, auditID, targetAuditID, id); err != nil || remaining != 0 {
		t.Fatalf("PII remains: count=%d err=%v", remaining, err)
	}
	var markerCount int
	if err = db.Get(&markerCount, `SELECT count(*) FROM admin_audit_logs WHERE id IN ($1,$2) AND details->>'user_id'=$3`, auditID, targetAuditID, deleted); err != nil || markerCount != 2 {
		t.Fatalf("deletion markers not linked: %d %v", markerCount, err)
	}
}
