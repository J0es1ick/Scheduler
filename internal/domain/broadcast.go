package domain

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"
)

type Broadcast struct {
	ID           string                `db:"id" json:"id"`
	AuthorID     string                `db:"author_id" json:"author_id"`
	Document     json.RawMessage       `db:"document" json:"document"`
	Body         string                `db:"body" json:"body"`
	AudienceMode string                `db:"audience_mode" json:"audience_mode"`
	Status       string                `db:"status" json:"status"`
	Version      int                   `db:"version" json:"version"`
	CreatedAt    time.Time             `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time             `db:"updated_at" json:"updated_at"`
	SentAt       *time.Time            `db:"sent_at" json:"sent_at"`
	CompletedAt  *time.Time            `db:"completed_at" json:"completed_at"`
	Attachments  []BroadcastAttachment `json:"attachments"`
	Recipients   []BroadcastRecipient  `json:"recipients"`
	Counts       map[string]int        `json:"counts"`
}

type BroadcastAttachment struct {
	ID             string `db:"id" json:"id"`
	BroadcastID    string `db:"broadcast_id" json:"-"`
	Filename       string `db:"filename" json:"filename"`
	MediaType      string `db:"media_type" json:"media_type"`
	ContentType    string `db:"content_type" json:"content_type"`
	Size           int    `db:"size" json:"size"`
	Position       int    `db:"position" json:"position"`
	TelegramFileID string `db:"telegram_file_id" json:"-"`
	Data           []byte `db:"data" json:"-"`
}

type BroadcastRecipient struct {
	UserID   string `db:"user_id" json:"user_id"`
	Username string `db:"username" json:"username"`
	Status   string `db:"status" json:"status"`
	Reason   string `db:"reason" json:"reason"`
	Parts    int    `db:"parts" json:"parts"`
}

type BroadcastDocument struct {
	Type    string                     `json:"type"`
	Text    string                     `json:"text,omitempty"`
	Content []BroadcastDocument        `json:"content,omitempty"`
	Marks   []BroadcastMark            `json:"marks,omitempty"`
	Attrs   map[string]json.RawMessage `json:"attrs,omitempty"`
}

type BroadcastMark struct {
	Type  string            `json:"type"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

func RenderBroadcastDocument(raw json.RawMessage) (string, error) {
	if len(raw) > 100000 {
		return "", fmt.Errorf("слишком большой документ")
	}
	var doc BroadcastDocument
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Type != "doc" {
		return "", fmt.Errorf("неверный формат документа")
	}
	var plain strings.Builder
	var render func(BroadcastDocument, int, string) (string, error)
	render = func(n BroadcastDocument, depth int, parent string) (string, error) {
		if depth > 12 {
			return "", fmt.Errorf("слишком глубокая структура текста")
		}
		if n.Type == "text" {
			if len(n.Content) > 0 || (parent != "paragraph") {
				return "", fmt.Errorf("неверная структура текста")
			}
			plain.WriteString(n.Text)
			text := html.EscapeString(n.Text)
			for _, m := range n.Marks {
				switch m.Type {
				case "bold":
					text = "<b>" + text + "</b>"
				case "italic":
					text = "<i>" + text + "</i>"
				case "link":
					u, err := url.Parse(m.Attrs["href"])
					if err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto") || ((u.Scheme == "https" || u.Scheme == "http") && u.Host == "") {
						return "", fmt.Errorf("ссылка должна начинаться с https://, http:// или mailto:")
					}
					text = "<a href=\"" + html.EscapeString(u.String()) + "\">" + text + "</a>"
				default:
					return "", fmt.Errorf("неподдерживаемое оформление: %s", m.Type)
				}
			}
			return text, nil
		}
		if n.Type == "hardBreak" && parent == "paragraph" {
			plain.WriteString("\n")
			return "\n", nil
		}
		valid := n.Type == "doc" && depth == 0 || n.Type == "paragraph" && (parent == "doc" || parent == "listItem") || (n.Type == "bulletList" || n.Type == "orderedList") && (parent == "doc" || parent == "listItem") || n.Type == "listItem" && (parent == "bulletList" || parent == "orderedList")
		if !valid || n.Text != "" || len(n.Marks) > 0 {
			return "", fmt.Errorf("неподдерживаемый элемент: %s", n.Type)
		}
		start := 1
		if n.Type == "orderedList" && len(n.Attrs["start"]) > 0 {
			if err := json.Unmarshal(n.Attrs["start"], &start); err != nil || start < 1 || start > 1000000 {
				return "", fmt.Errorf("неверное начало нумерованного списка")
			}
		}
		var out strings.Builder
		for i, child := range n.Content {
			if i > 0 && n.Type != "paragraph" {
				out.WriteString("\n")
				plain.WriteString("\n")
			}
			if n.Type == "bulletList" {
				out.WriteString("• ")
				plain.WriteString("• ")
			}
			if n.Type == "orderedList" {
				prefix := fmt.Sprintf("%d. ", i+start)
				out.WriteString(prefix)
				plain.WriteString(prefix)
			}
			part, err := render(child, depth+1, n.Type)
			if err != nil {
				return "", err
			}
			out.WriteString(part)
		}
		return out.String(), nil
	}
	body, err := render(doc, 0, "")
	if err != nil {
		return "", err
	}
	text := plain.String()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("введите текст сообщения")
	}
	if len(utf16.Encode([]rune(text))) > 4096 {
		return "", fmt.Errorf("текст сообщения превышает 4096 символов")
	}
	return body, nil
}
