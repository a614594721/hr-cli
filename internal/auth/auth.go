package auth

import (
	"os"

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

func CurrentOperator() Operator {
	profile, hasProfile := runtime.ActiveProfile()
	role := firstNonEmpty(os.Getenv("HR_OPERATOR_ROLE"), profile.OperatorRole)
	if role == "" {
		if os.Getenv("DB_ENV") == "test" || os.Getenv("DB_ENV") == "" {
			role = "HR_ADMIN"
		} else {
			role = "SELF"
		}
	}
	name := firstNonEmpty(os.Getenv("HR_OPERATOR_NAME"), profile.OperatorName)
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if name == "" {
		name = "local-operator"
	}
	source := "environment"
	if os.Getenv("HR_OPERATOR_NAME") == "" && hasProfile && profile.OperatorName != "" {
		source = "profile"
	}
	return Operator{
		EID:    firstNonEmpty(os.Getenv("HR_OPERATOR_EID"), profile.OperatorEID),
		URID:   firstNonEmpty(os.Getenv("HR_OPERATOR_URID"), profile.OperatorURID),
		Badge:  firstNonEmpty(os.Getenv("HR_OPERATOR_BADGE"), profile.OperatorBadge),
		Name:   name,
		Role:   role,
		Source: source,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
