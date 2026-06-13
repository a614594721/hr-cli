# HR Approval

Approval query commands:

```bash
hr approval +tasks --assignee me
hr approval +task --task-id <task-id>
hr approval +instances --employee <eid-or-urid> --status pending
```

Approval write dry-run commands:

```bash
hr approval +approve --task-id <task-id> --comment "<comment>" --dry-run
hr approval +reject --task-id <task-id> --reason "<reason>" --dry-run
hr approval +transfer --task-id <task-id> --to-badge <badge> --comment "<comment>" --dry-run
```

Rules:

- `--yes` is intentionally disabled until a native approve/reject/transfer state-machine path is verified.
- Do not simulate approval by updating `skywftask` or `skywfinstance` status fields.
- Dry-run reports the current task, operator match checks, and unverified native candidates.

