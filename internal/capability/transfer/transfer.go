package transfer

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"hr-cli/internal/audit"
	"hr-cli/internal/auth"
	"hr-cli/internal/capability/employee"
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/perm"
	"hr-cli/internal/preview"
)

const (
	defaultTransferXType = 12
	transferFormID       = 110001
	transferTableName    = "EEMPLOYEE_WORK"
	defaultTransferRefID = 0
)

func Preview(badge string, dept, job int, effectDate, reason string) (preview.Payload, *errs.Error) {
	if err := perm.Require("transfer.preview", ""); err != nil {
		return preview.Payload{}, err
	}
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
	if err := perm.Require("transfer.apply", ""); err != nil {
		return nil, err
	}
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
	return executeNativeApply(payload, result)
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
	uridCheck := map[string]any{"name": "operator_urid_present", "ok": operator.URID != ""}
	if operator.URID == "" {
		uridCheck["hint"] = "HR_OPERATOR_URID is required before native stored procedure execution"
	}
	checks = append(checks, uridCheck)

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

	hasEffectiveChange := false
	effectDatePresent := false
	for _, item := range changes {
		change, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field, _ := change["field"].(string)
		if field == "EFFECTDATE" && fmt.Sprint(change["new"]) != "" {
			effectDatePresent = true
		}
		if (field == "DPID" || field == "JBID") && !equalValue(change["old"], change["new"]) {
			hasEffectiveChange = true
		}
	}
	checks = append(checks, map[string]any{"name": "has_effective_org_or_job_change", "ok": hasEffectiveChange})
	checks = append(checks, map[string]any{"name": "effect_date_present", "ok": effectDatePresent})

	preflightOK := oldValueOK && noOpenWork && operator.URID != "" && hasEffectiveChange && effectDatePresent
	return map[string]any{
		"preview_id":       payload.PreviewID,
		"preflight_ok":     preflightOK,
		"target":           current,
		"checks":           checks,
		"native_chain":     plan["native_chain"],
		"transfer_type":    defaultTransferXType,
		"table_name":       transferTableName,
		"form_id":          transferFormID,
		"ref_id":           defaultTransferRefID,
		"execution_status": "native_execution_available",
	}, nil
}

func executeNativeApply(payload preview.Payload, preflight map[string]any) (map[string]any, *errs.Error) {
	cfg, cfgErr := db.EnvConfig()
	if cfgErr != nil {
		return nil, cfgErr
	}
	if cfg.Env != "test" {
		err := errs.Policy("production_write_denied", "transfer apply is only enabled when DB_ENV=test")
		err.Hint = "production write protection is active"
		return nil, err
	}

	plan := payload.Plan.(map[string]any)
	current := preflight["target"].(map[string]any)
	operator := auth.CurrentOperator()
	urid, convErr := strconv.Atoi(operator.URID)
	if convErr != nil || urid <= 0 {
		err := errs.Validation("invalid_operator_urid", "HR_OPERATOR_URID must be a positive integer")
		err.Param = "HR_OPERATOR_URID"
		return nil, err
	}
	eid, eidErr := intValue(current["EID"])
	if eidErr != nil {
		return nil, eidErr
	}
	changes := plan["changes"].([]any)
	effectDate, changedFields, changeErr := collectApplyChanges(changes)
	if changeErr != nil {
		return nil, changeErr
	}

	if auditErr := audit.Write("transfer.apply.start", map[string]any{
		"preview_id": payload.PreviewID,
		"operator":   operator,
		"target":     current,
		"changes":    changedFields,
	}); auditErr != nil {
		return nil, auditErr
	}

	result := map[string]any{
		"preview_id": payload.PreviewID,
		"target":     current,
		"steps":      []map[string]any{},
	}
	var workID int
	nativeErr := db.WithConn(func(ctx context.Context, conn *sql.Conn) *errs.Error {
		var err *errs.Error
		workID, err = callEmpChangeAdd(ctx, conn, eid, effectDate, urid)
		if err != nil {
			return err
		}
		appendStep(result, "eSP_EmpChangeAdd", true, map[string]any{"work_id": workID})

		if err := updateWorkFields(ctx, conn, workID, changedFields); err != nil {
			bestEffortCancel(ctx, conn, workID, urid, result)
			return err
		}
		appendStep(result, "update_eemployee_work_fields", true, map[string]any{"work_id": workID, "fields": changedFields})

		if err := callRetvalProc(ctx, conn, "CALL eSP_EmpChangeCheck(?, ?, ?, ?, @hr_cli_ret)", workID, urid, transferTableName, transferFormID); err != nil {
			bestEffortCancel(ctx, conn, workID, urid, result)
			return err
		}
		appendStep(result, "eSP_EmpChangeCheck", true, map[string]any{"work_id": workID, "table_name": transferTableName, "form_id": transferFormID})

		if err := callRetvalProc(ctx, conn, "CALL eSP_EmpChangeStart(?, ?, @hr_cli_ret)", workID, urid); err != nil {
			bestEffortCancel(ctx, conn, workID, urid, result)
			return err
		}
		appendStep(result, "eSP_EmpChangeStart", true, map[string]any{"work_id": workID})

		verify, err := verifyApply(ctx, conn, workID, eid, changedFields)
		if err != nil {
			return err
		}
		result["verification"] = verify
		return nil
	})
	if nativeErr != nil {
		_ = audit.Write("transfer.apply.failed", map[string]any{"preview_id": payload.PreviewID, "error": nativeErr})
		return nil, nativeErr
	}
	result["status"] = "applied"
	result["work_id"] = workID
	if auditErr := audit.Write("transfer.apply.success", result); auditErr != nil {
		return nil, auditErr
	}
	return result, nil
}

func bestEffortCancel(ctx context.Context, conn *sql.Conn, workID, urid int, result map[string]any) {
	if workID == 0 {
		return
	}
	err := callRetvalProc(ctx, conn, "CALL eSP_EmpChangeCancel(?, ?, @hr_cli_ret)", workID, urid)
	appendStep(result, "eSP_EmpChangeCancel_best_effort", err == nil, map[string]any{"work_id": workID})
}

func callEmpChangeAdd(ctx context.Context, conn *sql.Conn, eid int, effectDate string, urid int) (int, *errs.Error) {
	if err := callRetvalProc(ctx, conn, "CALL eSP_EmpChangeAdd(?, ?, ?, ?, ?, @hr_cli_ret)", eid, defaultTransferXType, effectDate, defaultTransferRefID, urid); err != nil {
		return 0, err
	}
	row, err := db.QueryOneOnConn(ctx, conn, `
		SELECT ID
		FROM eemployee_work
		WHERE EID=? AND XTYPE=? AND IFNULL(REFID,0)=? AND REGBY=? AND IFNULL(SUBMIT,0)=0 AND IFNULL(CLOSED,0)=0
		ORDER BY ID DESC
		LIMIT 1`, eid, defaultTransferXType, defaultTransferRefID, urid)
	if err != nil {
		return 0, err
	}
	if row == nil {
		return 0, errs.DB("work_row_not_found", "eSP_EmpChangeAdd succeeded but no matching eemployee_work row was found")
	}
	return intValue(row["ID"])
}

func callRetvalProc(ctx context.Context, conn *sql.Conn, call string, args ...any) *errs.Error {
	if err := db.ExecOnConn(ctx, conn, "SET @hr_cli_ret = NULL"); err != nil {
		return err
	}
	if err := db.ExecOnConn(ctx, conn, call, args...); err != nil {
		return err
	}
	row, err := db.QueryOneOnConn(ctx, conn, "SELECT @hr_cli_ret AS ret")
	if err != nil {
		return err
	}
	ret := numericString(row["ret"])
	if ret != "0" {
		e := errs.DB("stored_procedure_failed", "stored procedure returned non-zero code: "+ret)
		e.Hint = "inspect .hr-cli/audit and database procedure return-code documentation"
		return e
	}
	return nil
}

func updateWorkFields(ctx context.Context, conn *sql.Conn, workID int, changes map[string]any) *errs.Error {
	allowed := []string{"DPID", "JBID", "EFFECTDATE", "reason", "CHGCOMMENT"}
	set := []string{}
	args := []any{}
	for _, field := range allowed {
		value, ok := changes[field]
		if !ok {
			continue
		}
		set = append(set, field+"=?")
		args = append(args, value)
	}
	if len(set) == 0 {
		return errs.Validation("missing_changes", "no allowed transfer fields to update")
	}
	args = append(args, workID)
	return db.ExecOnConn(ctx, conn, "UPDATE eemployee_work SET "+strings.Join(set, ", ")+" WHERE ID=?", args...)
}

func verifyApply(ctx context.Context, conn *sql.Conn, workID, eid int, changes map[string]any) (map[string]any, *errs.Error) {
	workRow, err := db.QueryOneOnConn(ctx, conn, "SELECT COUNT(*) AS cnt FROM eemployee_work WHERE ID=?", workID)
	if err != nil {
		return nil, err
	}
	historyRow, err := db.QueryOneOnConn(ctx, conn, "SELECT COUNT(*) AS cnt FROM eemployee_work_all WHERE ID=?", workID)
	if err != nil {
		return nil, err
	}
	current, err := db.QueryOneOnConn(ctx, conn, "SELECT EID, DPID, JBID FROM eemployee WHERE EID=?", eid)
	if err != nil {
		return nil, err
	}
	verify := map[string]any{
		"work_row_removed": numericString(workRow["cnt"]) == "0",
		"history_written":  numericString(historyRow["cnt"]) == "1",
		"current_employee": current,
	}
	if effect, ok := changes["EFFECTDATE"]; ok && effectDueNow(fmt.Sprint(effect)) {
		verify["current_values_expected_now"] = true
		verify["dpid_matches"] = equalValue(current["DPID"], changes["DPID"])
		verify["jbid_matches"] = equalValue(current["JBID"], changes["JBID"])
	}
	return verify, nil
}

func collectApplyChanges(changes []any) (string, map[string]any, *errs.Error) {
	out := map[string]any{}
	effectDate := ""
	for _, item := range changes {
		change, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field, _ := change["field"].(string)
		newValue := change["new"]
		switch field {
		case "DPID", "JBID":
			out[field] = newValue
		case "EFFECTDATE":
			effectDate = fmt.Sprint(newValue)
			out[field] = effectDate
		case "reason":
			out["reason"] = newValue
			out["CHGCOMMENT"] = newValue
		}
	}
	if effectDate == "" {
		e := errs.Validation("missing_effect_date", "transfer apply requires EFFECTDATE in preview")
		e.Param = "--effect-date"
		return "", nil, e
	}
	return effectDate, out, nil
}

func appendStep(result map[string]any, name string, ok bool, detail map[string]any) {
	steps, _ := result["steps"].([]map[string]any)
	step := map[string]any{"name": name, "ok": ok}
	for k, v := range detail {
		step[k] = v
	}
	result["steps"] = append(steps, step)
}

func intValue(value any) (int, *errs.Error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case []byte:
		i, err := strconv.Atoi(string(v))
		if err != nil {
			return 0, errs.Validation("invalid_int", "value is not an integer")
		}
		return i, nil
	default:
		i, err := strconv.Atoi(fmt.Sprint(v))
		if err != nil {
			return 0, errs.Validation("invalid_int", "value is not an integer")
		}
		return i, nil
	}
}

func effectDueNow(value string) bool {
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05"} {
		t, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return !t.After(time.Now())
		}
	}
	return false
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
