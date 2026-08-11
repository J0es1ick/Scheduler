package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "snapshot.json")
	payload := `{
	  "schema_version":"1.0","snapshot_id":"test-1","generated_at":"2026-08-06T10:00:00Z",
	  "institution":{"external_id":"test","name":"Test","timezone":"Europe/Moscow"},
	  "term":{"external_id":"autumn","name":"Autumn","starts_on":"2026-09-01","ends_on":"2027-01-31"},
	  "groups":[{"external_id":"g1","name":"G1","lessons":[]}]
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"validate", path}, &output, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "valid:") {
		t.Fatalf("unexpected output: %s", output.String())
	}
}
