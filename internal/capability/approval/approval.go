package approval

import (
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
)

func Tasks(assignee string, limit int) ([]map[string]any, *errs.Error) {
	args := []any{}
	query := `
		SELECT t.ID AS task_id, t.INSTANCEID AS instance_id, t.NODEID AS node_id,
		       t.APPROVER AS approver, t.AGENT AS agent, t.RECEIVEDTIME AS received_time,
		       t.HASREAD AS has_read, t.APRREMARK AS remark
		FROM skywftask t`
	if assignee != "" && assignee != "me" {
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

func WriteNotVerified(action string) *errs.Error {
	err := errs.Policy("approval_write_not_verified", action+" is not implemented because the approval state machine has not been verified")
	err.Hint = "V1 only supports approval task query/detail"
	return err
}
