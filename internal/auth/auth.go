package auth

import (
	"fmt"
	"os"
	"strings"

	"hr-cli/internal/db"
	"hr-cli/internal/errs"
	"hr-cli/internal/runtime"
)

type Operator struct {
	EID    string `json:"eid,omitempty"`
	URID   string `json:"urid,omitempty"`
	Badge  string `json:"badge,omitempty"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Source string `json:"source"`
}

type LoginRequest struct {
	EID        int
	Badge      string
	Email      string
	Phone      string
	Name       string
	DingUserID string
	Role       string
}

func CurrentOperator() Operator {
	profile, hasProfile := runtime.ActiveProfile()
	session, hasSession := runtime.LoadSession()
	role := firstNonEmpty(os.Getenv("HR_OPERATOR_ROLE"), session.Role, profile.OperatorRole)
	if role == "" {
		dbEnv := firstNonEmpty(os.Getenv("DB_ENV"), profile.DBEnv)
		if dbEnv == "test" || (!hasProfile && dbEnv == "") {
			role = "HR_ADMIN"
		} else {
			role = "SELF"
		}
	}
	name := firstNonEmpty(os.Getenv("HR_OPERATOR_NAME"), session.Name, profile.OperatorName)
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if name == "" {
		name = "local-operator"
	}
	source := "environment"
	if noOperatorEnv() && hasSession {
		source = session.Source
	} else if os.Getenv("HR_OPERATOR_NAME") == "" && hasProfile && profile.OperatorName != "" {
		source = "profile"
	}
	return Operator{
		EID:    firstNonEmpty(os.Getenv("HR_OPERATOR_EID"), session.EID, profile.OperatorEID),
		URID:   firstNonEmpty(os.Getenv("HR_OPERATOR_URID"), session.URID, profile.OperatorURID),
		Badge:  firstNonEmpty(os.Getenv("HR_OPERATOR_BADGE"), session.Badge, profile.OperatorBadge),
		Name:   name,
		Role:   role,
		Source: source,
	}
}

func Login(req LoginRequest) (map[string]any, *errs.Error) {
	row, err := resolveEmployee(req)
	if err != nil {
		return nil, err
	}
	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if role == "" {
		role = defaultRole()
	}
	if !validRole(role) {
		e := errs.Validation("invalid_role", "--role must be SELF, HRBP, MANAGER, or HR_ADMIN")
		e.Param = "--role"
		return nil, e
	}
	eid := fmt.Sprint(row["EID"])
	session := runtime.Session{
		EID:    eid,
		URID:   eid,
		Badge:  fmt.Sprint(row["badge"]),
		Name:   fmt.Sprint(row["NAME"]),
		Role:   role,
		Source: "db_session",
	}
	if err := runtime.SaveSession(session); err != nil {
		return nil, err
	}
	return map[string]any{
		"status":   "active",
		"mode":     "db_session",
		"operator": sessionToOperator(session),
		"identity": map[string]any{
			"email":       row["EMAIL"],
			"ding_userid": row["ding_userid"],
		},
	}, nil
}

func Logout() (map[string]any, *errs.Error) {
	removed, err := runtime.ClearSession()
	if err != nil {
		return nil, err
	}
	status := "no_session"
	if removed {
		status = "cleared"
	}
	return map[string]any{"status": status, "mode": "db_session"}, nil
}

func resolveEmployee(req LoginRequest) (map[string]any, *errs.Error) {
	where := []string{}
	args := []any{}
	if req.EID != 0 {
		where = append(where, "e.EID=?")
		args = append(args, req.EID)
	}
	if req.Badge != "" {
		where = append(where, "e.badge=?")
		args = append(args, req.Badge)
	}
	if req.Email != "" {
		where = append(where, "e.EMAIL=?")
		args = append(args, req.Email)
	}
	if req.Phone != "" {
		where = append(where, "e.MOBILE=?")
		args = append(args, req.Phone)
	}
	if req.Name != "" {
		where = append(where, "e.NAME=?")
		args = append(args, req.Name)
	}
	if req.DingUserID != "" {
		where = append(where, "d.userid=?")
		args = append(args, req.DingUserID)
	}
	if len(where) == 0 {
		return nil, errs.Validation("missing_login_identifier", "provide one of --eid, --badge, --email, --phone, --name, or --ding-userid")
	}
	query := `
		SELECT e.EID, e.badge, e.NAME, e.EMAIL, e.MOBILE, MAX(d.userid) AS ding_userid
		FROM eemployee e
		LEFT JOIN employee_dingding d ON (
			d.email=e.EMAIL OR d.mobile=e.MOBILE OR d.job_number=e.badge OR d.job_number=SUBSTRING(e.badge, 2)
		)
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY e.EID, e.badge, e.NAME, e.EMAIL, e.MOBILE
		ORDER BY e.STATUS ASC, e.EID ASC
		LIMIT 3`
	rows, err := db.QueryRows(query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		e := errs.Validation("login_identity_not_found", "no employee matched the login identifier")
		e.Hint = "check eemployee and employee_dingding mappings"
		return nil, e
	}
	if len(rows) > 1 {
		e := errs.Validation("multiple_login_matches", "multiple employees matched; use --eid or --badge")
		e.Hint = "login does not guess identity"
		return nil, e
	}
	return rows[0], nil
}

func sessionToOperator(session runtime.Session) Operator {
	return Operator{EID: session.EID, URID: session.URID, Badge: session.Badge, Name: session.Name, Role: session.Role, Source: session.Source}
}

func defaultRole() string {
	profile, hasProfile := runtime.ActiveProfile()
	dbEnv := firstNonEmpty(os.Getenv("DB_ENV"), profile.DBEnv)
	if dbEnv == "test" || (!hasProfile && dbEnv == "") {
		return "HR_ADMIN"
	}
	return "SELF"
}

func validRole(role string) bool {
	switch role {
	case "SELF", "HRBP", "MANAGER", "HR_ADMIN":
		return true
	default:
		return false
	}
}

func noOperatorEnv() bool {
	for _, key := range []string{"HR_OPERATOR_EID", "HR_OPERATOR_URID", "HR_OPERATOR_BADGE", "HR_OPERATOR_NAME", "HR_OPERATOR_ROLE"} {
		if os.Getenv(key) != "" {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
