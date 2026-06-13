package transfer

import (
	"hr-cli/internal/auth"
	"hr-cli/internal/capability/employee"
	"hr-cli/internal/errs"
	"hr-cli/internal/preview"
)

func Preview(badge string, dept, job int, effectDate, reason string) (preview.Payload, *errs.Error) {
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
		"capability": "transfer.plan",
		"operator":   auth.CurrentOperator(),
		"target":     target,
		"changes":    changes,
		"write_path": []string{"eEmployee_Work", "eSP_EmpChangeStart", "eemployee/eEmployee_Work_All verification"},
		"status":     "preview_only",
	}
	return preview.Save("transfer", plan)
}
