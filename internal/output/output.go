package output

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"hr-cli/internal/errs"
)

type Meta map[string]any

type Envelope struct {
	OK    bool `json:"ok"`
	Data  any  `json:"data,omitempty"`
	Error any  `json:"error,omitempty"`
	Meta  Meta `json:"meta"`
}

func Success(w io.Writer, data any, meta Meta, format string) error {
	if format == "table" {
		return Table(w, data)
	}
	return json.NewEncoder(w).Encode(Envelope{OK: true, Data: normalize(data), Meta: meta})
}

func Failure(err *errs.Error, meta Meta) {
	_ = json.NewEncoder(os.Stderr).Encode(Envelope{OK: false, Error: err, Meta: meta})
}

func normalize(v any) any {
	switch value := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[k] = normalize(item)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, 0, len(value))
		for _, row := range value {
			out = append(out, normalize(row).(map[string]any))
		}
		return out
	case []any:
		out := make([]any, 0, len(value))
		for _, item := range value {
			out = append(out, normalize(item))
		}
		return out
	case time.Time:
		return value.Format("2006-01-02 15:04:05")
	case []byte:
		return string(value)
	case sql.NullString:
		if value.Valid {
			return value.String
		}
		return nil
	case nil:
		return nil
	default:
		return value
	}
}

func Table(w io.Writer, data any) error {
	rows := extractRows(normalize(data))
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "(empty)")
		return err
	}
	headers := sortedHeaders(rows)
	widths := make(map[string]int, len(headers))
	for _, h := range headers {
		widths[h] = len(h)
	}
	for _, row := range rows {
		for _, h := range headers {
			if n := len(fmt.Sprint(row[h])); n > widths[h] {
				widths[h] = n
			}
		}
	}
	printLine := func(parts []string) {
		_, _ = fmt.Fprintln(w, "| "+strings.Join(parts, " | ")+" |")
	}
	headerParts := make([]string, 0, len(headers))
	sepParts := make([]string, 0, len(headers))
	for _, h := range headers {
		headerParts = append(headerParts, pad(h, widths[h]))
		sepParts = append(sepParts, strings.Repeat("-", widths[h]))
	}
	printLine(headerParts)
	printLine(sepParts)
	for _, row := range rows {
		parts := make([]string, 0, len(headers))
		for _, h := range headers {
			parts = append(parts, pad(fmt.Sprint(row[h]), widths[h]))
		}
		printLine(parts)
	}
	return nil
}

func extractRows(data any) []map[string]any {
	switch value := data.(type) {
	case map[string]any:
		if items, ok := value["items"]; ok {
			return extractRows(items)
		}
		return []map[string]any{value}
	case []map[string]any:
		return value
	case []any:
		rows := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	default:
		return nil
	}
}

func sortedHeaders(rows []map[string]any) []string {
	seen := map[string]bool{}
	var headers []string
	for _, row := range rows {
		for k := range row {
			if !seen[k] {
				seen[k] = true
				headers = append(headers, k)
			}
		}
	}
	sort.Strings(headers)
	return headers
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
