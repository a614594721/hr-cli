# Ory Hydra Device Authentication

`hr-cli` can authenticate as the public OAuth client `hr-cli` with Ory Hydra's
Device Authorization Grant. Hydra issues the platform access and refresh
tokens; DingTalk or another external identity provider is used by the
server-side login and consent UI to prove the user's identity.

Hydra mode is opt-in. Existing profiles continue to use the legacy gateway
broker until `auth_provider=hydra` or `--provider hydra` is selected.

## Profile

Use the Hydra issuer so the CLI can resolve endpoints from OIDC discovery:

```powershell
hr profile add hydra-qa `
  --auth-base-url https://gateway-qa.example.com `
  --auth-provider hydra `
  --hydra-issuer https://auth-qa.example.com `
  --hydra-client-id hr-cli `
  --hydra-audience hr-gateway `
  --hydra-scope "openid offline_access hr.employee.read"
hr profile use hydra-qa
```

The discovery document must provide:

- `device_authorization_endpoint`, normally `/oauth2/device/auth` on Hydra
  v26.2.0;
- `token_endpoint`, normally `/oauth2/token`;
- `revocation_endpoint`, normally `/oauth2/revoke`.

Explicit `--hydra-device-endpoint`, `--hydra-token-endpoint`, and
`--hydra-revoke-endpoint` values are supported for installations where
discovery cannot publish the externally reachable URLs. Non-local issuers and
OAuth endpoints must use HTTPS. Plain HTTP is accepted only for `localhost`,
`127.0.0.1`, or `::1` development.

The `hr-cli` client is public and must be registered with
`token_endpoint_auth_method=none`. The CLI never accepts, stores, or sends a
client secret.

## Login

Interactive login:

```powershell
hr auth +login --provider hydra
```

The CLI requests `client_id=hr-cli`, an `offline_access` scope, and
`audience=hr-gateway`. It opens `verification_uri_complete` when Hydra returns
one; otherwise it opens `verification_uri` and prints the user code. Without
an explicit `--timeout`, polling continues until Hydra's device code expiry or
command cancellation. `--timeout <seconds>` applies a shorter caller cap.

For an AI agent or remote shell:

```powershell
hr auth +login --provider hydra --no-wait
hr auth +login --resume-id <resume_id>
```

The pending device code is stored in the existing OS keychain. Command output
contains only a random `resume_id`, verification URL, and user code; it does
not expose the OAuth device code.

## Refresh And Logout

Business commands continue to call the existing hr-gateway HTTP routes with a
Bearer access token. The CLI refreshes close-to-expiry tokens through Hydra's
refresh-token grant and stores rotated tokens in the OS keychain.

```powershell
hr auth status
hr auth status --verify
hr auth +logout
```

Logout attempts RFC 7009 revocation for the refresh and access tokens, then
clears the local keychain entry and non-secret session metadata. A live login
requires the Hydra public client, device login/consent UI, issuer routing, and
gateway Hydra-token validation to be configured by the owning services.
