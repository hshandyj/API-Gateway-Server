package schema

import (
	"encoding/json"
	"time"
)

type DocumentResponse struct {
	DocumentId string          `json:"document_id"`
	OwnerId    string          `json:"owner_id"`
	WriteIds   []string        `json:"write_ids"`
	ReadIds    []string        `json:"read_ids"`
	Title      string          `json:"title"`
	Content    json.RawMessage `json:"content"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type DocumentRequest struct {
	DocumentId string `json:"document_id"`
}
