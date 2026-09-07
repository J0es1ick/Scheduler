package v1

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedConformanceLimits(t *testing.T) {
	var cases []struct {
		Name            string
		Groups, Lessons int
		Valid           bool
	}
	raw, err := os.ReadFile("../../testdata/connector-limits.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("../../testdata/connector/valid-example.json")
	if err != nil {
		t.Fatal(err)
	}
	var base Snapshot
	if err = json.Unmarshal(raw, &base); err != nil {
		t.Fatal(err)
	}
	for _, sample := range cases {
		t.Run(sample.Name, func(t *testing.T) {
			snapshot := base
			snapshot.Groups = nil
			for i := 0; i < sample.Groups; i++ {
				group := base.Groups[0]
				group.ExternalID = fmt.Sprintf("g%d", i)
				group.Name = group.ExternalID
				group.Lessons = []Lesson{}
				snapshot.Groups = append(snapshot.Groups, group)
			}
			for i := 0; i < sample.Lessons; i++ {
				lesson := base.Groups[0].Lessons[0]
				lesson.ExternalID = fmt.Sprintf("l%d", i)
				snapshot.Groups[0].Lessons = append(snapshot.Groups[0].Lessons, lesson)
			}
			if err := Validate(snapshot); (err == nil) != sample.Valid {
				t.Fatalf("accepted=%t want=%t: %v", err == nil, sample.Valid, err)
			}
		})
	}
}

func TestSharedConformanceCorpus(t *testing.T) {
	files, err := filepath.Glob("../../testdata/connector/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("missing corpus: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot Snapshot
			err = json.Unmarshal(raw, &snapshot)
			if err == nil {
				err = Validate(snapshot)
			}
			valid := strings.HasPrefix(filepath.Base(file), "valid-")
			if (err == nil) != valid {
				t.Fatalf("accepted=%v, want %v: %v", err == nil, valid, err)
			}
		})
	}
}
