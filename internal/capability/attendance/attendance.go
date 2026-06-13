package attendance

import (
	"strconv"
	"strings"

	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/perm"
)

const recordColumns = `
	TERM, BADGE, NAME, EID, card_times, cardbegintime, cardendtime,
	attend_total, Terms, REMARK, AMOUNT_1477, AMOUNT_1478, AMOUNT_1540,
	AMOUNT_1541, AMOUNT_1543, AMOUNT_1571`

func Records(badge string, eid int, from, to string, limit int) ([]map[string]any, *errs.Error) {
	if err := perm.Require("attendance.records.query", targetEID(eid)); err != nil {
		return nil, err
	}
	if badge == "" && eid == 0 {
		return nil, errs.Validation("missing_target", "provide --badge or --eid")
	}
	var where []string
	var args []any
	if badge != "" {
		where = append(where, "BADGE=?")
		args = append(args, badge)
	}
	if eid != 0 {
		where = append(where, "EID=?")
		args = append(args, eid)
	}
	if from != "" {
		where = append(where, "TERM >= ?")
		args = append(args, from)
	}
	if to != "" {
		where = append(where, "TERM <= ?")
		args = append(args, to)
	}
	args = append(args, positiveLimit(limit, 100))
	return db.QueryRows("SELECT "+recordColumns+" FROM attend_information WHERE "+strings.Join(where, " AND ")+" ORDER BY TERM DESC LIMIT ?", args...)
}

func Summary(badge string, dept int, date string, limit int) ([]map[string]any, *errs.Error) {
	if err := perm.Require("attendance.summary.query", ""); err != nil {
		return nil, err
	}
	var where []string
	var args []any
	if badge != "" {
		where = append(where, "a.BADGE=?")
		args = append(args, badge)
	}
	if dept != 0 {
		where = append(where, "e.DPID=?")
		args = append(args, dept)
	}
	if date != "" {
		where = append(where, "a.TERM=?")
		args = append(args, date)
	}
	if len(where) == 0 {
		return nil, errs.Validation("missing_filter", "provide --badge, --dept, or --date")
	}
	args = append(args, positiveLimit(limit, 100))
	query := `
		SELECT a.BADGE, a.NAME, a.EID, COUNT(*) AS days,
		       SUM(COALESCE(a.card_times,0)) AS card_times,
		       SUM(COALESCE(a.attend_total,0)) AS attend_total,
		       SUM(CASE WHEN a.REMARK IS NULL OR a.REMARK='' THEN 0 ELSE 1 END) AS exception_days
		FROM attend_information a
		LEFT JOIN eemployee e ON e.EID=a.EID
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY a.BADGE, a.NAME, a.EID
		ORDER BY exception_days DESC, a.BADGE ASC
		LIMIT ?`
	return db.QueryRows(query, args...)
}

func Exceptions(badge string, dept int, from, to string, limit int) ([]map[string]any, *errs.Error) {
	if err := perm.Require("attendance.exception.query", ""); err != nil {
		return nil, err
	}
	where := []string{"a.REMARK IS NOT NULL", "a.REMARK <> ''"}
	var args []any
	join := ""
	if badge != "" {
		where = append(where, "a.BADGE=?")
		args = append(args, badge)
	}
	if dept != 0 {
		join = "LEFT JOIN eemployee e ON e.EID=a.EID"
		where = append(where, "e.DPID=?")
		args = append(args, dept)
	}
	if from != "" {
		where = append(where, "a.TERM >= ?")
		args = append(args, from)
	}
	if to != "" {
		where = append(where, "a.TERM <= ?")
		args = append(args, to)
	}
	args = append(args, positiveLimit(limit, 100))
	query := `
		SELECT a.TERM, a.BADGE, a.NAME, a.EID, a.card_times,
		       a.cardbegintime, a.cardendtime, a.REMARK
		FROM attend_information a
		` + join + `
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY a.TERM DESC
		LIMIT ?`
	return db.QueryRows(query, args...)
}

func targetEID(eid int) string {
	if eid == 0 {
		return ""
	}
	return strconv.Itoa(eid)
}

func positiveLimit(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
