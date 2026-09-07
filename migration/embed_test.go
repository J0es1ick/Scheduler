package migration

import (
	"bytes"
	"testing"
)

func TestMigrationChecksumsUsePortableLineEndings(t *testing.T) {
	entries, err := Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		body, err := Files.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte("\r")) {
			t.Errorf("%s contains CR bytes: migration checksums would vary by checkout", entry.Name())
		}
	}
}
