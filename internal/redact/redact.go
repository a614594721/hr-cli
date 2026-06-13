package redact

func Phone(v any) any {
	if v == nil {
		return nil
	}
	s := toString(v)
	if len(s) < 7 {
		return "***"
	}
	return s[:3] + "****" + s[len(s)-4:]
}

func ID(v any) any {
	if v == nil {
		return nil
	}
	s := toString(v)
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "********" + s[len(s)-4:]
}

func Employee(row map[string]any) map[string]any {
	out := clone(row)
	for _, key := range []string{"MOBILE", "mobile", "phone", "PHONE"} {
		if value, ok := out[key]; ok {
			out[key] = Phone(value)
		}
	}
	for _, key := range []string{"CERTNO", "id_number", "ID_NUMBER"} {
		if value, ok := out[key]; ok {
			out[key] = ID(value)
		}
	}
	return out
}

func clone(row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		out[k] = v
	}
	return out
}

func toString(v any) string {
	switch value := v.(type) {
	case []byte:
		return string(value)
	case string:
		return value
	default:
		return ""
	}
}
