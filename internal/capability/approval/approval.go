package approval

import (
	"fmt"
	"strconv"
	"strings"

	"hr-cli/internal/auth"
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/perm"
)

func Tasks(assignee string, limit int) ([]map[string]any, *errs.Error) {
	if err := perm.Require("approval.task.list", ""); err != nil {
		return nil, err
	}
	args := []any{}
	query := `
		SELECT t.ID AS task_id, t.INSTANCEID AS instance_id, t.NODEID AS node_id,
		       t.APPROVER AS approver, t.AGENT AS agent, t.RECEIVEDTIME AS received_time,
		       t.HASREAD AS has_read, t.APRREMARK AS remark
		FROM skywftask t`
	if assignee == "me" {
		operator := auth.CurrentOperator()
		if operator.URID == "" {
			err := errs.Authentication("missing_operator_urid", "--assignee me requires HR_OPERATOR_URID or profile operator_urid")
			err.Param = "--assignee"
			return nil, err
		}
		query += " WHERE (CAST(t.APPROVER AS CHAR)=? OR CAST(t.AGENT AS CHAR)=?)"
		args = append(args, operator.URID, operator.URID)
	} else if assignee != "" {
		query += " WHERE (CAST(t.APPROVER AS CHAR)=? OR CAST(t.AGENT AS CHAR)=?)"
		args = append(args, assignee, assignee)
	}
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)
	query += " ORDER BY t.RECEIVEDTIME DESC, t.ID DESC LIMIT ?"
	return db.QueryRows(query, args...)
}

func Task(taskID int) (map[string]any, *errs.Error) {
	if err := perm.Require("approval.task.get", ""); err != nil {
		return nil, err
	}
	row, err := db.QueryOne(`
		SELECT t.ID AS task_id, t.INSTANCEID AS instance_id, t.NODEID AS node_id,
		       t.SLOTID AS slot_id, t.APPROVER AS approver, t.AGENT AS agent,
		       t.RECEIVEDTIME AS received_time, t.HASREAD AS has_read,
		       t.APRREMARK AS remark, t.APROPTION AS option_text,
		       t.FULLPATH AS full_path, t.FULLPATHAGENT AS full_path_agent
		FROM skywftask t
		WHERE t.ID=?`, taskID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		e := errs.Validation("not_found", "approval task not found")
		e.Param = "--task-id"
		return nil, e
	}
	return row, nil
}

func Instances(employee, status string, limit int) ([]map[string]any, *errs.Error) {
	if err := perm.Require("approval.task.list", employee); err != nil {
		return nil, err
	}
	where := []string{}
	args := []any{}
	if employee != "" {
		where = append(where, "(CAST(i.INITIATOR AS CHAR)=? OR CAST(i.PRINCIPAL AS CHAR)=?)")
		args = append(args, employee, employee)
	}
	if status != "" {
		value, err := formState(status)
		if err != nil {
			return nil, err
		}
		where = append(where, "i.FORMSTATE=?")
		args = append(args, value)
	}
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT i.INSTANCEID AS instance_id, i.FLOWID AS flow_id, i.NODEID AS node_id,
		       i.INITIATOR AS initiator, i.PRINCIPAL AS principal, i.FORMSTATE AS form_state,
		       i.SUBMITTEDTIME AS submitted_time, i.CLOSEDTIME AS closed_time,
		       i.LASTAPPROVETIME AS last_approve_time, i.XSUBJECT AS subject,
		       i.APPREMARK AS remark
		FROM skywfinstance i`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, limit)
	query += " ORDER BY i.SUBMITTEDTIME DESC, i.INSTANCEID DESC LIMIT ?"
	return db.QueryRows(query, args...)
}

func WritePlan(action string, taskID int, comment, reason, toBadge string, dryRun, yes bool) (map[string]any, *errs.Error) {
	if err := perm.Require(actionPermission(action), ""); err != nil {
		return nil, err
	}
	if !dryRun && !yes {
		err := errs.Confirmation("missing_confirmation", action+" requires --dry-run or --yes")
		err.Hint = "use --dry-run to inspect the approval task and native-chain status"
		return nil, err
	}
	task, err := Task(taskID)
	if err != nil {
		return nil, err
	}
	operator := auth.CurrentOperator()
	checks := []map[string]any{
		{"name": "operator_resolved", "ok": operator.Name != "", "operator": operator},
		{"name": "operator_urid_present", "ok": operator.URID != ""},
		{"name": "task_current_handler_matches_operator", "ok": taskMatchesOperator(task, operator)},
		{"name": "native_approval_entry_verified", "ok": false, "hint": "no generic approve/reject/transfer stored procedure has been verified"},
	}
	plan := map[string]any{
		"action":            action,
		"task":              task,
		"operator":          operator,
		"comment":           comment,
		"reason":            reason,
		"to_badge":          toBadge,
		"checks":            checks,
		"execution_status":  "native_write_not_verified",
		"native_candidates": []string{"esp_ddflow_delete", "esp_ddflow_approver_agent", "esp_ddflow_resubmit", "domain-specific skyWF_* procedures"},
	}
	if dryRun {
		plan["status"] = "dry_run_only"
		return plan, nil
	}
	err = errs.Policy("approval_write_not_verified", action+" is not implemented in hr-cli 1.0; the approval state machine has not been verified")
	err.Hint = "1.0 ships read-only approval queries plus dry-run only; --yes will be enabled once the native approve/reject/transfer chain is confirmed in a future release"
	return nil, err
}

func actionPermission(action string) string {
	switch action {
	case "+approve":
		return "approval.task.approve"
	case "+reject":
		return "approval.task.reject"
	case "+transfer":
		return "approval.task.transfer"
	default:
		return action
	}
}

func formState(status string) (int, *errs.Error) {
	switch strings.ToLower(status) {
	case "pending":
		return 2, nil
	case "closed", "completed", "approved":
		return 3, nil
	case "rejected":
		return 4, nil
	default:
		value, err := strconv.Atoi(status)
		if err != nil {
			e := errs.Validation("invalid_status", "--status must be pending, closed, approved, rejected, or a numeric FORMSTATE")
			e.Param = "--status"
			return 0, e
		}
		return value, nil
	}
}

func taskMatchesOperator(task map[string]any, operator auth.Operator) bool {
	if operator.URID == "" {
		return false
	}
	return fmt.Sprint(task["approver"]) == operator.URID || fmt.Sprint(task["agent"]) == operator.URID
}
