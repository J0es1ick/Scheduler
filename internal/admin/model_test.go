package admin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPageMarshalsNilItemsAsEmptyArray(t *testing.T) {
	payload, err := json.Marshal(Page[GroupView]{
		Pagination: Pagination{Page: 1, PageSize: 20},
	})
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	if !strings.Contains(string(payload), `"items":[]`) {
		t.Fatalf("expected empty JSON array, got %s", payload)
	}
}
