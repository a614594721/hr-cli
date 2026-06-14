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
  -> hr-cli writes local .hr-cli/session.json
```

`access_token` is short lived and is used as the bearer credential for broker
validation. `refresh_token` is long lived, opaque, and rotated by the broker on
each refresh. DingTalk OAuth is required only for the first login or after the
refresh token is revoked or expired.

## Commands

```powershell
hr profile add test --auth-base-url https://your-domain.example.com
hr auth +login --dingtalk
hr auth +me
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

## Current Boundary

The local DB-backed login remains available for controlled test use:

```powershell
hr auth +login --badge P000487
```

Production-facing identity should use `--dingtalk`. The DingTalk app secret and
token signing secret must stay on the broker side and must not be copied into
`hr-cli`.
