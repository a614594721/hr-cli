# Command Contract

The V1a implementation is available through:

```bash
python hr.py <domain> <command> [flags]
```

Global flags:

- `--format json|table`, default `json`.
- `--version`.

## Implemented Commands

Auth:

```bash
python hr.py auth +me
python hr.py auth status
python hr.py auth +login
python hr.py auth +logout
```

Employee:

```bash
python hr.py employee +find --name 张三
python hr.py employee +find --badge A00123
python hr.py employee +find --phone 13800000000
python hr.py employee get --eid 12345
```

Attendance:

```bash
python hr.py attendance +records --badge A00123 --from 2026-06-01 --to 2026-06-13
python hr.py attendance +summary --dept 1001 --date 2026-06-13
python hr.py attendance +exceptions --from 2026-06-01 --to 2026-06-13
```

Approval query:

```bash
python hr.py approval +tasks --limit 20
python hr.py approval +task --task-id 10086
```

Transfer preview:

```bash
python hr.py transfer +preview --badge A00123 --dept 1001 --job 2002 --effect-date 2026-06-20 --reason "组织调整"
python hr.py transfer preview show <preview-id>
```

Profile info preview:

```bash
python hr.py profile-info +preview --user-id 6094 --set emergency_contact=李四 --set emergency_phone=13900000000
```

Raw diagnostics:

```bash
python hr.py db query --sql "select EID,badge,NAME from eemployee where badge=?" --arg A00123
```

Only `SELECT`, `SHOW`, `DESCRIBE`, `DESC`, and `EXPLAIN` are allowed. `CALL`, DDL, and raw writes are blocked.

## Intentionally Not Implemented

These commands return typed policy errors in V1a:

- `transfer +apply`
- `profile-info +apply`
- `approval +approve`
- `approval +reject`
- `approval +transfer`

They require verified database write chains, state-machine checks, audit records, and concurrency protection before activation.
