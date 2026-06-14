package employee

import (
	"fmt"
	"strings"

	"hr-cli/internal/auth"
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/perm"
	"hr-cli/internal/redact"
)

const columns = `
	EID, badge, NAME, DPID, DPTITLE, JBID, JBTITLE, REPORTTO, REPORTTONAME,
	HRBP, STATUS, DISABLED, MOBILE, EMAIL, JOINDATE, LEAVEDATE`

const scopeSource = "psoradiationrangeeidlist"

type ScopeFilter struct {
	Source      string `json:"source"`
	OperatorEID string `json:"operator_eid"`
	Applied     bool   `json:"applied"`
	BypassRole  string `json:"bypass_role,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func Find(name, badge, phone string, limit int) ([]map[string]any, bool, *ScopeFilter, *errs.Error) {
	if err := perm.Require("employee.find", ""); err != nil {
		return nil, false, nil, err
	}
	var where []string
	var args []any
	if name != "" {
		where = append(where, "NAME LIKE ?")
		args = append(args, "%"+name+"%")
	}
	if badge != "" {
		where = append(where, "badge = ?")
		args = append(args, badge)
	}
	if phone != "" {
		where = append(where, "MOBILE = ?")
		args = append(args, phone)
	}
	if len(where) == 0 {
		return nil, false, nil, errs.Validation("missing_criteria", "provide one of --name, --badge, or --phone")
	}
	if limit <= 0 {
		limit = 20
	}
	scope, scopeClause, scopeArgs := buildScopeClause()
	if scopeClause != "" {
		where = append(where, scopeClause)
		args = append(args, scopeArgs...)
	}
	args = append(args, limit+1)
	query := fmt.Sprintf("SELECT %s FROM eemployee WHERE %s ORDER BY STATUS ASC, EID ASC LIMIT ?", columns, strings.Join(where, " AND "))
	rows, err := db.QueryRows(query, args...)
	if err != nil {
		return nil, false, scope, err
	}
	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}
	for i, row := range rows {
		rows[i] = normalize(row)
	}
	return rows, truncated, scope, nil
}

func Get(eid int) (map[string]any, *errs.Error) {
	if err := perm.Require("employee.get", fmt.Sprint(eid)); err != nil {
		return nil, err
	}
	row, err := db.QueryOne(fmt.Sprintf("SELECT %s, CERTNO FROM eemployee WHERE EID=?", columns), eid)
	if err != nil {
		return nil, err
	}
	if row == nil {
		e := errs.Validation("not_found", "employee not found")
		e.Param = "--eid"
		return nil, e
	}
	return normalize(row), nil
}

func ByBadge(badge string) (map[string]any, *errs.Error) {
	rows, _, _, err := Find("", badge, "", 2)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		e := errs.Validation("not_found", "employee not found")
		e.Param = "--badge"
		return nil, e
	}
	if len(rows) > 1 {
		return nil, errs.Validation("multiple_matches", "multiple employees matched; narrow the criteria")
	}
	return rows[0], nil
}

// ByBadgeUnscoped resolves a badge directly against eemployee without applying
// the operator's scope filter. Callers that need to surface an explicit
// "out of scope" error (transfer, profile-info) use this so the scope gate can
// produce a clear policy error instead of a misleading "not_found".
func ByBadgeUnscoped(badge string) (map[string]any, *errs.Error) {
	if err := perm.Require("employee.find", ""); err != nil {
		return nil, err
	}
	row, err := db.QueryOne(fmt.Sprintf("SELECT %s FROM eemployee WHERE badge=?", columns), badge)
	if err != nil {
		return nil, err
	}
	if row == nil {
		e := errs.Validation("not_found", "employee not found")
		e.Param = "--badge"
		return nil, e
	}
	return normalize(row), nil
}

// buildScopeClause returns a SQL fragment that restricts results to the
// operator's data radiation range. HR_ADMIN bypasses the filter, mirroring
// the super-admin whitelist semantics inside PsoRadiationRange_new.
func buildScopeClause() (*ScopeFilter, string, []any) {
	op := auth.CurrentOperator()
	role := strings.ToUpper(op.Role)
	scope := &ScopeFilter{Source: scopeSource, OperatorEID: op.EID}
	if role == "HR_ADMIN" || role == "SSC" {
		scope.Applied = false
		scope.BypassRole = role
		scope.Reason = "role " + role + " bypasses scope filter"
		return scope, "", nil
	}
	if strings.TrimSpace(op.EID) == "" {
		scope.Applied = true
		scope.Reason = "operator eid missing; restricting to empty scope"
		return scope, "EID IN (NULL)", nil
	}
	scope.Applied = true
	scope.Reason = "filtered by psoradiationrangeeidlist"
	return scope, "EID IN (SELECT eid FROM psoradiationrangeeidlist WHERE ueid=?)", []any{op.EID}
}

func normalize(row map[string]any) map[string]any {
	out := redact.Employee(row)
	out["actions"] = map[string]bool{
		"can_transfer":         true,
		"can_edit_profile":     true,
		"can_query_attendance": true,
	}
	return out
}
