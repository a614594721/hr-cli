# Full Test Audit - 2026-06-14

## Scope

This audit checks the implementation against `docs/hr-cli产品设计方案.md` after the v4 feature work.

Environment:

- Workspace: `D:\projects\hr-cli`
- Database environment: `DB_ENV=test`
- Database: `hrmv9`
- CLI binary rebuilt: `hr.exe`

## Automated Checks

Passed:

- `go test ./...`
- `go vet ./...`
- `go build -o .\hr.exe .`
- `hr doctor`

## DB-backed Smoke Tests

Passed:

- `auth +me`
- `auth +login --name 吴邦`
- `auth +logout`
- `credential status`
- `perm explain --action transfer.apply --target-eid 108`
- `employee +find --badge G000001`
- `employee get --eid 108`
- `attendance +records --badge G000001 --limit 1`
- `attendance +summary --dept 223 --date 2026-12-25 --limit 1`
- `attendance +exceptions --from 2026-06-01 --to 2026-06-14 --limit 1`
- `approval +task --task-id 206704`
- `approval +instances --status pending --limit 1`
- `approval +approve --task-id 206704 --comment "同意" --dry-run`
- `approval +approve --task-id 206704 --comment "同意" --yes` returned the expected `approval_write_not_verified` policy error.
- `db query --sql "UPDATE eemployee SET NAME=NAME LIMIT 1"` returned the expected `raw_write_denied` policy error.
- `db query --sql "SELECT 1 AS ok; SELECT 2 AS bad"` returned the expected `multi_statement_denied` policy error.
- `db query --sql "SHOW CREATE PROCEDURE esp_ddflow_delete" --limit 1`

## High-risk Operation Tests

Profile info:

- Ran `profile-info +preview` for `user_id=6711` with current `emergency_contact` and `emergency_phone`.
- Ran `profile-info +apply <preview-id> --yes`.
- Result: passed. This was a no-op value write, so it exercised apply, old-value hash verification, audit write, transaction handling, and `EPRE_STAFFREGISTER` trigger verification without changing the data.

Transfer:

- Ran `transfer +preview --badge G000001 --dept 224 --job 547 --effect-date 2026-06-20`.
- Ran `transfer +apply <preview-id> --dry-run`.
- Without `HR_OPERATOR_URID`, dry-run correctly reported `preflight_ok=false`.
- With temporary `HR_OPERATOR_URID=1224`, dry-run reported `preflight_ok=true`.
- After DB-backed login as 吴邦 (`EID/URID=94`), transfer dry-run used session identity and reported `preflight_ok=true`.
- Did not run `transfer +apply --yes` because it would create a real employee change through native stored procedures and needs a dedicated disposable test employee plus rollback plan.

Approval:

- Query and dry-run paths passed.
- `--yes` remains intentionally disabled because the native approve/reject/transfer state-machine entrypoints are not verified.

## Bugs Fixed During Audit

1. `db query` applied `--limit` only after fetching rows.
   - Fixed by wrapping `SELECT` diagnostics as a subquery and applying `LIMIT` in MySQL.
   - `SHOW`, `DESCRIBE`, and `EXPLAIN` keep their existing behavior.

2. `auth +login` and `auth +logout` returned `status=stub`.
   - Fixed to return explicit environment/profile identity-mode status.

3. Operator default role could incorrectly fall back to `HR_ADMIN` when an active profile used a non-test `DB_ENV` and no role was set.
   - Fixed to consider the active profile's `DB_ENV`.

## Completion Assessment

Implemented as usable:

- M0 database inventory.
- M1 CLI framework, config/profile/credential metadata, JSON envelope, typed errors, doctor, raw read-only diagnostics.
- M2 auth identity resolution, permission engine, employee query/detail.
- M3 attendance records/summary/exceptions and approval task/detail/instance query.
- M4 transfer and profile-info preview store/diff/redaction.
- M5 transfer apply implementation, profile-info apply implementation, audit logs, old-value checks.
- M6 approval dry-run and safety boundary.
- HR Agent skills.

Needs user confirmation before enabling further:

- A disposable employee and rollback plan for an end-to-end `transfer +apply --yes` test.
- The true native approval approve/reject/transfer entrypoints.
- Fine-grained target-scope rules for SELF, HRBP, and MANAGER beyond the current action-level permission matrix.
- Whether auth should remain environment/profile based or must integrate a real external login/session provider.
