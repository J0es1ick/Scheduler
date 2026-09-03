package main

import (
	"slices"
	"testing"
)

func TestDatabaseRolesCoverRuntimeAndRecoveryAccounts(t *testing.T) {
	want := []string{"MIGRATOR", "BOT", "PARSER", "PRIVACY", "ADMIN", "SITE", "BACKUP", "RESTORE"}
	if !slices.Equal(databaseRoles(), want) {
		t.Fatalf("database roles = %v, want %v", databaseRoles(), want)
	}
}
