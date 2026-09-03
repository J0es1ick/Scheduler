package database

import (
	"context"
	"testing"
)

func TestApplyRuntimeGrantsRejectsInvalidRoles(t *testing.T) {
	for _, roles := range [][4]string{
		{"", "admin", "parser", "privacy"},
		{"bot", "bot", "parser", "privacy"},
		{"bot", "admin", "admin", "privacy"},
		{"bot", "admin", "parser", "bot"},
	} {
		if err := ApplyRuntimeGrants(
			context.Background(), nil, roles[0], roles[1], roles[2], roles[3],
		); err == nil {
			t.Fatalf("roles %v were accepted", roles)
		}
	}
}
