# Changelog

All notable changes to this project will be documented in this file.

## [1.0.0-rc.5] - 2026-08-11

### Fixed

- Build every npm binary with the package version, Git commit, and build date
  injected into `internal/build`, so `hr-cli --version` matches the published
  npm version instead of reporting `0.0.0-dev`.
- Add the missing Windows ARM64 binary. The package already advertised ARM64
  Windows support, but previous releases did not contain that artifact.
- Make the npm package the canonical installation artifact. Installing from
  the Gitee Git repository is not supported because release binaries are
  intentionally excluded from Git.

### Added

- Cross-platform `build:npm` automation for all six supported OS/architecture
  targets.
- A release gate that packs the npm tarball, installs it into a clean temporary
  prefix, verifies all bundled binaries, and asserts that the installed CLI
  reports the same version as `package.json`.

### Changed

- Refresh installation and release documentation for the Gitee source
  repository and the public `@wubang9527/hr-cli` npm package.

## [1.0.0-rc.1] - 2026-06-14

First release candidate. The CLI is feature-complete for read-only and
preview-time HR operations; transfer apply has been validated through
preflight but the end-to-end native write needs a one-shot test employee.
Approval write paths remain disabled by policy.

### Added

- DB-backed identity: `auth +login --name|--badge|--eid` resolves the
  operator from `eemployee`; role auto-resolved from
  `skysecrolemember JOIN skysecuser` (HR_ADMIN > SSC > HRBP > MANAGER >
  SELF).
- DingTalk OAuth login (`--dingtalk`) via the `bi_ehr` auth broker with
  OS-secure token storage and refresh-token rotation.
- `employee +find/get` with badge/name/phone/EID lookups; HRBP/MANAGER
  results are scope-filtered by `psoradiationrangeeidlist`; HR_ADMIN/SSC
  bypass.
- `attendance +records/+summary/+exceptions` read-only commands; both
  `--badge` and `--eid` paths run through the scope gate.
- `approval +tasks/+task/+instances` read-only queries plus dry-run for
  `+approve/+reject/+transfer`. The `--yes` path is intentionally
  disabled in 1.0; the native approve/reject/transfer chain has not been
  verified.
- `transfer +preview` and `transfer +apply` (dry-run + native chain via
  `eSP_EmpChangeAdd → eSP_EmpChangeStart`); old-value match, open work-row
  check, and post-write verification.
- `profile-info +preview/+apply` with whitelisted fields, sensitive
  gate, audit log, old-value hash check, and trigger verification.
- `db query` read-only diagnostics: SELECT/SHOW/DESCRIBE/EXPLAIN single
  statements only; raw write and multi-statement are denied at the
  policy layer.
- `perm explain` shows action-level decision plus target-scope decision
  (`psoradiationrangeeidlist`) for any operator.
- `doctor` self-check for DB env, connection, and required tables.

### Known limitations

- `transfer +apply --yes` is implemented but has not been validated in a
  real end-to-end run; the first run requires a one-shot test employee
  and a rollback plan.
- `profile-info` and `approval` capabilities have action-level
  permissions only; target-scope filtering will land in 1.0.x.
- `attendance +summary` and `+exceptions` aggregate paths run the action
  gate but do not yet filter rows by operator scope.
- SSC role currently shares the HR_ADMIN permission map; sensitive-field
  separation is planned for a later release.
- Audit is double-written to both `.hr-cli/audit/YYYYMMDD.jsonl` and the
  DB table `hr_cli_audit_log`. DB write failures degrade to a stderr
  warning and never block the business operation. Auto-purge / archival
  is delegated to the DBA.
