package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"hr-cli/internal/errs"
)

func Write(event string, payload map[string]any) *errs.Error {
	dir := filepath.Join(".", ".hr-cli", "audit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Config("audit_write_failed", err.Error())
	}
	record := map[string]any{
		"event":      event,
		"created_at": time.Now().Format("2006-01-02 15:04:05"),
		"payload":    payload,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return errs.Config("audit_encode_failed", err.Error())
	}
	path := filepath.Join(dir, time.Now().Format("20060102")+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return errs.Config("audit_write_failed", err.Error())
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return errs.Config("audit_write_failed", err.Error())
	}
	return nil
}
