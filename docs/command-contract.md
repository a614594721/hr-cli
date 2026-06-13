# Command Contract

The V1a implementation is available through:

```bash
hr <domain> <command> [flags]
```

Global flags:

- `--format json|table`, default `json`.
- `--version`.

## Implemented Commands

Auth:

```bash
hr auth +me
hr auth status
hr auth +login
hr auth +logout
```

Employee:

```bash
hr employee +find --name 张三
hr employee +find --badge A00123
hr employee +find --phone 13800000000
hr employee get --eid 12345
```

Attendance:

```bash
hr attendance +records --badge A00123 --from 2026-06-01 --to 2026-06-13
hr attendance +summary --dept 1001 --date 2026-06-13
hr attendance +exceptions --from 2026-06-01 --to 2026-06-13
```

Approval query:

```bash
hr approval +tasks --limit 20
hr approval +task --task-id 10086
```

Transfer preview:

```bash
hr transfer +preview --badge A00123 --dept 1001 --job 2002 --effect-date 2026-06-20 --reason "组织调整"
hr transfer +apply <preview-id> --dry-run
hr transfer +apply <preview-id> --yes
hr transfer preview show <preview-id>
```

Profile info preview:

```bash
hr profile-info +preview --user-id 6094 --set emergency_contact=李四 --set emergency_phone=13900000000
hr profile-info +apply <preview-id> --yes
```

Raw diagnostics:

```bash
hr db query --sql "select EID,badge,NAME from eemployee where badge=?" --arg A00123
```

Only `SELECT`, `SHOW`, `DESCRIBE`, `DESC`, and `EXPLAIN` are allowed. `CALL`, DDL, and raw writes are blocked.

## Intentionally Not Implemented

These commands return typed policy errors in V1a:

- `approval +approve`
- `approval +reject`
- `approval +transfer`

They require verified database write chains, state-machine checks, audit records, and concurrency protection before activation.

`transfer +apply <preview-id> --dry-run` is implemented as a preflight command. `transfer +apply <preview-id> --yes` executes the native stored-procedure chain only when `DB_ENV=test`, `HR_OPERATOR_URID` is present, and preflight passes.
`profile-info +apply <preview-id> --yes` updates whitelisted `personal_info` fields only when `DB_ENV=test`, preview old-value hashes still match, and apply-time sensitive-field authorization passes.
