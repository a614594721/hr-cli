# DB Capability Inventory

Generated for V1a implementation against `DB_ENV=test`, `DB_NAME=hrmv9`.

This inventory records schema-level findings only. It intentionally avoids dumping secrets or bulk HR/personnel data.

## Summary

| Domain | Current V1a status | Primary objects |
|--------|--------------------|-----------------|
| `employee` | Implemented read-only query/detail | `eemployee`, `employee_dingding` |
| `attendance` | Implemented read-only records/summary/exceptions | `attend_information`, `attend_information_all`, `cattendancefactoremp` |
| `approval` | Implemented task/detail/instance query plus write dry-run | `skywftask`, `skywfinstance`, `skyddoflowaprtasks`, `skyddoriginalflowinsaprinfo`, `skyddoflowaproprcds` |
| `transfer` | Implemented preview only | `eemployee_work`, `eSP_EmpChangeStart`, `eSP_EmpChangeAdd`, `eSP_EmpChangeCheck` |
| `profile-info` | Implemented preview/apply for whitelisted fields in test | `personal_info`, `users`, `EPRE_STAFFREGISTER` |

## Employee

Primary table:

- `eemployee`: 8033 rows observed.
- Key columns: `EID`, `badge`, `NAME`, `DPID`, `DPTITLE`, `JBID`, `JBTITLE`, `REPORTTO`, `REPORTTONAME`, `HRBP`, `STATUS`, `DISABLED`, `MOBILE`, `EMAIL`, `JOINDATE`, `LEAVEDATE`.

Identity mapping:

- `employee_dingding`: 1610 rows observed.
- Key columns: `job_number`, `userid`, `mobile`, `name`, `email`, `dept_id_list`, `title`, `unionid`.

V1a command mapping:

- `hr employee +find --name/--badge/--phone`
- `hr employee get --eid`

Notes:

- Mobile and certificate-like fields are redacted in output.
- Multiple name matches are returned to the caller; agents should narrow with badge or EID.

## Transfer

Primary checked objects:

- `eemployee_work`: work/change staging table.
- `eSP_EmpChangeStart(P_ID int, P_URID int, OUT P_Retval int)`
- `eSP_EmpChangeAdd(P_EID int, P_xType int, P_FromDate datetime, P_RefID int, P_URID int, OUT P_RetVal int)`
- `eSP_EmpChangeCheck(P_ID int, P_URID int, P_TableName varchar, P_FMID int, OUT P_RetVal int)`
- `eSP_EmpChangeCancel(P_ID int, P_UEID int, OUT P_RetVal int)`

Additional read-only findings:

- `eSP_EmpChangeAdd` copies the full employee snapshot from `eemployee` into `eemployee_work`; CLI code should not manually insert a partial staging row.
- `eSP_EmpChangeStart` writes `eemployee_work_all`, deletes the active `eemployee_work` row, calls refresh/sync routines, and can trigger webhooks.
- `eSP_EmpChangeStart` returns `910020` if the work row has not been initialized.
- `eSP_EmpChangeCheck` calls object/form validation and then sets `INITIALIZED=1`.
- Recent active staging rows show `XTYPE=12`, and `eCD_ChangeType.ID=12` is `批量变动`.
- Form metadata maps transfer management to `P_TableName=EEMPLOYEE_WORK`, `P_FMID=110001`.

V1a command mapping:

- `hr transfer +preview --badge ...`
- `hr transfer +apply <preview-id> --dry-run`
- `hr transfer preview show <preview-id>`

Safety conclusion:

- Native apply execution is implemented for `DB_ENV=test` only. It requires `HR_OPERATOR_URID`, uses `P_xType=12`, `P_RefID=0`, `P_TableName=EEMPLOYEE_WORK`, `P_FMID=110001`, writes local audit records, and verifies active work-row removal plus `eemployee_work_all` history creation.

## Profile Info

Primary tables:

- `personal_info`: write entry planned by V4.
- `users`: identity/profile mapping table.

Important `personal_info` fields:

- Normal editable V1a whitelist: `nickname`, `address`, `household_address`, `emergency_contact`, `emergency_phone`, `emergency_relation`, `marital_status`, `computer_preference`, `personal_intro`.
- Sensitive whitelist gated by `--sensitive` and `HR_ADMIN`: `id_number`, `bank_card`, `bank_name`, `branch_name`, `bank_code`, `provident_fund_account`, `phone`.

V1a command mapping:

- `hr profile-info +preview --user-id ... --set field=value`
- `hr profile-info +apply <preview-id> --yes`

Safety conclusion:

- Apply is implemented for `DB_ENV=test` only. Preview stores display-safe diffs plus old-value hashes; raw apply values are stored separately under the local ignored preview secret store. Apply re-checks sensitive-field authorization, locks the `personal_info` row, verifies old-value hashes, updates whitelisted columns, and verifies direct trigger synchronization into `EPRE_STAFFREGISTER`.
- Trigger verification is direct for fields such as `emergency_contact -> EMERGENCYCONTACT`, `emergency_phone -> EMERGENCYTELEPHONE`, `address -> RESIDENCE`, `household_address -> BIRTHPLACE`, `personal_intro -> Personal_Introduction`, and related bank/phone fields. Transformed enum fields still require domain-specific verification.

## Approval

Observed workflow-related tables/views include:

- DingTalk flow pull/import side: `skyddoflowaprtasks`, `skyddoflowaproprcds`, `skyddoriginalflowinsaprinfo`, `skyddoriginalflowinsid`, `skyddoriginalflowparas`.
- Internal workflow side: `skywftask`, `skywfinstance`, `skywfflow`, `skywfflowlog`, `skywftasknotify`, `skywftaskshift`.
- Procedure candidates include `esp_ddflow_delete`, `esp_ddflow_approver_agent`, `esp_ddflow_resubmit`, and multiple domain-specific `skyWF_*` procedures. These were not verified as generic approve/reject/transfer entrypoints.

Observed row counts:

- `skyddoflowaprtasks`: 0 rows.
- `skywftask`: 541 rows.

V1a command mapping:

- `hr approval +tasks`
- `hr approval +task --task-id ...`
- `hr approval +instances --employee ... --status pending`
- `hr approval +approve/+reject/+transfer --task-id ... --dry-run`

Safety conclusion:

- Approval write `--yes` operations are intentionally not implemented. Dry-run returns the task, operator match checks, and unverified native candidates. Do not simulate approval by updating task status columns. Need to verify state machine, native stored procedures or service entrypoint, logs, notifications, and business callbacks first.

## Attendance

Primary tables:

- `attend_information`: 577669 rows observed.
- `attend_information_all`: historical/full variant with matching core columns.
- `cattendancefactoremp`: monthly/factor-style attendance data.

Useful `attend_information` columns:

- Identity/date: `TERM`, `BADGE`, `NAME`, `EID`.
- Card data: `card_times`, `cardbegintime`, `cardendtime`.
- Totals/factors: `attend_total`, `Terms`, `AMOUNT_1477`, `AMOUNT_1478`, `AMOUNT_1540`, `AMOUNT_1541`, `AMOUNT_1543`, `AMOUNT_1571`.
- Exception/comment: `REMARK`.

V1a command mapping:

- `hr attendance +records --badge/--eid`
- `hr attendance +summary --badge/--dept/--date`
- `hr attendance +exceptions`

Notes:

- V1a uses `attend_information` as the stable read source.
- Raw DingTalk punch table `skyddpullattendlistrecord` was present but empty in the inspected database.
