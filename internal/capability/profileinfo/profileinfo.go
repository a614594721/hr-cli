package profileinfo

import (
	"strings"

	"hr-cli/internal/auth"
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/preview"
	"hr-cli/internal/redact"
)

var normalFields = map[string]bool{
	"nickname": true, "address": true, "household_address": true,
	"emergency_contact": true, "emergency_phone": true, "emergency_relation": true,
	"marital_status": true, "computer_preference": true, "personal_intro": true,
}

var sensitiveFields = map[string]bool{
	"id_number": true, "bank_card": true, "bank_name": true, "branch_name": true,
	"bank_code": true, "provident_fund_account": true, "phone": true,
}

func Preview(userID int, setValues []string, sensitive bool) (preview.Payload, *errs.Error) {
	changesIn, err := parseSets(setValues)
	if err != nil {
		return preview.Payload{}, err
	}
	if len(changesIn) == 0 {
		return preview.Payload{}, errs.Validation("missing_changes", "provide at least one --set field=value")
	}
	operator := auth.CurrentOperator()
	for field := range changesIn {
		if !normalFields[field] && !sensitiveFields[field] {
			e := errs.Authorization("field_denied", "one or more fields are not in the editable whitelist")
			e.Param = "--set " + field
			return preview.Payload{}, e
		}
		if sensitiveFields[field] && (!sensitive || operator.Role != "HR_ADMIN") {
			e := errs.Authorization("sensitive_field_denied", "sensitive fields require HR_ADMIN and --sensitive")
			e.Param = "--set " + field
			return preview.Payload{}, e
		}
	}
	cols := make([]string, 0, len(changesIn))
	for field := range changesIn {
		cols = append(cols, "`"+field+"`")
	}
	query := "SELECT id, user_id, name, " + strings.Join(cols, ", ") + " FROM personal_info WHERE user_id=?"
	row, rowErr := db.QueryOne(query, userID)
	if rowErr != nil {
		return preview.Payload{}, rowErr
	}
	if row == nil {
		e := errs.Validation("not_found", "personal_info row not found")
		e.Param = "--user-id"
		return preview.Payload{}, e
	}
	var changes []map[string]any
	for field, value := range changesIn {
		changes = append(changes, map[string]any{
			"field":     field,
			"old":       displayValue(field, row[field]),
			"new":       displayValue(field, value),
			"sensitive": sensitiveFields[field],
		})
	}
	plan := map[string]any{
		"capability": "profile_info.plan",
		"operator":   operator,
		"target": map[string]any{
			"user_id":          row["user_id"],
			"personal_info_id": row["id"],
			"name":             row["name"],
		},
		"changes":    changes,
		"write_path": []string{"personal_info", "existing trigger synchronization verification"},
		"status":     "preview_only",
	}
	return preview.Save("profile-info", plan)
}

func parseSets(values []string) (map[string]string, *errs.Error) {
	out := map[string]string{}
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			e := errs.Validation("invalid_set", "--set must use field=value")
			e.Param = "--set"
			return nil, e
		}
		out[strings.TrimSpace(key)] = value
	}
	return out, nil
}

func displayValue(field string, value any) any {
	switch field {
	case "phone", "emergency_phone":
		return redact.Phone(value)
	case "id_number", "bank_card", "provident_fund_account":
		return redact.ID(value)
	default:
		return value
	}
}
