# HR Shared

Use `hr doctor` before business operations when database connectivity is uncertain.

Rules:

- Prefer `--format json` for automation.
- Run `hr auth +login --badge <badge>` or another supported identifier to create a DB-backed local session.
- Run `hr auth +me` to confirm operator identity.
- Run `hr auth +logout` to clear the local session.
- Run `hr perm explain --action <action> --target-eid <eid>` before high-risk operations.
- Never place database passwords, session tokens, or employee sensitive data in prompts, docs, commits, or logs.
- Treat `DB_ENV=test` as the only write-enabled environment for V1.
- For high-risk writes, create a preview or dry-run first, inspect it, then apply with `--yes`.
- Current login resolves identity from `eemployee` and `employee_dingding`; it does not validate `users.password`.
