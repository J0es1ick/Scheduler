package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBroadcastFormatting(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Обновление 👩‍🏫 & <текст>","marks":[{"type":"bold"}]}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Читать","marks":[{"type":"link","attrs":{"href":"https://example.test/?a=1&b=2"}}]}]}]}]}]}`)
	got, err := RenderBroadcastDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "<b>Обновление 👩‍🏫 &amp; &lt;текст&gt;</b>\n• <a href=\"https://example.test/?a=1&amp;b=2\">Читать</a>" {
		t.Fatalf("format: %s", got)
	}
}
func TestBroadcastRejectsInvalidDocuments(t *testing.T) {
	for _, raw := range []string{`{}`, `{"type":"doc","content":[]}`, `{"type":"doc","content":[{"type":"image"}]}`, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"test","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`} {
		if _, err := RenderBroadcastDocument(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, test := range []struct {
		text  string
		valid bool
	}{{strings.Repeat("a", 4096), true}, {strings.Repeat("a", 4097), false}, {strings.Repeat("😀", 2048), true}, {strings.Repeat("😀", 2049), false}} {
		raw, _ := json.Marshal(BroadcastDocument{Type: "doc", Content: []BroadcastDocument{{Type: "paragraph", Content: []BroadcastDocument{{Type: "text", Text: test.text}}}}})
		_, err := RenderBroadcastDocument(raw)
		if (err == nil) != test.valid {
			t.Fatalf("length %d: %v", len(test.text), err)
		}
	}
}

func TestBroadcastOrderedListKeepsStart(t *testing.T) {
	body, err := RenderBroadcastDocument(json.RawMessage(`{"type":"doc","content":[{"type":"orderedList","attrs":{"start":3},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Update"}]}]}]}]}`))
	if err != nil || body != "3. Update" {
		t.Fatalf("%q %v", body, err)
	}
}
