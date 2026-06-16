package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"hr-cli/internal/errs"
)

// DefaultAuthBaseURL is the production hr-gateway URL baked into the
// distributed binary. Users who install via npm hit this gateway without
// any profile setup; `hr-cli profile add ... --auth-base-url ...` still
// works as an override for local development.
const DefaultAuthBaseURL = "http://139.224.32.44:3002"

type Config struct {
	CurrentProfile string             `json:"current_profile,omitempty"`
	Profiles       map[string]Profile `json:"profiles,omitempty"`
}

// Profile is the gateway-only client profile. After the gateway cutover the
// client never reads a database directly: there is no DB host/user/credential
// to track here, and the operator identity is derived from the JWT issued by
// hr-gateway, not from local fields.
type Profile struct {
	AuthBaseURL string `json:"auth_base_url,omitempty"`
}

func Init() (map[string]any, *errs.Error) {
	cfg := Config{Profiles: map[string]Profile{}}
	if existing, err := Load(); err == nil {
		cfg = existing
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if err := save(cfg); err != nil {
		return nil, err
	}
	path, _ := filepath.Abs(configPath())
	return map[string]any{"path": path, "current_profile": cfg.CurrentProfile, "profiles": len(cfg.Profiles)}, nil
}

func Load() (Config, *errs.Error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{Profiles: map[string]Profile{}}, errs.Config("config_not_initialized", "local config has not been initialized")
		}
		return Config{}, errs.Config("config_read_failed", err.Error())
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, errs.Config("config_parse_failed", err.Error())
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return cfg, nil
}

func Show() (map[string]any, *errs.Error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	path, _ := filepath.Abs(configPath())
	return map[string]any{"path": path, "current_profile": cfg.CurrentProfile, "profiles": cfg.Profiles}, nil
}

func AddProfile(name string, profile Profile) (map[string]any, *errs.Error) {
	if name == "" {
		e := errs.Validation("missing_profile_name", "profile name is required")
		e.Param = "name"
		return nil, e
	}
	cfg, _ := Load()
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	cfg.Profiles[name] = profile
	if cfg.CurrentProfile == "" {
		cfg.CurrentProfile = name
	}
	if err := save(cfg); err != nil {
		return nil, err
	}
	return map[string]any{"name": name, "profile": profile, "current_profile": cfg.CurrentProfile}, nil
}

func UseProfile(name string) (map[string]any, *errs.Error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	profile, ok := cfg.Profiles[name]
	if !ok {
		e := errs.Validation("profile_not_found", "profile not found")
		e.Param = "name"
		return nil, e
	}
	cfg.CurrentProfile = name
	if err := save(cfg); err != nil {
		return nil, err
	}
	return map[string]any{"current_profile": name, "profile": profile}, nil
}

func ListProfiles() (map[string]any, *errs.Error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for name, profile := range cfg.Profiles {
		items = append(items, map[string]any{
			"name":          name,
			"active":        name == cfg.CurrentProfile,
			"auth_base_url": profile.AuthBaseURL,
		})
	}
	return map[string]any{"items": items, "current_profile": cfg.CurrentProfile}, nil
}

func ActiveProfile() (Profile, bool) {
	cfg, err := Load()
	if err != nil || cfg.CurrentProfile == "" {
		return Profile{AuthBaseURL: DefaultAuthBaseURL}, true
	}
	profile, ok := cfg.Profiles[cfg.CurrentProfile]
	if !ok {
		return Profile{AuthBaseURL: DefaultAuthBaseURL}, true
	}
	if profile.AuthBaseURL == "" {
		profile.AuthBaseURL = DefaultAuthBaseURL
	}
	return profile, true
}

func save(cfg Config) *errs.Error {
	dir := filepath.Dir(configPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Config("config_write_failed", err.Error())
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return errs.Config("config_encode_failed", err.Error())
	}
	if err := os.WriteFile(configPath(), data, 0o600); err != nil {
		return errs.Config("config_write_failed", err.Error())
	}
	return nil
}

func configPath() string {
	return filepath.Join(".", ".hr-cli", "config.json")
}
