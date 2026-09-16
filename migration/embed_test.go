package migration

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
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

func TestLegacyChecksumsMatchOnlyKnownCRLFArtifacts(t *testing.T) {
	for line := range strings.Lines(checksumAliases) {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) != 3 {
			t.Fatalf("invalid checksum alias: %q", line)
		}
		body, err := Files.ReadFile(fields[0])
		if err != nil {
			t.Fatal(err)
		}
		canonical := fmt.Sprintf("%x", sha256.Sum256(body))
		legacy := fmt.Sprintf("%x", sha256.Sum256(bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n"))))
		if fields[1] != canonical || fields[2] != legacy {
			t.Fatalf("alias does not match the unchanged SQL with LF/CRLF: %s", fields[0])
		}
		if !IsKnownLegacyChecksum(fields[0], canonical, legacy) ||
			IsKnownLegacyChecksum("001_init.up.sql", canonical, legacy) ||
			IsKnownLegacyChecksum(fields[0], "modified", legacy) ||
			IsKnownLegacyChecksum(fields[0], canonical, "modified") {
			t.Fatal("checksum alias is not restricted to the known artifact")
		}
	}
}
