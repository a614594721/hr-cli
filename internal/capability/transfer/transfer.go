package transfer

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"hr-cli/internal/auth"
	"hr-cli/internal/capability/employee"
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/preview"
)

const defaultTransferXType = 12

func Preview(badge string, dept, job int, effectDate, reason string) (preview.Payload, *errs.Error) {
	target, err := employee.ByBadge(badge)
	if err != nil {
		return preview.Payload{}, err
	}
	var changes []map[string]any
	if dept != 0 {
		changes = append(changes, map[string]any{"field": "DPID", "old": target["DPID"], "new": dept})
	}
	if job != 0 {
		changes = append(changes, map[string]any{"field": "JBID", "old": target["JBID"], "new": job})
	}
	if effectDate != "" {
		changes = append(changes, map[string]any{"field": "EFFECTDATE", "old": nil, "new": effectDate})
	}
	if reason != "" {
		changes = append(changes, map[string]any{"field": "reason", "old": nil, "new": reason})
	}
	if len(changes) == 0 {
		return preview.Payload{}, errs.Validation("missing_changes", "provide at least one of --dept, --job, --effect-date, or --reason")
	}
	plan := map[string]any{
		"capability":    "transfer.plan",
		"operator":      auth.CurrentOperator(),
		"target":        target,
		"changes":       changes,
		"transfer_type": defaultTransferXType,
		"native_chain": []string{
			"CALL eSP_EmpChangeAdd(P_EID, P_xType, P_FromDate, P_RefID, P_URID, OUT P_RetVal)",
			"UPDATE eEmployee_Work SET approved changed fields WHERE ID = generated_work_id",
			"CALL eSP_EmpChangeCheck(P_ID, P_URID, P_TableName, P_FMID, OUT P_RetVal)",
			"CALL eSP_EmpChangeStart(P_ID, P_URID, OUT P_RetVal)",
			"Verify eemployee/eEmployee_Work_All and audit result",
		},
		"write_path": []string{"eSP_EmpChangeAdd", "eEmployee_Work field update", "eSP_EmpChangeCheck", "eSP_EmpChangeStart", "eemployee/eEmployee_Work_All verification"},
		"status":     "preview_only",
	}
	return preview.Save("transfer", plan)
}

func Apply(previewID string, yes, dryRun bool) (map[string]any, *errs.Error) {
	if !yes && !dryRun {
		err := errs.Confirmation("missing_confirmation", "transfer apply requires --yes or --dry-run")
		err.Hint = "use --dry-run to inspect the apply preflight without writing"
		return nil, err
	}
	payload, loadErr := preview.Load(previewID)
	if loadErr != nil {
		return nil, loadErr
	}
	result, preflightErr := preflight(payload)
	if preflightErr != nil {
		return nil, preflightErr
	}
	if dryRun {
		result["status"] = "dry_run_preflight_only"
		return result, nil
	}
	if ok, _ := result["preflight_ok"].(bool); !ok {
		err := errs.Policy("transfer_preflight_failed", "transfer apply preflight failed")
		err.Hint = "run transfer +apply <preview-id> --dry-run and fix failed checks"
		return nil, err
	}
	err := errs.Policy("transfer_apply_disabled", "transfer apply preflight passed, but native write execution is not enabled yet")
	err.Hint = "enable only after audited calls to eSP_EmpChangeAdd, eSP_EmpChangeCheck, and eSP_EmpChangeStart are implemented"
	return nil, err
}

func preflight(payload preview.Payload) (map[string]any, *errs.Error) {
	if payload.Kind != "transfer" {
		err := errs.Validation("invalid_preview_kind", "preview is not a transfer preview")
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
	badge, _ := target["badge"].(string)
	if badge == "" {
		return nil, errs.Validation("invalid_preview", "preview target badge is missing")
	}
	current, currentErr := employee.ByBadge(badge)
	if currentErr != nil {
		return nil, currentErr
	}
	checks := []map[string]any{}
	operator := auth.CurrentOperator()
	checks = append(checks, map[string]any{"name": "operator_resolved", "ok": operator.Name != "", "operator": operator})
	checks = append(checks, map[string]any{"name": "operator_urid_present", "ok": operator.URID != "", "hint": "HR_OPERATOR_URID is required before native stored procedure execution"})

	changes, ok := plan["changes"].([]any)
	if !ok {
		return nil, errs.Validation("invalid_preview", "preview changes are invalid")
	}
	oldValueOK := true
	for _, item := range changes {
		change, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field, _ := change["field"].(string)
		if field != "DPID" && field != "JBID" {
			continue
		}
		matches := equalValue(current[field], change["old"])
		if !matches {
			oldValueOK = false
		}
		checks = append(checks, map[string]any{
			"name":    "old_value_matches",
			"field":   field,
			"ok":      matches,
			"current": current[field],
			"preview": change["old"],
		})
	}

	eid := current["EID"]
	openRows, countErr := db.QueryOne("SELECT COUNT(*) AS cnt FROM eemployee_work WHERE EID=? AND XTYPE=?", eid, defaultTransferXType)
	if countErr != nil {
		return nil, countErr
	}
	openCount := openRows["cnt"]
	noOpenWork := numericString(openCount) == "0"
	checks = append(checks, map[string]any{"name": "no_open_same_type_work_row", "ok": noOpenWork, "open_work_rows": openCount, "xtype": defaultTransferXType})

	preflightOK := oldValueOK && noOpenWork && operator.URID != ""
	return map[string]any{
		"preview_id":       payload.PreviewID,
		"preflight_ok":     preflightOK,
		"target":           current,
		"checks":           checks,
		"native_chain":     plan["native_chain"],
		"transfer_type":    defaultTransferXType,
		"execution_status": "native_execution_disabled",
	}, nil
}

func equalValue(a, b any) bool {
	return numericString(a) == numericString(b)
}

func numericString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if math.Trunc(v) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}
