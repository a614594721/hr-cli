# HR Attendance

Attendance query commands:

```bash
hr attendance +records --badge <badge> --from <yyyy-mm-dd> --to <yyyy-mm-dd>
hr attendance +summary --dept <dept-id> --date <yyyy-mm-dd>
hr attendance +exceptions --from <yyyy-mm-dd> --to <yyyy-mm-dd>
```

Rules:

- V1 attendance commands are read-only.
- Prefer badge or EID for employee-specific records.
- Use summary for department-level review and exceptions for rows with remarks.

