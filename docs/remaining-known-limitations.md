# Remaining Known Limitations

This file records the remaining items after implementing the v4 plan as far as the verified database chain allows.

## Approval `--yes` Writes

Status: intentionally disabled.

Implemented:

- `approval +tasks`
- `approval +task`
- `approval +instances`
- `approval +approve --dry-run`
- `approval +reject --dry-run`
- `approval +transfer --dry-run`

Not enabled:

- `approval +approve --yes`
- `approval +reject --yes`
- `approval +transfer --yes`

Reason:

- The database contains candidate workflow procedures such as `esp_ddflow_delete`, `esp_ddflow_approver_agent`, `esp_ddflow_resubmit`, and many domain-specific `skyWF_*` procedures.
- The inspected procedures are not a verified generic approve/reject/transfer API.
- `esp_ddflow_delete` updates DingTalk push/task callback state, not the HR approval state machine itself.
- `esp_ddflow_approver_agent` delegates to `esp_ddflow` with an agent, but does not prove generic approve/reject semantics.
- Direct updates to `skywftask` or `skywfinstance` would violate the v4 rule against simulating approval by status-field changes.

Required before enabling:

- Identify the native approval action entrypoint for approve, reject, and transfer.
- Verify current-handler checks, node/action permissions, task and instance state transitions, logs, notifications, callbacks, and business table side effects.
- Add apply-time audit and post-action verification for each action.
- Run against a disposable test workflow instance.

## Target Scope Enforcement

Status: action-level permissions implemented; fine-grained target-scope filtering remains initial.

Implemented:

- Role/action matrix in `internal/perm`.
- `perm explain`.
- Capability-level action checks for employee, attendance, transfer, profile-info, and approval.

Still needed:

- SELF-only target matching for employee/profile/attendance reads.
- HRBP responsible-employee scope checks.
- MANAGER direct-report scope checks.
- Approval node-level permission checks beyond current task handler dry-run reporting.

## Transfer End-to-end Write Test

Status: implementation and dry-run preflight are complete; live `--yes` end-to-end test needs a disposable employee.

Verified:

- Preview creation.
- Dry-run preflight.
- Required `HR_OPERATOR_URID` behavior.
- Old-value match checks.
- Open same-type work-row check.
- Effective department/job change check.

Still needed:

- Select a dedicated test employee that can be safely moved.
- Define the before/after department and job IDs.
- Define the rollback path, either through a reverse transfer or database restore procedure.
- Run `transfer +apply --yes` against that employee and verify `eemployee`, `eemployee_work`, `eemployee_work_all`, audit output, and downstream hooks.

## Credential Backend

Status: profile metadata and credential references implemented; secrets are not stored.

Still needed:

- Native Windows Credential Manager integration from Go, or a documented external credential helper contract.
- Session token storage if future auth is not environment/profile based.
