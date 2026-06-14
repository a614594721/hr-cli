package perm

import (
	"strconv"
	"strings"

	"hr-cli/internal/auth"
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
)

// ScopeResult records the outcome of a target-scope check against
// psoradiationrangeeidlist. It is embedded into Decision so callers can
// surface the data source and freshness in their JSON envelopes.
type ScopeResult struct {
	Source    string `json:"source"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	OperEID   string `json:"operator_eid,omitempty"`
	TargetEID string `json:"target_eid,omitempty"`
}

const scopeSource = "psoradiationrangeeidlist"

// CheckTargetScope verifies operator can act on the given target employee.
// HR_ADMIN bypasses the lookup because the underlying PsoRadiationRange_new
// function already returns the full headcount for super-admin EIDs and
// CLI uses role-level allow for HR_ADMIN.
//
// For other roles the check joins on psoradiationrangeeidlist (ueid, eid).
// An empty targetEID short-circuits to allow because some capabilities
// (list/search) do not have a single target.
func CheckTargetScope(operator auth.Operator, targetEID string) (ScopeResult, *errs.Error) {
	role := strings.ToUpper(operator.Role)
	res := ScopeResult{
		Source:    scopeSource,
		OperEID:   operator.EID,
		TargetEID: targetEID,
	}
	if role == "HR_ADMIN" || role == "SSC" {
		res.Decision = "allow"
		res.Reason = "role " + role + " bypasses target-scope check"
		return res, nil
	}
	if strings.TrimSpace(targetEID) == "" {
		res.Decision = "allow"
		res.Reason = "no target_eid supplied; target-scope skipped"
		return res, nil
	}
	if _, convErr := strconv.Atoi(strings.TrimSpace(operator.EID)); convErr != nil {
		res.Decision = "deny"
		res.Reason = "operator eid is not numeric"
		return res, nil
	}
	if _, convErr := strconv.Atoi(strings.TrimSpace(targetEID)); convErr != nil {
		res.Decision = "deny"
		res.Reason = "target_eid is not numeric"
		return res, nil
	}
	row, queryErr := db.QueryOne(
		"SELECT 1 AS hit FROM psoradiationrangeeidlist WHERE ueid=? AND eid=? LIMIT 1",
		operator.EID, targetEID,
	)
	if queryErr != nil {
		return res, queryErr
	}
	if row == nil {
		res.Decision = "deny"
		res.Reason = "target_eid not in operator data radiation range"
		return res, nil
	}
	res.Decision = "allow"
	res.Reason = "target_eid found in operator data radiation range"
	return res, nil
}

// RequireTargetScope returns a policy error when the target is out of scope.
func RequireTargetScope(action string, operator auth.Operator, targetEID string) *errs.Error {
	res, err := CheckTargetScope(operator, targetEID)
	if err != nil {
		return err
	}
	if res.Decision == "allow" {
		return nil
	}
	policyErr := errs.Authorization("target_out_of_scope", res.Reason)
	policyErr.Param = action
	policyErr.Hint = "run perm explain --action " + action + " --target-eid " + targetEID
	return policyErr
}
