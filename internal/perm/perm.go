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
	Target    *ScopeResult  `json:"target_scope,omitempty"`
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
	"SSC":      {"*": true},
}

// Explain returns the action-level decision plus, when a target is supplied,
// the target-scope decision derived from psoradiationrangeeidlist.
// On a DB error during the scope lookup the action result is still returned;
// scope is reported as "error" so callers can surface the issue.
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
	out := Decision{
		Action:    action,
		TargetEID: targetEID,
		Operator:  operator,
		Decision:  decision,
		Reason:    reason,
		Scope:     scopeForRole(role),
	}
	if strings.TrimSpace(targetEID) != "" {
		res, err := CheckTargetScope(operator, targetEID)
		if err != nil {
			res.Decision = "error"
			res.Reason = "scope lookup failed: " + err.Message
		}
		out.Target = &res
		if decision == "allow" && res.Decision == "deny" {
			out.Decision = "deny"
			out.Reason = res.Reason
		}
	}
	return out
}

// Require denies when either the action-level check or the target-scope check
// fails. An empty targetEID skips the scope check (e.g. list/search APIs).
func Require(action, targetEID string) *errs.Error {
	operator := auth.CurrentOperator()
	role := strings.ToUpper(operator.Role)
	if !(roleActions[role][action] || roleActions[role]["*"]) {
		err := errs.Authorization("action_denied", "action is not allowed for role "+role)
		err.Param = action
		err.Hint = "run perm explain --action " + action
		return err
	}
	if strings.TrimSpace(targetEID) == "" {
		return nil
	}
	return RequireTargetScope(action, operator, targetEID)
}

func scopeForRole(role string) string {
	switch role {
	case "SELF":
		return "self"
	case "HRBP":
		return "hrbp_scope"
	case "MANAGER":
		return "direct_reports"
	case "SSC":
		return "all"
	case "HR_ADMIN":
		return "all"
	default:
		return "none"
	}
}
