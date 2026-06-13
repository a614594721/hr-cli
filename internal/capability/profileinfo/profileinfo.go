package profileinfo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"hr-cli/internal/audit"
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

var staffRegisterDirectFields = map[string]string{
	"nickname":               "nickname",
	"address":                "RESIDENCE",
	"household_address":      "BIRTHPLACE",
	"emergency_contact":      "EMERGENCYCONTACT",
	"emergency_phone":        "EMERGENCYTELEPHONE",
	"personal_intro":         "Personal_Introduction",
	"computer_preference":    "QQ",
	"bank_card":              "banK_num",
	"branch_name":            "bank_branch",
	"bank_code":              "bank_no",
	"provident_fund_account": "AccuAccount",
	"phone":                  "MOBILE",
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
			"old_hash":  hashValue(row[field]),
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
	payload, saveErr := preview.Save("profile-info", plan)
	if saveErr != nil {
		return preview.Payload{}, saveErr
	}
	if secretErr := preview.SaveSecret(payload.PreviewID, map[string]any{"values": changesIn}); secretErr != nil {
		return preview.Payload{}, secretErr
	}
	return payload, nil
}

func Apply(previewID string, yes bool) (map[string]any, *errs.Error) {
	if !yes {
		err := errs.Confirmation("missing_confirmation", "profile-info apply requires --yes")
		err.Hint = "run profile-info +preview first, inspect the diff, then apply with --yes"
		return nil, err
	}
	cfg, cfgErr := db.EnvConfig()
	if cfgErr != nil {
		return nil, cfgErr
	}
	if cfg.Env != "test" {
		err := errs.Policy("production_write_denied", "profile-info apply is only enabled when DB_ENV=test")
		err.Hint = "production write protection is active"
		return nil, err
	}
	payload, loadErr := preview.Load(previewID)
	if loadErr != nil {
		return nil, loadErr
	}
	if payload.Kind != "profile-info" {
		err := errs.Validation("invalid_preview_kind", "preview is not a profile-info preview")
		err.Param = "preview-id"
		return nil, err
	}
	plan, ok := payload.Plan.(map[string]any)
	if !ok {
		return nil, errs.Validation("invalid_preview", "preview plan is invalid")
	}
	target, ok := plan["target"].(map[string]any)
	if !ok {
		return nil, errs.Validation("invalid_preview", "preview target is invalid")
	}
	userID, err := intValue(target["user_id"])
	if err != nil {
		return nil, err
	}
	secret, secretErr := preview.LoadSecret(payload.PreviewID)
	if secretErr != nil {
		return nil, secretErr
	}
	changes, err := collectApplyChanges(plan, secret)
	if err != nil {
		return nil, err
	}
	operator := auth.CurrentOperator()
	if err := authorizeApply(operator, changes); err != nil {
		return nil, err
	}
	if auditErr := audit.Write("profile_info.apply.start", map[string]any{
		"preview_id": payload.PreviewID,
		"operator":   operator,
		"target":     target,
		"changes":    redactedChanges(changes),
	}); auditErr != nil {
		return nil, auditErr
	}

	result := map[string]any{
		"preview_id": payload.PreviewID,
		"target":     target,
		"changes":    redactedChanges(changes),
	}
	txErr := db.WithTx(func(ctx context.Context, tx *sql.Tx) *errs.Error {
		current, err := lockPersonalInfo(ctx, tx, userID, changes)
		if err != nil {
			return err
		}
		if current == nil {
			e := errs.Validation("not_found", "personal_info row not found")
			e.Param = "preview-id"
			return e
		}
		if err := verifyOldHashes(current, changes); err != nil {
			return err
		}
		if err := updatePersonalInfo(ctx, tx, userID, changes); err != nil {
			return err
		}
		verify, err := verifyProfileInfo(ctx, tx, userID, changes)
		if err != nil {
			return err
		}
		result["verification"] = verify
		return nil
	})
	if txErr != nil {
		_ = audit.Write("profile_info.apply.failed", map[string]any{"preview_id": payload.PreviewID, "error": txErr})
		return nil, txErr
	}
	result["status"] = "applied"
	if auditErr := audit.Write("profile_info.apply.success", result); auditErr != nil {
		return nil, auditErr
	}
	return result, nil
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

type applyChange struct {
	Field     string
	New       any
	OldHash   string
	Sensitive bool
}

func collectApplyChanges(plan map[string]any, secret map[string]any) ([]applyChange, *errs.Error) {
	rawChanges, ok := plan["changes"].([]any)
	if !ok {
		return nil, errs.Validation("invalid_preview", "preview changes are invalid")
	}
	values, ok := secret["values"].(map[string]any)
	if !ok {
		return nil, errs.Validation("invalid_preview_secret", "preview secret values are invalid")
	}
	out := []applyChange{}
	for _, item := range rawChanges {
		change, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field, _ := change["field"].(string)
		if !normalFields[field] && !sensitiveFields[field] {
			e := errs.Authorization("field_denied", "one or more fields are not in the editable whitelist")
			e.Param = field
			return nil, e
		}
		oldHash, _ := change["old_hash"].(string)
		if oldHash == "" {
			err := errs.Validation("preview_missing_old_hash", "preview was created before old-value hashing was available")
			err.Hint = "re-run profile-info +preview and apply the new preview id"
			return nil, err
		}
		newValue, ok := values[field]
		if !ok {
			err := errs.Validation("preview_secret_value_missing", "preview apply value is missing")
			err.Param = field
			err.Hint = "re-run profile-info +preview and apply the new preview id"
			return nil, err
		}
		sensitive, _ := change["sensitive"].(bool)
		out = append(out, applyChange{Field: field, New: newValue, OldHash: oldHash, Sensitive: sensitive})
	}
	if len(out) == 0 {
		return nil, errs.Validation("missing_changes", "preview does not contain changes")
	}
	return out, nil
}

func lockPersonalInfo(ctx context.Context, tx *sql.Tx, userID int, changes []applyChange) (map[string]any, *errs.Error) {
	cols := []string{"id", "user_id", "name"}
	for _, change := range changes {
		cols = append(cols, "`"+change.Field+"`")
	}
	return db.QueryOneTx(ctx, tx, "SELECT "+strings.Join(cols, ", ")+" FROM personal_info WHERE user_id=? FOR UPDATE", userID)
}

func verifyOldHashes(row map[string]any, changes []applyChange) *errs.Error {
	for _, change := range changes {
		if hashValue(row[change.Field]) != change.OldHash {
			err := errs.Policy("old_value_changed", "personal_info value changed after preview")
			err.Param = change.Field
			err.Hint = "re-run profile-info +preview and review the latest diff"
			return err
		}
	}
	return nil
}

func authorizeApply(operator auth.Operator, changes []applyChange) *errs.Error {
	for _, change := range changes {
		if sensitiveFields[change.Field] && operator.Role != "HR_ADMIN" {
			err := errs.Authorization("sensitive_field_denied", "sensitive fields require HR_ADMIN at apply time")
			err.Param = change.Field
			return err
		}
	}
	return nil
}

func updatePersonalInfo(ctx context.Context, tx *sql.Tx, userID int, changes []applyChange) *errs.Error {
	set := []string{}
	args := []any{}
	for _, change := range changes {
		set = append(set, "`"+change.Field+"`=?")
		args = append(args, change.New)
	}
	args = append(args, userID)
	return db.ExecTx(ctx, tx, "UPDATE personal_info SET "+strings.Join(set, ", ")+" WHERE user_id=?", args...)
}

func verifyProfileInfo(ctx context.Context, tx *sql.Tx, userID int, changes []applyChange) (map[string]any, *errs.Error) {
	fields := []string{"user_id", "id", "name"}
	staffFields := []string{"ID", "NAME", "ky_id"}
	for _, change := range changes {
		fields = append(fields, "`"+change.Field+"`")
		if staffField, ok := staffRegisterDirectFields[change.Field]; ok {
			staffFields = append(staffFields, staffField)
		}
	}
	current, err := db.QueryOneTx(ctx, tx, "SELECT "+strings.Join(fields, ", ")+" FROM personal_info WHERE user_id=?", userID)
	if err != nil {
		return nil, err
	}
	staff, err := db.QueryOneTx(ctx, tx, "SELECT "+strings.Join(staffFields, ", ")+" FROM EPRE_STAFFREGISTER WHERE ID=?", userID)
	if err != nil {
		return nil, err
	}
	checks := []map[string]any{{"name": "personal_info_updated", "ok": current != nil}}
	if staff == nil {
		checks = append(checks, map[string]any{"name": "staffregister_row_present", "ok": false, "hint": "trigger updates existing EPRE_STAFFREGISTER rows only"})
	} else {
		checks = append(checks, map[string]any{"name": "staffregister_row_present", "ok": true})
		for _, change := range changes {
			staffField, ok := staffRegisterDirectFields[change.Field]
			if !ok {
				continue
			}
			checks = append(checks, map[string]any{
				"name":        "staffregister_field_synced",
				"field":       change.Field,
				"target":      staffField,
				"ok":          fmt.Sprint(current[change.Field]) == fmt.Sprint(staff[staffField]),
				"current":     displayValue(change.Field, current[change.Field]),
				"staff_value": displayValue(change.Field, staff[staffField]),
			})
		}
	}
	return map[string]any{"personal_info": redactRow(current, changes), "staffregister": redactStaffRow(staff, changes), "checks": checks}, nil
}

func redactedChanges(changes []applyChange) []map[string]any {
	out := []map[string]any{}
	for _, change := range changes {
		out = append(out, map[string]any{
			"field":     change.Field,
			"new":       displayValue(change.Field, change.New),
			"sensitive": change.Sensitive,
		})
	}
	return out
}

func redactRow(row map[string]any, changes []applyChange) map[string]any {
	if row == nil {
		return nil
	}
	out := map[string]any{}
	for k, v := range row {
		out[k] = v
	}
	for _, change := range changes {
		out[change.Field] = displayValue(change.Field, out[change.Field])
	}
	return out
}

func redactStaffRow(row map[string]any, changes []applyChange) map[string]any {
	if row == nil {
		return nil
	}
	out := map[string]any{}
	for k, v := range row {
		out[k] = v
	}
	for _, change := range changes {
		if field, ok := staffRegisterDirectFields[change.Field]; ok {
			out[field] = displayValue(change.Field, out[field])
		}
	}
	return out
}

func intValue(value any) (int, *errs.Error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		var i int
		if _, err := fmt.Sscan(fmt.Sprint(v), &i); err != nil {
			return 0, errs.Validation("invalid_int", "value is not an integer")
		}
		return i, nil
	}
}

func hashValue(value any) string {
	sum := sha256.Sum256([]byte(fmt.Sprint(value)))
	return hex.EncodeToString(sum[:])
}
