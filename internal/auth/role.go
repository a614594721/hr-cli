package auth

import (
	"strconv"
	"strings"

	"hr-cli/internal/db"
	"hr-cli/internal/errs"
)

// dbRoleMap maps skysecrole.ID to the hr-cli role used for capability/scope
// gating. Roles outside this set grant no hr-cli capability.
//
// Role priority (highest first): HR_ADMIN > SSC > HRBP > MANAGER > SELF.
var dbRoleMap = map[int]string{
	181084: "HR_ADMIN", // admin-临时
	181090: "SSC",
	181089: "HRBP",
	181083: "MANAGER", // 一级部门负责人自助
	1001:   "SELF",    // 信飞PC-员工自助
}

var rolePriority = []string{"HR_ADMIN", "SSC", "HRBP", "MANAGER", "SELF"}

// ResolveDBRoleByEID joins skysecrolemember to skysecuser by URID->ID and
// returns the highest-priority hr-cli role granted to the EID. The full
// granted set is returned in priority order so callers can show "active /
// other roles".
//
// Returns ("", nil, nil) when the EID has no enabled mapped roles.
// On a DB error the caller decides how to fall back; this function does not
// fabricate a default role.
func ResolveDBRoleByEID(eid string) (string, []string, *errs.Error) {
	trimmed := strings.TrimSpace(eid)
	if trimmed == "" {
		return "", nil, nil
	}
	if _, convErr := strconv.Atoi(trimmed); convErr != nil {
		return "", nil, nil
	}
	rows, err := db.QueryRows(
		`SELECT DISTINCT m.RUID
		 FROM skysecrolemember m
		 JOIN skysecuser u ON u.ID = m.URID
		 WHERE u.EID = ?
		   AND COALESCE(m.DISABLED,0) = 0
		   AND COALESCE(u.DISABLED,0) = 0`,
		trimmed,
	)
	if err != nil {
		return "", nil, err
	}
	granted := map[string]bool{}
	for _, row := range rows {
		ruid := toInt(row["RUID"])
		if mapped, ok := dbRoleMap[ruid]; ok {
			granted[mapped] = true
		}
	}
	if len(granted) == 0 {
		return "", nil, nil
	}
	ordered := make([]string, 0, len(granted))
	for _, role := range rolePriority {
		if granted[role] {
			ordered = append(ordered, role)
		}
	}
	return ordered[0], ordered, nil
}

// HighestDBRole returns the highest-priority hr-cli role from a slice.
func HighestDBRole(roles []string) string {
	set := map[string]bool{}
	for _, r := range roles {
		set[strings.ToUpper(strings.TrimSpace(r))] = true
	}
	for _, r := range rolePriority {
		if set[r] {
			return r
		}
	}
	return ""
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}
