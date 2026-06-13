# Error Contract

All command failures are emitted as JSON error envelopes on stderr.

```json
{
  "ok": false,
  "error": {
    "type": "policy",
    "subtype": "raw_write_denied",
    "message": "raw diagnostics only allow SELECT, SHOW, DESCRIBE, DESC, or EXPLAIN",
    "param": "--sql"
  },
  "meta": {
    "command": "db.query",
    "db_env": "test",
    "db_name": "hrmv9"
  }
}
```

## Types

| Type | Meaning |
|------|---------|
| `validation` | Bad or incomplete user input, not found, multiple matches |
| `authentication` | Missing or invalid operator identity |
| `authorization` | Role, target, or field permission denied |
| `config` | Missing environment or dependency |
| `db` | MySQL connection/query failure |
| `policy` | Safety boundary blocked the action |
| `confirmation` | High-risk operation lacks explicit confirmation |
| `internal` | Unexpected bug |

## V1a Policy Errors

The following policy errors are expected and intentional:

- `raw_write_denied`: raw SQL is not read-only.
- `multi_statement_denied`: raw SQL contains multiple statements.
- `apply_not_implemented`: high-risk apply command is disabled in V1a.
- `approval_write_not_verified`: approval write path has not been verified.
