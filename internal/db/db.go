package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"hr-cli/internal/errs"
	"hr-cli/internal/runtime"
)

type Config struct {
	Env      string
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func EnvConfig() (Config, *errs.Error) {
	profile, hasProfile := runtime.ActiveProfile()
	cfg := Config{
		Env:      valueOrDefault(firstNonEmpty(os.Getenv("DB_ENV"), profile.DBEnv), "test"),
		Host:     firstNonEmpty(os.Getenv("DB_HOST"), profile.DBHost),
		Port:     valueOrDefault(firstNonEmpty(os.Getenv("DB_PORT"), profile.DBPort), "3306"),
		User:     firstNonEmpty(os.Getenv("DB_USER"), profile.DBUser),
		Password: os.Getenv("DB_PASSWORD"),
		Name:     firstNonEmpty(os.Getenv("DB_NAME"), profile.DBName),
	}
	if !hasProfile {
		cfg.Env = valueOrDefault(os.Getenv("DB_ENV"), "test")
	}
	var missing []string
	for key, value := range map[string]string{
		"DB_HOST": cfg.Host, "DB_PORT": cfg.Port, "DB_USER": cfg.User, "DB_PASSWORD": cfg.Password, "DB_NAME": cfg.Name,
	} {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		err := errs.Config("missing_db_env", "missing required database environment variables")
		err.Hint = "set " + strings.Join(missing, ", ")
		return cfg, err
	}
	if _, convErr := strconv.Atoi(cfg.Port); convErr != nil {
		err := errs.Config("invalid_db_port", "DB_PORT must be a number")
		err.Param = "DB_PORT"
		return cfg, err
	}
	return cfg, nil
}

func Open() (*sql.DB, Config, *errs.Error) {
	cfg, cfgErr := EnvConfig()
	if cfgErr != nil {
		return nil, cfg, cfgErr
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=10s&readTimeout=30s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, cfg, errs.DB("connect_failed", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, cfg, errs.DB("connect_failed", err.Error())
	}
	return conn, cfg, nil
}

func Meta() map[string]any {
	cfg, err := EnvConfig()
	if err != nil {
		return map[string]any{}
	}
	return map[string]any{"db_env": cfg.Env, "db_host": cfg.Host, "db_name": cfg.Name}
}

func QueryRows(query string, args ...any) ([]map[string]any, *errs.Error) {
	conn, _, openErr := Open()
	if openErr != nil {
		return nil, openErr
	}
	defer conn.Close()
	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, errs.DB("query_failed", err.Error())
	}
	defer rows.Close()
	return scanRows(rows)
}

func QueryOne(query string, args ...any) (map[string]any, *errs.Error) {
	rows, err := QueryRows(query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func RawQuery(sqlText string, args []string, limit int) ([]map[string]any, *errs.Error) {
	if err := EnsureReadOnly(sqlText); err != nil {
		return nil, err
	}
	params := make([]any, 0, len(args))
	for _, arg := range args {
		params = append(params, arg)
	}
	rows, err := QueryRows(strings.ReplaceAll(sqlText, "?", "?"), params...)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(rows) > limit {
		return rows[:limit], nil
	}
	return rows, nil
}

func WithConn(fn func(context.Context, *sql.Conn) *errs.Error) *errs.Error {
	conn, _, openErr := Open()
	if openErr != nil {
		return openErr
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sqlConn, err := conn.Conn(ctx)
	if err != nil {
		return errs.DB("connect_failed", err.Error())
	}
	defer sqlConn.Close()
	return fn(ctx, sqlConn)
}

func ExecOnConn(ctx context.Context, conn *sql.Conn, query string, args ...any) *errs.Error {
	if _, err := conn.ExecContext(ctx, query, args...); err != nil {
		return errs.DB("exec_failed", err.Error())
	}
	return nil
}

func QueryOneOnConn(ctx context.Context, conn *sql.Conn, query string, args ...any) (map[string]any, *errs.Error) {
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errs.DB("query_failed", err.Error())
	}
	defer rows.Close()
	scanned, scanErr := scanRows(rows)
	if scanErr != nil {
		return nil, scanErr
	}
	if len(scanned) == 0 {
		return nil, nil
	}
	return scanned[0], nil
}

func EnsureReadOnly(sqlText string) *errs.Error {
	cleaned := strings.TrimSpace(regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(sqlText, " "))
	if cleaned == "" {
		err := errs.Policy("raw_write_denied", "raw diagnostics only allow SELECT, SHOW, DESCRIBE, DESC, or EXPLAIN")
		err.Param = "--sql"
		return err
	}
	first := regexp.MustCompile(`(?i)^[a-z]+`).FindString(cleaned)
	allowed := map[string]bool{"select": true, "show": true, "describe": true, "desc": true, "explain": true}
	if !allowed[strings.ToLower(first)] {
		err := errs.Policy("raw_write_denied", "raw diagnostics only allow SELECT, SHOW, DESCRIBE, DESC, or EXPLAIN")
		err.Param = "--sql"
		return err
	}
	if strings.Contains(strings.TrimRight(cleaned, "; \t\r\n"), ";") {
		err := errs.Policy("multi_statement_denied", "raw diagnostics allow one statement only")
		err.Param = "--sql"
		return err
	}
	forbidden := regexp.MustCompile(`(?i)\b(insert|update|delete|replace|alter|drop|truncate|create|call|grant|revoke)\b`).FindString(cleaned)
	if forbidden != "" {
		err := errs.Policy("raw_write_denied", "forbidden SQL token: "+forbidden)
		err.Param = "--sql"
		return err
	}
	return nil
}

func scanRows(rows *sql.Rows) ([]map[string]any, *errs.Error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, errs.DB("scan_failed", err.Error())
	}
	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, errs.DB("scan_failed", err.Error())
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			switch value := values[i].(type) {
			case []byte:
				row[col] = string(value)
			default:
				row[col] = value
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.DB("scan_failed", err.Error())
	}
	return result, nil
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
