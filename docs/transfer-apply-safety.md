# Transfer Apply Safety Plan

This document records the read-only investigation and the current safe apply skeleton for `transfer +apply`.

## Current Conclusion

`transfer +apply` must use the native HR change chain. It must not hand-build a partial `eemployee_work` row and must not update `eemployee` directly.

The verified native direction is:

1. `CALL eSP_EmpChangeAdd(P_EID, P_xType, P_FromDate, P_RefID, P_URID, OUT P_RetVal)`
2. Update only approved changed fields on the generated `eemployee_work` row.
3. `CALL eSP_EmpChangeCheck(P_ID, P_URID, P_TableName, P_FMID, OUT P_RetVal)`
4. `CALL eSP_EmpChangeStart(P_ID, P_URID, OUT P_RetVal)`
5. Verify `eemployee`, `eemployee_work_all`, and audit result.

Native execution is still disabled in code. The implemented command supports preflight only:

```bash
hr transfer +apply <preview-id> --dry-run
```

`--yes` still stops before write execution until the remaining unknowns below are closed.

## Read-Only Findings

### `eemployee_work`

- `ID` is the only observed index and is an auto-increment primary key.
- Non-null columns include `EZID`, `EID`, `badge`, `CPID`, `DPID`, `JBID`, `WorkStartDate`, `joindate`, and `STATUS`.
- Because many required fields are copied from the current employee snapshot, CLI code should not manually insert the staging row.

### `eSP_EmpChangeAdd`

Parameters:

- `P_EID int`
- `P_xType int`
- `P_FromDate datetime`
- `P_RefID int`
- `P_URID int`
- `OUT P_RetVal int`

Observed behavior:

- Deletes matching prior work rows for the same employee/type/ref combination.
- Inserts a complete `eemployee_work` snapshot selected from `eemployee`.
- Sets `RefType` from `eCD_ChangeType`.
- Sets `Initialized` to `NULL`, `Submit` to `0`, `Closed` to `0`.
- Returns `0` on success and `-1` on SQL exception.

### `eSP_EmpChangeStart`

Parameters:

- `P_ID int`
- `P_URID int`
- `OUT P_Retval int`

Observed behavior:

- Refuses work rows where `IFNULL(initialized,0) = 0`, returning `910020`.
- Updates related part-org and reporting fields.
- Calls event and employee-all refresh routines.
- Inserts the final row into `eemployee_work_all`.
- Deletes the row from `eemployee_work`.
- Runs daily sync when the effect date is current or past.
- Triggers webhook routines after commit for selected change types.

This means apply must either call the correct check/initialization routine before `Start`, or understand which procedure marks the row initialized.

### Current Type Evidence

Recent active `eemployee_work` rows show `XTYPE=12` for recent batch transfer/change rows. The CLI currently treats `12` as the default transfer change type for preflight, but final enablement should confirm the exact type mapping for the product operation.

## Implemented Preflight

`transfer +apply <preview-id> --dry-run` now checks:

- Preview exists and is kind `transfer`.
- Operator identity is resolved.
- `HR_OPERATOR_URID` is present, because native procedures require `P_URID`.
- Current `DPID`/`JBID` old values still match the preview.
- No open `eemployee_work` row exists for the same employee and default transfer type.
- Native execution is still disabled.

## Remaining Enablement Work

Before enabling real writes:

- Confirm the exact `P_xType`, `P_RefID`, `P_TableName`, and `P_FMID` values for this product flow.
- Identify and verify the procedure that marks the work row initialized before `eSP_EmpChangeStart`.
- Decide how to obtain and validate `HR_OPERATOR_URID`.
- Add an audit table/file writer for preview, preflight, stored procedure return codes, verification, and failures.
- Add transaction boundaries around the CLI-controlled field update and stored procedure calls where applicable.
- Add post-apply verification against `eemployee` and `eemployee_work_all`.
- Add integration tests against a dedicated disposable test employee.
