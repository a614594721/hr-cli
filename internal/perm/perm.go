package perm

import (
	"strings"

	"hr-cli/internal/auth"
	"hr-cli/internal/errs"
)

type Decision struct {
	Action    string        `json:"action"`
	TargetEID string        `json:"target_eid,omitempty"`
	Operator  auth.Operator `json:"operator"`
	Decision  string        `json:"decision"`
	Reason    string        `json:"reason"`
	Scope     string        `json:"scope"`
}

var roleActions = map[string]map[string]bool{
	"SELF": {
		"auth.me":                  true,
		"employee.find":            true,
		"employee.get":             true,
		"profile_info.preview":     true,
		"profile_info.apply":       true,
		"approval.task.list":       true,
		"approval.task.get":        true,
		"approval.task.approve":    true,
		"approval.task.reject":     true,
		"approval.task.transfer":   true,
		"attendance.records.query": true,
	},
	"HRBP": {
		"auth.me":                    true,
		"employee.find":              true,
		"employee.get":               true,
		"transfer.preview":           true,
		"transfer.apply":             true,
		"profile_info.preview":       true,
		"profile_info.apply":         true,
		"approval.task.list":         true,
		"approval.task.get":          true,
		"approval.task.approve":      true,
		"approval.task.reject":       true,
		"approval.task.transfer":     true,
		"attendance.records.query":   true,
		"attendance.summary.query":   true,
		"attendance.exception.query": true,
	},
	"MANAGER": {
		"auth.me":                    true,
		"employee.find":              true,
		"employee.get":               true,
		"approval.task.list":         true,
		"approval.task.get":          true,
		"approval.task.approve":      true,
		"approval.task.reject":       true,
		"approval.task.transfer":     true,
		"attendance.summary.query":   true,
		"attendance.exception.query": true,
	},
	"HR_ADMIN": {"*": true},
}

func Explain(action, targetEID string) Decision {
	operator := auth.CurrentOperator()
	role := strings.ToUpper(operator.Role)
	allowed := roleActions[role][action] || roleActions[role]["*"]
	decision := "deny"
	reason := "action is not allowed for role " + role
	if allowed {
		decision = "allow"
		reason = "action is allowed for role " + role
	}
	return Decision{
		Action:    action,
		TargetEID: targetEID,
		Operator:  operator,
		Decision:  decision,
		Reason:    reason,
		Scope:     scopeForRole(role),
	}
}

func Require(action, targetEID string) *errs.Error {
	decision := Explain(action, targetEID)
	if decision.Decision == "allow" {
		return nil
	}
	err := errs.Authorization("action_denied", decision.Reason)
	err.Param = action
	err.Hint = "run perm explain --action " + action
	return err
}

func scopeForRole(role string) string {
	switch role {
	case "SELF":
		return "self"
	case "HRBP":
		return "hrbp_scope"
	case "MANAGER":
		return "direct_reports"
	case "HR_ADMIN":
		return "all"
	default:
		return "none"
	}
}
