package v1

import (
	"testing"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
)

func TestValidateManifest(t *testing.T) {
	manifest := Manifest{
		ContractVersion: ContractVersion,
		ParserID:        "example-parser",
		Version:         "1.0.0",
		DisplayName:     "Example",
		Institution: connector.Institution{
			ExternalID: "example",
			Name:       "Example University",
			Timezone:   "Europe/Moscow",
		},
		UpdateInterval: time.Hour,
	}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ParserID = "Bad ID"
	if err := ValidateManifest(manifest); err == nil {
		t.Fatal("invalid parser id was accepted")
	}
}
