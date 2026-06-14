package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hr-cli/internal/auth"
	"hr-cli/internal/build"
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
)

const tableName = "hr_cli_audit_log"

const ddl = `CREATE TABLE IF NOT EXISTS hr_cli_audit_log (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  event         VARCHAR(64) NOT NULL,
  operator_eid  INT NULL,
  operator_name VARCHAR(100) NULL,
  operator_role VARCHAR(20) NULL,
  target_eid    INT NULL,
  preview_id    VARCHAR(40) NULL,
  payload_json  MEDIUMTEXT NULL,
  client_host   VARCHAR(120) NULL,
  client_user   VARCHAR(120) NULL,
  cli_version   VARCHAR(40) NULL,
  db_env        VARCHAR(20) NULL,
  KEY idx_operator_created (operator_eid, created_at),
  KEY idx_event_created    (event, created_at),
  KEY idx_target_created   (target_eid, created_at),
  KEY idx_preview          (preview_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='hr-cli audit trail'`

var (
	tableEnsured     bool
	tableEnsureMutex sync.Mutex
)

// Write records an audit event. It writes to the local JSONL file unconditionally,
// then attempts a DB insert into hr_cli_audit_log. DB failures degrade to a stderr
// warning so business operations are never blocked by audit issues.
func Write(event string, payload map[string]any) *errs.Error {
	if err := writeFile(event, payload); err != nil {
		return err
	}
	if err := writeDB(event, payload); err != nil {
		fmt.Fprintf(os.Stderr, "hr-cli audit warning: db write failed for event %s: %s\n", event, err.Message)
	}
	return nil
}

func writeFile(event string, payload map[string]any) *errs.Error {
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

func writeDB(event string, payload map[string]any) *errs.Error {
	conn, cfg, openErr := db.Open()
	if openErr != nil {
		return openErr
	}
	defer conn.Close()
	if err := ensureTable(conn); err != nil {
		return err
	}
	op := operatorFromPayload(payload)
	host, _ := os.Hostname()
	user := firstNonEmpty(os.Getenv("USERNAME"), os.Getenv("USER"))
	body, err := json.Marshal(payload)
	if err != nil {
		return errs.DB("audit_encode_failed", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, execErr := conn.ExecContext(ctx, `INSERT INTO `+tableName+`
	  (event, operator_eid, operator_name, operator_role, target_eid, preview_id,
	   payload_json, client_host, client_user, cli_version, db_env)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event,
		nullableInt(op.EID),
		nullableStr(op.Name),
		nullableStr(op.Role),
		nullableInt(stringValue(payload, "target_eid", "EID")),
		nullableStr(stringValue(payload, "preview_id")),
		string(body),
		nullableStr(host),
		nullableStr(user),
		nullableStr(build.Version),
		nullableStr(cfg.Env),
	)
	if execErr != nil {
		return errs.DB("audit_insert_failed", execErr.Error())
	}
	return nil
}

func ensureTable(conn *sql.DB) *errs.Error {
	tableEnsureMutex.Lock()
	defer tableEnsureMutex.Unlock()
	if tableEnsured {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.ExecContext(ctx, ddl); err != nil {
		return errs.DB("audit_table_create_failed", err.Error())
	}
	tableEnsured = true
	return nil
}

func operatorFromPayload(payload map[string]any) auth.Operator {
	if payload == nil {
		return auth.CurrentOperator()
	}
	if raw, ok := payload["operator"]; ok {
		if op, ok := raw.(auth.Operator); ok {
			return op
		}
		if m, ok := raw.(map[string]any); ok {
			return auth.Operator{
				EID:    fmt.Sprint(m["eid"]),
				Name:   fmt.Sprint(m["name"]),
				Role:   fmt.Sprint(m["role"]),
				Badge:  fmt.Sprint(m["badge"]),
				Source: fmt.Sprint(m["source"]),
			}
		}
	}
	return auth.CurrentOperator()
}

func stringValue(payload map[string]any, keys ...string) string {
	if payload == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := payload[key]; ok && v != nil {
			s := fmt.Sprint(v)
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	if target, ok := payload["target"].(map[string]any); ok {
		for _, key := range keys {
			if v, ok := target[key]; ok && v != nil {
				s := fmt.Sprint(v)
				if s != "" && s != "<nil>" {
					return s
				}
			}
		}
	}
	return ""
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
