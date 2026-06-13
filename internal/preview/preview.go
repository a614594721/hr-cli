package preview

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"hr-cli/internal/errs"
)

type Payload struct {
	PreviewID string `json:"preview_id"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
	Plan      any    `json:"plan"`
	Path      string `json:"path,omitempty"`
}

func Save(kind string, plan any) (Payload, *errs.Error) {
	id := newID()
	dir := filepath.Join(".", ".hr-cli", "previews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Payload{}, errs.Config("preview_store_failed", err.Error())
	}
	payload := Payload{
		PreviewID: id,
		Kind:      kind,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		Plan:      plan,
	}
	path := filepath.Join(dir, id+".json")
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return Payload{}, errs.Config("preview_encode_failed", err.Error())
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Payload{}, errs.Config("preview_store_failed", err.Error())
	}
	return payload, nil
}

func Load(id string) (Payload, *errs.Error) {
	path := filepath.Join(".", ".hr-cli", "previews", id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		e := errs.Validation("not_found", "preview not found")
		e.Param = "preview-id"
		return Payload{}, e
	}
	var payload Payload
	if err := json.Unmarshal(data, &payload); err != nil {
		return Payload{}, errs.Validation("invalid_preview", err.Error())
	}
	abs, _ := filepath.Abs(path)
	payload.Path = abs
	return payload, nil
}

func newID() string {
	buf := make([]byte, 3)
	_, _ = rand.Read(buf)
	return time.Now().Format("20060102-150405-") + hex.EncodeToString(buf)
}
