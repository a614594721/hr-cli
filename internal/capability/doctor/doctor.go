package doctor

import (
	"hr-cli/internal/db"
	"hr-cli/internal/errs"
)

func Run() (map[string]any, *errs.Error) {
	cfg, cfgErr := db.EnvConfig()
	if cfgErr != nil {
		return nil, cfgErr
	}
	checks := []map[string]any{
		{"name": "db_env", "ok": cfg.Env == "test", "value": cfg.Env},
		{"name": "db_config", "ok": true, "value": map[string]any{"host": cfg.Host, "port": cfg.Port, "name": cfg.Name}},
	}
	conn, _, openErr := db.Open()
	if openErr != nil {
		checks = append(checks, map[string]any{"name": "db_connect", "ok": false, "value": openErr.Message})
		return map[string]any{"checks": checks}, nil
	}
	defer conn.Close()
	for _, table := range []string{"eemployee", "eemployee_work", "personal_info", "attend_information", "skywftask"} {
		var count int
		err := conn.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name=?", cfg.Name, table).Scan(&count)
		if err != nil {
			return nil, errs.DB("query_failed", err.Error())
		}
		checks = append(checks, map[string]any{"name": "table:" + table, "ok": count > 0})
	}
	checks = append(checks, map[string]any{"name": "db_connect", "ok": true})
	return map[string]any{"checks": checks}, nil
}
