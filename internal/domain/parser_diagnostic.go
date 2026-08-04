package domain

import (
	"encoding/json"
	"time"
)

type ParserDiagnostic struct {
	ID              string          `db:"id"`
	ParseLogID      string          `db:"parse_log_id"`
	DataSourceID    string          `db:"data_source_id"`
	Stage           string          `db:"stage"`
	Category        string          `db:"category"`
	Summary         string          `db:"summary"`
	GroupID         string          `db:"group_id"`
	HTTPStatus      int             `db:"http_status"`
	ContentType     string          `db:"content_type"`
	ResponseSize    int             `db:"response_size"`
	ResponseSHA256  string          `db:"response_sha256"`
	ResponsePreview string          `db:"response_preview"`
	Occurrences     int             `db:"occurrences"`
	Metadata        json.RawMessage `db:"metadata"`
	CreatedAt       time.Time       `db:"created_at"`
}
