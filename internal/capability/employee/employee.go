package employee

import (
	"fmt"
	"strings"

	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/redact"
)

const columns = `
	EID, badge, NAME, DPID, DPTITLE, JBID, JBTITLE, REPORTTO, REPORTTONAME,
	HRBP, STATUS, DISABLED, MOBILE, EMAIL, JOINDATE, LEAVEDATE`

func Find(name, badge, phone string, limit int) ([]map[string]any, bool, *errs.Error) {
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
		return nil, false, errs.Validation("missing_criteria", "provide one of --name, --badge, or --phone")
	}
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit+1)
	query := fmt.Sprintf("SELECT %s FROM eemployee WHERE %s ORDER BY STATUS ASC, EID ASC LIMIT ?", columns, strings.Join(where, " AND "))
	rows, err := db.QueryRows(query, args...)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}
	for i, row := range rows {
		rows[i] = normalize(row)
	}
	return rows, truncated, nil
}

func Get(eid int) (map[string]any, *errs.Error) {
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
	rows, _, err := Find("", badge, "", 2)
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

func normalize(row map[string]any) map[string]any {
	out := redact.Employee(row)
	out["actions"] = map[string]bool{
		"can_transfer":         true,
		"can_edit_profile":     true,
		"can_query_attendance": true,
	}
	return out
}
