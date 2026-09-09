package servicelogs

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
func testQuery() Query {
	now := time.Now()
	return Query{Since: now.Add(-time.Hour), Until: now.Add(time.Hour), Limit: 100}
}

func TestSyslogScopeAndSecretRedactionBeforeStorage(t *testing.T) {
	store := testStore(t)
	message := `{"level":"ERROR","msg":"parser: failed","dataSourceID":"ispu-main","request_id":"req-123","user_id":42,"nested":{"password":"secret-pw","token":"opaque-token"},"err":"postgres://dbuser:dbpass@database/app password='other-pw' https://api.telegram.org/bot123456:abcdefghijklmnopqrstuvwxyz/getUpdates"}`
	packet := "<27>1 " + time.Now().Format(time.RFC3339Nano) + " host scheduler/scheduler-parser-worker 123 tag - " + message
	component, at, text, err := ParseSyslog([]byte(packet), "scheduler")
	if err != nil || component != "parser-worker" {
		t.Fatalf("syslog: %s %v", component, err)
	}
	if err = store.Append(component, at, text); err != nil {
		t.Fatal(err)
	}
	file, err := store.root.Open("parser-worker.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var entry Entry
	if err = json.NewDecoder(file).Decode(&entry); err != nil {
		t.Fatal(err)
	}
	if entry.Source != "ispu-main" || entry.Module != "parser" || entry.Level != "ERROR" || entry.Fields["request_id"] != "req-123" {
		t.Fatalf("diagnostic context missing: %+v", entry)
	}
	body, _ := json.Marshal(entry)
	for _, secret := range []string{"secret-pw", "opaque-token", "dbpass", "other-pw", "abcdefghijklmnopqrstuvwxyz", `:42`} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("secret persisted: %s", body)
		}
	}
	for _, bad := range []string{strings.Replace(packet, "scheduler/scheduler-parser-worker", "foreign/bot", 1), strings.Replace(packet, "scheduler/scheduler-parser-worker", "scheduler/../../escape", 1), "invalid"} {
		if _, _, _, err := ParseSyslog([]byte(bad), "scheduler"); err == nil {
			t.Fatal("untrusted syslog accepted")
		}
	}
	if err = store.Append("../escape", at, text); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestStoredLogsFiltersPaginationAndRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	for _, message := range []string{`{"level":"INFO","msg":"ready"}`, `{"level":"ERROR","msg":"parser: denied","source":"ispu-main","err":"permission denied"}`, `{"level":"ERROR","msg":"notification worker: failed","request_id":"req-2"}`} {
		if err = store.Append("bot", at, message); err != nil {
			t.Fatal(err)
		}
	}
	store.Close()
	store, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	q := testQuery()
	q.Limit = 1
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		page, err := store.Read(context.Background(), q)
		if err != nil || len(page.Entries) != 1 {
			t.Fatalf("page %d: %+v %v", i, page, err)
		}
		if seen[page.Entries[0].ID] {
			t.Fatal("repeated log with equal timestamp")
		}
		seen[page.Entries[0].ID] = true
		if i < 2 {
			values := url.Values{"since": {q.Since.Format(time.RFC3339Nano)}, "until": {q.Until.Format(time.RFC3339Nano)}, "limit": {"1"}, "cursor": {page.NextCursor}}
			q, err = ParseQuery(values, time.Now())
			if err != nil {
				t.Fatal(err)
			}
		} else if page.NextCursor != "" {
			t.Fatal("extra page")
		}
	}
	q.Before = nil
	q.Module = "parser"
	q.Source = "ispu-main"
	q.Level = "ERROR"
	q.Search = "PERMISSION"
	page, err := store.Read(context.Background(), q)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Source != "ispu-main" {
		t.Fatalf("filters: %+v %v", page, err)
	}
}

func TestStoredLogsRotationRetentionAndConcurrentReads(t *testing.T) {
	store := testStore(t)
	store.rotateBytes = 500
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			for n := 0; n < 20; n++ {
				if err := store.Append("bot", time.Now(), "notification worker: test"); err != nil {
					t.Error(err)
				}
				if _, err := store.Read(context.Background(), testQuery()); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	dir, err := store.root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	files, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil || len(files) > 5 || len(files) < 2 {
		t.Fatalf("unbounded rotation: %d %v", len(files), err)
	}
	if err = store.Purge(time.Now().Add(8 * 24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	page, err := store.Read(context.Background(), testQuery())
	if err != nil || len(page.Entries) != 0 {
		t.Fatalf("expired logs retained: %+v %v", page, err)
	}
}

func TestStoredLogsIncompleteWritesAndUnreadableStorage(t *testing.T) {
	store := testStore(t)
	if err := store.root.WriteFile("bot.jsonl", []byte(`{"broken":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("bot", time.Now(), "recovered"); err != nil {
		t.Fatal(err)
	}
	page, err := store.Read(context.Background(), testQuery())
	if err != nil || len(page.Entries) != 1 || len(page.Warnings) == 0 {
		t.Fatalf("partial write concealed: %+v %v", page, err)
	}
	store.Close()
	rr := httptest.NewRecorder()
	store.ServeHTTP(rr, httptest.NewRequest("GET", "/logs", nil))
	if rr.Code != 503 {
		t.Fatalf("outage reported as success: %d", rr.Code)
	}
}

func TestLogCatalogSurvivesInterruptedScan(t *testing.T) {
	store := testStore(t)
	for _, component := range []string{"admin", "bot", "parser-worker"} {
		if err := store.Append(component, time.Now(), "ready"); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page, err := store.Read(ctx, testQuery())
	if err != nil || len(page.Components) != 3 || len(page.Warnings) == 0 {
		t.Fatalf("cannot select component after interrupted scan: %+v %v", page, err)
	}
}

func TestTextLogLevelsAndNotificationModule(t *testing.T) {
	for _, message := range []string{"time=2026-09-09T00:00:00Z level=error msg=failed", "ERROR: permission denied", `{"level":"ERROR","msg":"bot outbox delivery failed"}`} {
		entry := newEntry("bot", time.Now(), message)
		if entry.Level != "ERROR" {
			t.Fatalf("error level lost: %+v", entry)
		}
		if strings.Contains(message, "outbox") && entry.Module != "notification" {
			t.Fatalf("delivery module lost: %+v", entry)
		}
	}
}

func TestAdminIdentifiersAreRedactedWithoutLosingRequestID(t *testing.T) {
	entry := newEntry("admin", time.Now(), `{"msg":"admin request","admin_id":"99123","path":"/api/users/99123/role","request_id":"request-42"}`)
	data, _ := json.Marshal(entry)
	if strings.Contains(string(data), "99123") || entry.Fields["request_id"] != "request-42" {
		t.Fatalf("incorrect admin redaction: %s", data)
	}
}

func TestLogQueryAndEndpointRejectUnboundedOrUnsafeInput(t *testing.T) {
	for _, raw := range []string{"limit=0", "limit=201", "level=FATAL", "since=2026-09-09", "since=2026-09-01T00:00:00Z&until=2026-09-09T00:00:00Z", "cursor=invalid", "component=bot&component=admin", "path=/containers/json"} {
		values, _ := url.ParseQuery(raw)
		if _, err := ParseQuery(values, time.Now()); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	store := testStore(t)
	for _, method := range []string{"POST", "DELETE", "PUT"} {
		rr := httptest.NewRecorder()
		store.ServeHTTP(rr, httptest.NewRequest(method, "/logs", nil))
		if rr.Code != 404 {
			t.Errorf("accepted %s", method)
		}
	}
	out := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(out, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.root.Symlink(out, "outside.jsonl"); err == nil {
		page, err := store.Read(context.Background(), testQuery())
		if err != nil || len(page.Entries) != 0 {
			t.Fatal("followed external symlink")
		}
	}
}
