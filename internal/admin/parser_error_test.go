package admin

import (
	"strings"
	"testing"
)

func TestCompactParserErrorRemovesPerGroupList(t *testing.T) {
	message := "parser: candidate was not published because 522/522 " +
		"schedule requests failed: group 1/0: unexpected end of JSON input " +
		strings.Repeat("group 1/100: unexpected end of JSON input ", 50)
	result := compactParserError(message)
	if strings.Contains(result, "group 1/0") {
		t.Fatalf("compacted error still contains per-group details: %s", result)
	}
	if !strings.Contains(result, "диагностике") {
		t.Fatalf("compacted error does not reference diagnostics: %s", result)
	}
	if len([]rune(result)) > 750 {
		t.Fatalf("compacted error is too long: %d", len([]rune(result)))
	}
}
