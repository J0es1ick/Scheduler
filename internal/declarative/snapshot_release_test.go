package declarative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/snapshotconvert"
)

func TestPullRetainsCompleteSnapshot(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/connector/valid-example.json")
	if err != nil {
		t.Fatal(err)
	}
	var document connector.Snapshot
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(raw) }))
	defer server.Close()
	parser := newParser(Config{URL: server.URL, UniversityID: document.Institution.ExternalID})
	parser.client = server.Client()
	push, err := snapshotconvert.Convert("source", document.Institution.ExternalID, document)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err = parser.FetchGroups(context.Background()); err != nil {
			t.Fatal(err)
		}
		full := parser.FullSnapshot()
		if full == nil || !reflect.DeepEqual(*full, document) {
			t.Fatal("pull lost document metadata")
		}
		pull, err := snapshotconvert.Convert("source", document.Institution.ExternalID, *full)
		if err != nil {
			t.Fatal(err)
		}
		for i := range push.Groups {
			for j := range push.Groups[i].Lessons {
				push.Groups[i].Lessons[j].UpdatedAt = time.Time{}
				pull.Groups[i].Lessons[j].UpdatedAt = time.Time{}
			}
		}
		if !reflect.DeepEqual(push, pull) {
			t.Fatal("push and repeated pull produced different calendar/identities")
		}
	}
}
