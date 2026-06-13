# HR Transfer

Transfer workflow:

```bash
hr transfer +preview --badge <badge> --dept <dept-id> --job <job-id> --effect-date <yyyy-mm-dd> --reason "<reason>"
hr transfer +apply <preview-id> --dry-run
hr transfer +apply <preview-id> --yes
```

Rules:

- Always run preview first.
- Always run `--dry-run` before `--yes`.
- `--yes` requires `DB_ENV=test`, `HR_OPERATOR_URID`, matching old values, an effective department or job change, and no open same-type work row.
- Apply uses `eSP_EmpChangeAdd`, `eemployee_work` field update, `eSP_EmpChangeCheck`, and `eSP_EmpChangeStart`.

