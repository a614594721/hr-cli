# DingTalk OAuth Auth Broker

`hr-cli` supports a DingTalk OAuth login path through an external Auth Broker.
The first broker implementation is expected to run in `bi_ehr` under the
`/api/hr-cli/auth/*` and `/auth/hr-cli/*` routes.

## CLI Flow

```text
hr auth +login --dingtalk --auth-base-url https://your-domain.example.com
  -> POST /api/hr-cli/auth/login/start
  -> open /auth/hr-cli/start?login_id=...
  -> DingTalk OAuth callback to /auth/hr-cli/callback
  -> CLI polls /api/hr-cli/auth/login/poll
  -> broker returns access_token and refresh_token once
  -> hr-cli stores tokens in OS secure storage and writes non-secret session metadata
```

`access_token` is short lived and is used as the bearer credential for broker
validation. `refresh_token` is long lived, opaque, and rotated by the broker on
each refresh. DingTalk OAuth is required only for the first login or after the
refresh token is revoked or expired.

Local storage follows the Lark CLI pattern:

- `access_token` and `refresh_token` are stored in the OS secure credential
  store. On Windows this uses Windows Credential Manager.
- `.hr-cli/session.json` stores only non-secret identity metadata such as EID,
  badge, name, role, broker URL, and token expiration timestamps.
- Tokens are keyed by broker base URL and employee EID so different brokers or
  operators do not share credentials.
- Access tokens are refreshed five minutes before expiry. Refresh uses a
  process-safe lock under `.hr-cli/locks` so concurrent commands do not race on
  rotated refresh tokens.

## Commands

```powershell
hr profile add test --auth-base-url https://your-domain.example.com
hr auth +login --dingtalk
hr auth +me
hr auth status
hr auth status --verify
hr auth +logout
```

You can also avoid writing the broker URL into the profile:

```powershell
$env:HR_AUTH_BASE_URL = "https://your-domain.example.com"
hr auth +login --dingtalk
```

Use `--no-browser` when running in an environment that cannot open a browser;
the command prints the login URL and keeps polling until the login completes or
times out.

Use `--no-wait` for agent or remote-shell workflows where the command should not
block while the user completes browser authorization:

```powershell
hr auth +login --dingtalk --no-wait
# open auth_url in the returned JSON, then after browser authorization:
hr auth +login --dingtalk --login-id <login_id> --login-secret <login_secret>
```

`auth status` reads local session/token state only. `auth status --verify`
validates against the broker and may refresh the local access token.

## Current Boundary

The local DB-backed login remains available for controlled test use:

```powershell
hr auth +login --badge P000487
```

Production-facing identity should use `--dingtalk`. The DingTalk app secret and
token signing secret must stay on the broker side and must not be copied into
`hr-cli`.
