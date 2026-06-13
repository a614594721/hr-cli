package auth

import "os"

type Operator struct {
	EID    string `json:"eid,omitempty"`
	Badge  string `json:"badge,omitempty"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Source string `json:"source"`
}

func CurrentOperator() Operator {
	role := os.Getenv("HR_OPERATOR_ROLE")
	if role == "" {
		if os.Getenv("DB_ENV") == "test" || os.Getenv("DB_ENV") == "" {
			role = "HR_ADMIN"
		} else {
			role = "SELF"
		}
	}
	name := os.Getenv("HR_OPERATOR_NAME")
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if name == "" {
		name = "local-operator"
	}
	return Operator{
		EID:    os.Getenv("HR_OPERATOR_EID"),
		Badge:  os.Getenv("HR_OPERATOR_BADGE"),
		Name:   name,
		Role:   role,
		Source: "environment",
	}
}
