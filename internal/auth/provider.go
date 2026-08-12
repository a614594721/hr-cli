package auth

import (
	"context"
	"os"
	"strings"

	"hr-cli/internal/errs"
	appruntime "hr-cli/internal/runtime"
)

const (
	ProviderLegacyGateway = "legacy_gateway"
	ProviderHydra         = "hydra"
)

func resolveLoginProvider(req LoginRequest) (string, *errs.Error) {
	profile, _ := appruntime.ActiveProfile()
	provider := firstNonEmpty(req.Provider, os.Getenv("HR_AUTH_PROVIDER"), profile.AuthProvider)
	if req.ResumeID != "" && req.DingTalk {
		return "", errs.Validation("conflicting_auth_provider", "--resume-id cannot be combined with --dingtalk")
	}
	if req.ResumeID != "" {
		if provider != "" && normalizeProvider(provider) != ProviderHydra && req.Provider != "" {
			return "", errs.Validation("conflicting_auth_provider", "--resume-id requires --provider hydra")
		}
		provider = ProviderHydra
	}
	if req.DingTalk {
		if req.Provider != "" && normalizeProvider(req.Provider) != ProviderLegacyGateway {
			return "", errs.Validation("conflicting_auth_provider", "--dingtalk cannot be combined with --provider hydra")
		}
		provider = ProviderLegacyGateway
	}
	provider = normalizeProvider(provider)
	if provider == "" {
		provider = ProviderLegacyGateway
	}
	if provider != ProviderLegacyGateway && provider != ProviderHydra {
		e := errs.Validation("invalid_auth_provider", "authentication provider must be legacy_gateway or hydra")
		e.Param = "--provider"
		return "", e
	}
	if provider == ProviderHydra && (req.LoginID != "" || req.LoginSecret != "") {
		return "", errs.Validation("conflicting_auth_provider", "--login-id and --login-secret are only valid with legacy_gateway")
	}
	if provider == ProviderHydra && req.ResumeID != "" && req.NoWait {
		return "", errs.Validation("conflicting_login_flags", "--no-wait cannot be combined with --resume-id")
	}
	return provider, nil
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", ProviderLegacyGateway:
		return strings.ToLower(strings.TrimSpace(provider))
	case "legacy", "dingtalk", "dingtalk_oauth":
		return ProviderLegacyGateway
	case ProviderHydra:
		return ProviderHydra
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func sessionProvider(session appruntime.Session) string {
	if provider := normalizeProvider(session.Provider); provider != "" {
		return provider
	}
	return ProviderLegacyGateway
}

func refreshSessionIfNeeded(ctx context.Context, session appruntime.Session, force bool) (appruntime.Session, *errs.Error) {
	switch sessionProvider(session) {
	case ProviderHydra:
		return refreshHydraSessionIfNeeded(ctx, session, force)
	default:
		return refreshLegacySessionIfNeeded(session, force)
	}
}

func loginHint(session appruntime.Session) string {
	if sessionProvider(session) == ProviderHydra {
		return "run hr auth +login --provider hydra"
	}
	return "run hr auth +login --dingtalk"
}
