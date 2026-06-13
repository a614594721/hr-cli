# hr-cli 方案 v4：基于 Lark-CLI 框架的 DB-backed HR 能力网关

> 本文档是当前产品落地基线。
> 产品初始能力不止人员调动和个人资料修改，还包括员工信息查询、审批流操作、打卡信息查询；系统底层基于 MySQL，没有原生 OpenAPI。

## 0. 结论

方案需要继续补充，但方向不变。

借鉴 Lark-CLI 时，不能照搬它的“OpenAPI 元数据 -> API 命令 -> Raw API”模型，因为 HR 系统没有原生 API。我们应该借鉴的是它的框架思想：

- 人和 AI Agent 都能稳定调用。
- 命令按业务域组织。
- 输出和错误结构化。
- 安全策略在框架层统一执行。
- 高风险操作必须 dry-run / preview / apply。
- 凭证、profile、审计、doctor、skills 都是一等能力。

对 HR CLI 来说，三层命令应调整为：

```text
Shortcuts 快捷业务命令
  -> DB Capability Commands 数据库能力命令
  -> Restricted Raw Diagnostics 受限原始诊断
```

其中第二层不是外部 API，而是我们在 MySQL 表、视图、trigger、存储过程、审批表、考勤表之上封装出来的稳定能力层。

## 1. 初始产品范围

V1 目标能力应扩展为五个业务域：

| 业务域 | 能力 | 风险等级 | V1 策略 |
|--------|------|----------|---------|
| `auth` | 用户鉴权、身份映射、权限判断 | 高 | 先做，所有业务命令前置依赖 |
| `employee` | 查员工信息 | 低到中 | V1 必做，默认只读 |
| `transfer` | 操作人员调动 | 高 | V1 做单人 preview/apply |
| `profile-info` | 修改人员信息 | 高 | V1 做白名单字段 preview/apply |
| `approval` | 审批流查询和操作 | 高 | V1 建议先查询/处理待办，写操作需单独核实链路 |
| `attendance` | 打卡信息查询 | 低到中 | V1 做只读查询 |

如果要控制第一版范围，建议把 V1 分成：

- V1a：鉴权、员工查询、打卡查询、调动 preview、资料修改 preview。
- V1b：调动 apply、资料修改 apply。
- V1c：审批流操作。

审批流写操作不要和调动 apply 同时冒进，因为审批流通常涉及状态机、节点、任务、日志、通知和回调，必须先把数据库链路探清楚。

## 2. 架构修订

### 2.1 总体分层

```text
CLI Command Layer
  Cobra 命令、参数、帮助、补全、输出格式

Shortcut Layer
  面向人和 Agent 的高频业务命令
  例如 employee +find、transfer +preview、attendance +records

DB Capability Layer
  对 MySQL 表、存储过程、trigger、审批链路的稳定封装
  例如 employee.get、transfer.plan、approval.task.approve

Policy Layer
  鉴权、字段权限、目标范围、环境保护、确认策略

Data Access Layer
  参数化 SQL、事务、存储过程调用、只读 raw query

Audit / Preview / Output Layer
  preview 文件、审计日志、脱敏、JSON envelope、typed error
```

### 2.2 为什么需要 DB Capability Layer

没有原生 API 时，不能让命令直接散落 SQL。否则后续会出现：

- 同一张表在多个命令里重复拼 SQL。
- 业务规则绕过权限和审计。
- 审批流或调动链路被半模拟，造成状态不一致。
- Agent 为了完成任务直接尝试 raw SQL 写库。

DB Capability Layer 的职责是把数据库能力包装成稳定接口：

```text
EmployeeCapability
  FindEmployee(criteria)
  GetEmployeeSnapshot(eid)

TransferCapability
  BuildTransferPlan(operator, target, changes)
  ApplyTransferPlan(plan)

ProfileInfoCapability
  BuildProfileInfoPlan(operator, target, changes)
  ApplyProfileInfoPlan(plan)

ApprovalCapability
  ListTasks(operator)
  GetTask(taskID)
  Approve(taskID, opinion)
  Reject(taskID, reason)
  TransferTask(taskID, nextOperator)

AttendanceCapability
  QueryRecords(target, dateRange)
  SummarizeRecords(target, dateRange)
```

命令层只调用 capability，不直接拼业务 SQL。

## 3. 命令设计修订

### 3.1 Auth

```bash
hr auth +login
hr auth +me
hr auth +logout
hr auth status
hr perm explain --action transfer.apply --target-eid 12345
```

规则：

- 鉴权通过后才能操作有权限的人员。
- 钉钉或企业身份只解决“操作者是谁”。
- HR CLI 的 Permission Engine 负责“能不能操作这个人、这个动作、这些字段”。
- apply 类命令必须在执行前重新解析 operator，不能复用 preview 阶段鉴权结果。

### 3.2 Employee

```bash
hr employee +find --name 张三
hr employee +find --badge A00123
hr employee +find --phone 13800000000
hr employee get --eid 12345
```

输出建议包含：

- EID、工号、姓名。
- 部门、岗位、汇报对象、HRBP。
- 在职状态。
- 手机号脱敏。
- 钉钉身份是否已绑定。
- 可操作动作摘要：例如 `can_transfer=false`、`can_edit_profile=true`。

多人匹配必须停止，不能猜。

### 3.3 Transfer

```bash
hr transfer +preview --badge A00123 --dept 1001 --job 2002 --effect-date 2026-06-20 --reason "组织调整"
hr transfer +apply <preview-id> --yes
hr transfer preview show <preview-id>
```

底层仍采用已核实方向：

```text
eEmployee_Work -> eSP_EmpChangeStart -> eemployee / eEmployee_Work_All 验证
```

### 3.4 Profile Info

```bash
hr profile-info +preview --user-id 6094 --set emergency_contact=李四 --set emergency_phone=13900000000
hr profile-info +apply <preview-id> --yes
```

规则：

- 以 `personal_info` 为写入口。
- 字段必须白名单化。
- 高敏字段需要 HR_ADMIN + sensitive 模式。
- 修改后验证 trigger 同步结果。

### 3.5 Approval

审批流需要拆成查询和操作两类。

查询：

```bash
hr approval +tasks
hr approval +tasks --assignee me
hr approval +task --task-id 10086
hr approval +instances --employee 12345 --status pending
```

操作：

```bash
hr approval +approve --task-id 10086 --comment "同意"
hr approval +reject --task-id 10086 --reason "资料不完整"
hr approval +transfer --task-id 10086 --to-badge A00123 --comment "转交处理"
```

落地原则：

- V1 先做审批任务查询和详情。
- 写操作必须先核实审批流状态机和数据库链路。
- 不允许只改任务状态字段来“模拟审批”。
- 如果系统已有审批存储过程或业务入口，必须优先调用原生入口。
- 审批操作也应采用 preview/apply 或至少 `--dry-run` + `--yes`。

建议审批操作流程：

```text
approval +approve
  解析 operator
  查询 task/instance 当前状态
  校验 task 当前处理人是否为 operator
  查询可执行动作
  dry-run 输出将改变的任务、实例、节点、日志
  --yes 后调用原生审批链路
  验证状态迁移和日志
  写审计
```

### 3.6 Attendance

```bash
hr attendance +records --badge A00123 --from 2026-06-01 --to 2026-06-13
hr attendance +summary --dept 1001 --date 2026-06-13
hr attendance +exceptions --from 2026-06-01 --to 2026-06-13
```

规则：

- V1 只读。
- HRBP 只能查自己负责范围。
- SELF 只能查自己。
- HR_ADMIN 可查全部。
- 输出要脱敏定位、设备、异常说明中的敏感信息。

## 4. 权限模型补充

动作级权限扩展：

```text
auth.me
employee.find
employee.get
transfer.preview
transfer.apply
profile_info.preview
profile_info.apply
profile_info.apply_sensitive
approval.task.list
approval.task.get
approval.task.approve
approval.task.reject
approval.task.transfer
attendance.records.query
attendance.summary.query
```

目标范围：

| 角色 | 员工查询 | 调动 | 资料修改 | 审批 | 打卡 |
|------|----------|------|----------|------|------|
| SELF | 自己 | 不允许 | 自己部分字段 | 自己待办 | 自己 |
| HRBP | 负责员工 | 负责员工 | 负责员工普通字段 | 负责范围内待办或本人待办 | 负责员工 |
| MANAGER | 下属查询 | V1 不允许 | V1 不允许 | 本人待办 | 下属摘要，细节待定 |
| HR_ADMIN | 全部 | 全部 | 全部，含高敏闸门 | 全部或按审批权限 | 全部 |

审批权限不能只按 HRBP/ADMIN 推断，还必须结合审批任务当前处理人、节点权限和流程定义。

## 5. 没有原生 API 时的探查清单

在实现前，需要对 MySQL 做能力盘点。按业务域输出 `docs/db-capability-inventory.md`：

### 5.1 Employee

- 员工主表。
- 部门表。
- 岗位表。
- 在职状态字段。
- HRBP、主管、部门负责人关系。

### 5.2 Transfer

- 已核实的 `eEmployee_Work`、`eSP_EmpChangeStart`、历史表。
- 补充字段默认值和必填列。
- 准备测试员工和可回滚策略。

### 5.3 Profile Info

- `personal_info` 字段和 `users` 映射。
- trigger 同步目标。
- 字段敏感级别。

### 5.4 Approval

必须探查：

- 审批实例表。
- 审批任务表。
- 节点表。
- 审批日志表。
- 当前处理人字段。
- 可执行动作字段或状态机。
- 同意、拒绝、转交是否已有存储过程。
- 审批完成后是否触发业务表更新、通知、webhook。

没有核实前，不实现审批写操作。

### 5.5 Attendance

必须探查：

- 打卡记录表。
- 班次、排班、请假、外勤、补卡、异常表。
- 时间字段时区。
- 员工身份映射。
- 异常状态计算是落表还是运行时计算。

V1 如果异常计算复杂，可以先提供原始打卡记录查询，再做 summary。

## 6. Lark-CLI 框架映射修订

| Lark-CLI 概念 | HR CLI 对应 |
|---------------|-------------|
| service domain | `employee`、`transfer`、`profile-info`、`approval`、`attendance` |
| shortcuts | `+find`、`+preview`、`+apply`、`+records`、`+tasks` |
| API commands | DB Capability Commands，不是外部 API |
| raw API | 只读 DB diagnostic query，不允许 raw write |
| auth identity | HR operator identity |
| scopes | HR action permissions + field permissions + target scope |
| skills | HR Agent runbooks |
| dry-run | preview 或只读执行计划 |
| typed errors | HR CLI error envelope |
| keychain | session token / local credential storage |

## 7. 目录结构修订

```text
D:\projects\hr-cli\
├── cmd/
│   ├── auth/
│   ├── employee/
│   ├── transfer/
│   ├── profileinfo/
│   ├── approval/
│   ├── attendance/
│   ├── perm/
│   ├── db/
│   └── doctor/
├── shortcuts/
│   ├── auth/
│   ├── employee/
│   ├── transfer/
│   ├── profileinfo/
│   ├── approval/
│   └── attendance/
├── internal/
│   ├── capability/
│   │   ├── employee/
│   │   ├── transfer/
│   │   ├── profileinfo/
│   │   ├── approval/
│   │   └── attendance/
│   ├── auth/
│   ├── perm/
│   ├── db/
│   ├── preview/
│   ├── audit/
│   ├── output/
│   ├── errs/
│   ├── redact/
│   └── runtime/
├── skills/
│   ├── hr-shared/
│   ├── hr-employee/
│   ├── hr-transfer/
│   ├── hr-profile-info/
│   ├── hr-approval/
│   └── hr-attendance/
└── docs/
    ├── db-capability-inventory.md
    ├── command-contract.md
    └── error-contract.md
```

## 8. 里程碑修订

### M0：数据库能力盘点

- 输出 `docs/db-capability-inventory.md`。
- 明确 employee / transfer / profile-info / approval / attendance 的表和存储过程。
- 对审批流写操作给出是否可安全落地的结论。

### M1：CLI 框架

- Go + Cobra root。
- config / profile / credential。
- JSON envelope。
- typed error。
- doctor。
- 只读 raw query 策略。

### M2：鉴权和员工查询

- `auth +login/+me`。
- operator identity。
- Permission Engine 初版。
- `employee +find/get`。

### M3：只读业务能力

- `attendance +records`。
- `attendance +summary` 初版。
- `approval +tasks/+task` 查询。

### M4：高风险 preview

- `transfer +preview`。
- `profile-info +preview`。
- preview store、diff、脱敏。

### M5：高风险 apply

- `transfer +apply`。
- `profile-info +apply`。
- 审计日志。
- 并发旧值校验。

### M6：审批操作

- 仅在审批链路核实后实现。
- `approval +approve/+reject/+transfer`。
- dry-run / confirmation / audit。

## 9. 旧方案结论的吸收和调整

这些结论保留：

- Go + Cobra。
- Lark-CLI 风格命令框架。
- stdout/stderr 分离。
- typed error。
- profile / credential / strict mode。
- preview/apply。
- raw 写库禁止。

需要补充或改名：

- `person` 改成 `employee`，更贴近产品域。
- 第二层从 `API commands` 改成 `DB Capability Commands`。
- 初始业务域加入 `approval` 和 `attendance`。
- V1 里程碑先做只读 employee / attendance / approval query，再做高风险 apply。
- 审批写操作必须单独探查状态机，不能简单 UPDATE 审批状态字段。

## 10. 最终建议

后续以 v4 为产品落地基线。

一句话定义这个产品：

```text
hr-cli 是一个借鉴 Lark-CLI 交互和工程框架、基于 MySQL 业务链路封装的 HR 能力网关。
```

它不是数据库脚本集合，也不是简单 SQL 工具。它应该把员工、调动、资料、审批、考勤这些数据库能力封装成可鉴权、可预览、可审计、可被人和 Agent 稳定调用的命令体系。
