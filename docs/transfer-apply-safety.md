# Transfer Apply Safety Plan

This document records the read-only investigation and the native apply implementation for `transfer +apply`.

## Current Conclusion

`transfer +apply` must use the native HR change chain. It must not hand-build a partial `eemployee_work` row and must not update `eemployee` directly.

The verified native direction is:

1. `CALL eSP_EmpChangeAdd(P_EID, P_xType, P_FromDate, P_RefID, P_URID, OUT P_RetVal)`
2. Update only approved changed fields on the generated `eemployee_work` row.
3. `CALL eSP_EmpChangeCheck(P_ID, P_URID, P_TableName, P_FMID, OUT P_RetVal)`
4. `CALL eSP_EmpChangeStart(P_ID, P_URID, OUT P_RetVal)`
5. Verify `eemployee`, `eemployee_work_all`, and audit result.

The command supports preflight:

```bash
hr transfer +apply <preview-id> --dry-run
```

Native write execution is enabled only for `DB_ENV=test`:

```bash
set HR_OPERATOR_URID=100171
hr transfer +apply <preview-id> --yes
```

The command writes audit records under `.hr-cli/audit/`.

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

`eSP_EmpChangeCheck` is the check/initialization routine. It calls `esp_objnameckeck(...)` and then sets `INITIALIZED=1`, `INITIALIZEDBY`, and `INITIALIZEDTIME` on `eemployee_work`.

### Current Type Evidence

Recent active `eemployee_work` rows show `XTYPE=12` for recent batch transfer/change rows. `eCD_ChangeType.ID=12` is titled `批量变动`, and the CLI currently uses this as the transfer change type.

Form metadata lookup found:

- `P_TableName = EEMPLOYEE_WORK`
- `P_FMID = 110001`, form title `调动管理`
- `P_RefID = 0`

## Implemented Preflight

`transfer +apply <preview-id> --dry-run` checks:

- Preview exists and is kind `transfer`.
- Operator identity is resolved.
- `HR_OPERATOR_URID` is present, because native procedures require `P_URID`.
- Current `DPID`/`JBID` old values still match the preview.
- No open `eemployee_work` row exists for the same employee and default transfer type.
- The preview contains a real `DPID` or `JBID` change, not a no-op.
- The preview contains `EFFECTDATE`.

`transfer +apply <preview-id> --yes` executes:

1. Write `transfer.apply.start` audit event.
2. Call `eSP_EmpChangeAdd`.
3. Locate the generated `eemployee_work.ID`.
4. Update only approved fields: `DPID`, `JBID`, `EFFECTDATE`, `reason`, `CHGCOMMENT`.
5. Call `eSP_EmpChangeCheck`.
6. Call `eSP_EmpChangeStart`.
7. Verify active work row removal and `eemployee_work_all` history creation.
8. Write success or failure audit event.

## Remaining Enablement Work

Still needed:

- Replace environment `HR_OPERATOR_URID` with a real auth/session mapping.
- Add a dedicated disposable test employee and run an end-to-end write test.
- Expand audit from local jsonl to the final audit sink.
- Document stored procedure return-code meanings.
