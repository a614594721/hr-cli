package errs

type Error struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	Message  string `json:"message"`
	Param    string `json:"param,omitempty"`
	Hint     string `json:"hint,omitempty"`
	ExitCode int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

func New(typeName, subtype, message string, exitCode int) *Error {
	return &Error{Type: typeName, Subtype: subtype, Message: message, ExitCode: exitCode}
}

func Validation(subtype, message string) *Error {
	return New("validation", subtype, message, 2)
}

func Config(subtype, message string) *Error {
	return New("config", subtype, message, 2)
}

func Policy(subtype, message string) *Error {
	return New("policy", subtype, message, 3)
}

func Authorization(subtype, message string) *Error {
	return New("authorization", subtype, message, 3)
}

func DB(subtype, message string) *Error {
	return New("db", subtype, message, 4)
}
