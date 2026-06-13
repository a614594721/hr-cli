# hr-cli

`hr-cli` 是一个面向 HR 运维、HRBP 和 AI Agent 的命令行工具。它借鉴 [Lark-CLI](https://github.com/larksuite/cli) 的工程框架和交互设计，把现有人力系统 MySQL 数据库中的员工、调动、个人资料、审批、考勤能力封装成可鉴权、可预览、可审计、可稳定自动化调用的 CLI。

当前项目已提供 Go/Cobra V1a 可运行实现，详细落地方案见：

- [v4 方案：DB-backed HR 能力网关](docs/hr-cli-plan-lark-v4.md)
- [数据库能力盘点](docs/db-capability-inventory.md)
- [命令契约](docs/command-contract.md)
- [错误契约](docs/error-contract.md)
- [调动 apply 安全计划](docs/transfer-apply-safety.md)

[背景](#项目背景与愿景) · [定位](#为什么做-hr-cli) · [能力](#功能范围) · [快速开始](#安装与快速开始) · [命令体系](#三层命令体系) · [鉴权](#鉴权与权限) · [安全](#安全与风险提示) · [开发](#开发计划)

## 项目背景与愿景

项目发起背景是公司正在系统性推进 AI 工具在业务中的落地。作为金融互联网公司的 HRIS，当前可以在工作中充分使用各类顶级 AI 模型，公司层面也在人力、工具和氛围上持续投入。HR 部门管理层同样支持 AI 推广：在公司 AI 大赛之后，人力部门内部也组织了多轮 AI 使用培训和部门 AI 大赛，目标是让 AI 真正进入工作流程，为日常 HR 业务提效。

`hr-cli` 正是在这个背景下产生的项目。它不是单纯做一个命令行工具，而是希望把人力系统中原本需要登录系统、跨页面操作、人工核验的能力，封装成一组安全、可审计、适合 AI Agent 调用的业务入口。最终用户可以通过自然语言对话，例如和 Codex 这样的 AI Agent 对话，完成原本需要在人力系统中手动执行的查询、预览和操作。

典型目标场景包括：

- HRBP 无需登录人力系统，通过自然语言完成员工调动预览与执行，例如把员工从 A 部门调到 B 部门。
- HRBP 快速更新人员信息，例如将某员工的直接上级从张三调整为李四。
- SSC 快速查询员工当天打卡记录，并筛选存在打卡异常的人员。
- 部门负责人查询权限范围内员工的学历、工作履历、历史绩效等信息。
- 员工通过对话快速发起请假、加班等流程。
- 直接上级查询当前待审批流程，整体预览后进行快捷审批。
- 后续持续扩展更多 HR 业务场景，让人力服务更贴近自然语言工作流。

近期目标是把 `hr-cli` 打造成一个可参赛、可演示、可真实落地的人力 AI 赋能项目：先覆盖更多 HR 场景，再沉淀权限、审计、预览、脱敏和 Agent 调用协议。长期愿景则不止于 HR CLI，而是逐步扩展为企业级 CLI / Agent Gateway，覆盖公司更多中后台系统，让员工、管理者和职能团队都能通过自然语言安全地调用企业能力。

## 为什么做 hr-cli

很多 HR 系统的核心能力只存在于数据库、存储过程、trigger 和审批状态机里，没有可直接调用的开放 API。直接写 SQL 脚本虽然快，但会带来权限绕过、审计缺失、状态不一致和 Agent 误操作风险。

`hr-cli` 的目标是把这些数据库能力收敛成一个受控入口：

- **面向人和 Agent**：命令、参数、输出、错误都适合人读，也适合自动化解析。
- **DB-backed 能力封装**：不把业务 SQL 散落在命令里，而是封装成稳定的 Capability。
- **鉴权优先**：用户鉴权通过后，只能操作自己有权限的员工、字段和审批任务。
- **高风险双阶段**：调动、资料修改、审批操作默认先 preview / dry-run，再 apply。
- **结构化输出**：stdout 输出 JSON envelope 或表格，stderr 输出提示和错误。
- **可审计**：成功、失败、拒绝、高风险确认都进入审计链路。
- **生产保护**：V1 默认只允许 `DB_ENV=test` 执行写操作，生产写入另行设计强确认机制。

## 功能范围

| 业务域 | 能力 | V1 策略 |
|--------|------|---------|
| `auth` | 登录、身份映射、当前操作者、权限解释 | 优先实现，所有业务命令前置依赖 |
| `employee` | 员工查询、员工快照、可操作动作摘要 | V1 必做，默认只读 |
| `transfer` | 人员调动 preview / apply | V1 做单人调动，走系统原生链路 |
| `profile-info` | 个人资料修改 preview / apply | V1 做白名单字段，高敏字段单独闸门，测试环境可执行 |
| `approval` | 审批任务查询、审批详情、同意/拒绝/转交 | V1 先查询，写操作等状态机核实后实现 |
| `attendance` | 打卡记录、考勤汇总、异常查询 | V1 只读 |

已确认的核心业务方向：

- 调动不裸改 `eemployee`，优先走 `eEmployee_Work` + `eSP_EmpChangeStart`。
- 个人资料修改以 `personal_info` 为入口，复用现有 trigger 同步逻辑。
- 钉钉或企业身份只证明“操作者是谁”，HR CLI 自己判断“能不能操作”。
- 姓名、手机号、邮箱只能用于查询和展示，不能作为自动鉴权主键。

## 安装与快速开始

当前仓库已提供 V1a 可运行实现。

V1a 运行依赖：

- Go 1.26+
- Cobra / pflag
- go-sql-driver/mysql

构建：

```bash
go mod tidy
go build -o hr.exe .
```

运行：

```bash
.\hr.exe doctor
.\hr.exe auth +me
.\hr.exe employee +find --badge A00123
```

目标安装形态：

```bash
# 源码构建，后续补充 Makefile
git clone <repo-url>
cd hr-cli
go build -o hr.exe .
```

首次配置目标流程：

```bash
hr config init
hr profile add test --db-env test --db-host <host> --db-name <database> --db-user <user> --credential-target <credential-name>
hr profile use test
hr credential status
hr auth +login
hr auth +me
hr doctor
```

数据库连接优先读取环境变量：

```text
DB_ENV
DB_HOST
DB_PORT
DB_USER
DB_PASSWORD
DB_NAME
```

`DB_ENV=test` 视为测试环境，可按权限执行测试库写操作。`DB_ENV != test` 时 V1 默认禁止写操作。

## 快速示例

查询员工：

```bash
hr employee +find --name 张三 --format table
hr employee +find --badge A00123 --format json
hr employee get --eid 12345
```

预览人员调动：

```bash
hr transfer +preview \
  --badge A00123 \
  --dept 1001 \
  --job 2002 \
  --effect-date 2026-06-20 \
  --reason "组织调整"
```

执行已确认的调动：

```bash
hr transfer +apply 20260613-213000-abcdef --yes
```

预览个人资料修改：

```bash
hr profile-info +preview \
  --user-id 6094 \
  --set emergency_contact=李四 \
  --set emergency_phone=13900000000

hr profile-info +apply <preview-id> --yes
```

查询审批任务：

```bash
hr approval +tasks --assignee me
hr approval +task --task-id 10086
hr approval +instances --employee 12345 --status pending
hr approval +approve --task-id 10086 --comment "同意" --dry-run
```

查询打卡记录：

```bash
hr attendance +records --badge A00123 --from 2026-06-01 --to 2026-06-13
hr attendance +summary --dept 1001 --date 2026-06-13
```

## 三层命令体系

`hr-cli` 借鉴 Lark-CLI 的三层调用思想，但由于底层没有原生 OpenAPI，第二层改为数据库能力层。

```text
Shortcuts 快捷业务命令
  -> DB Capability Commands 数据库能力命令
  -> Restricted Raw Diagnostics 受限原始诊断
```

### 1. Shortcuts

快捷命令面向日常使用和 AI Agent，使用 `+` 前缀，内置智能默认值、权限检查、preview、审计和友好输出。

```bash
hr employee +find --name 张三
hr transfer +preview --badge A00123 --dept 1001 --job 2002
hr profile-info +preview --user-id 6094 --set address="上海市..."
hr approval +tasks --assignee me
hr attendance +records --badge A00123 --from 2026-06-01 --to 2026-06-13
```

### 2. DB Capability Commands

数据库能力命令面向开发、测试和排障。它们封装员工、调动、资料、审批、考勤的底层表、存储过程和状态机，但仍受权限、环境和审计约束。

```bash
hr ehr employee get --eid 12345
hr ehr transfer-work create --params preview.json --dry-run
hr perm explain --action transfer.apply --target-eid 67890
hr approval task get --task-id 10086
```

真实写入类能力默认要求 `--dry-run`。如后续开放低层写入，也必须要求显式开关、`--yes` 和测试环境保护。

### 3. Restricted Raw Diagnostics

原始诊断入口只用于只读排查。

```bash
hr db query --sql "select EID,badge,NAME from eemployee where badge=?" --arg A00123
```

规则：

- 只允许 `SELECT`、`SHOW`、`DESCRIBE`、`EXPLAIN`。
- 禁止 `INSERT`、`UPDATE`、`DELETE`、`CALL`、DDL。
- raw 命令默认输出 JSON。
- raw 命令不能绕过鉴权、脱敏和审计策略。

## 鉴权与权限

目标命令：

```bash
hr auth +login
hr auth +me
hr auth +logout
hr auth status
hr perm explain --action transfer.apply --target-eid 12345
```

权限模型分三层：

| 层级 | 示例 |
|------|------|
| 动作权限 | `employee.get`、`transfer.apply`、`approval.task.approve` |
| 目标范围 | 本人、负责员工、下属、全部员工、本人审批待办 |
| 字段权限 | 普通个人资料字段、高敏字段、调动字段 |

角色初版：

| 角色 | 能力边界 |
|------|----------|
| `SELF` | 查自己，修改自己的部分普通资料，查自己的打卡 |
| `HRBP` | 操作自己负责的员工，不能改高敏字段 |
| `MANAGER` | V1 以查询为主，写操作暂不开放 |
| `HR_ADMIN` | 可操作全部员工，高敏字段仍需显式模式和审计 |

审批权限不能只靠 HRBP 或 ADMIN 推断，还必须结合审批任务当前处理人、节点权限和流程定义。

## 输出协议

默认成功输出为 JSON envelope：

```json
{
  "ok": true,
  "data": {
    "preview_id": "20260613-213000-abcdef",
    "target": {
      "eid": 67890,
      "badge": "B00999",
      "name": "李四"
    },
    "changes": [
      {
        "field": "DPID",
        "old": 100,
        "new": 200
      }
    ]
  },
  "meta": {
    "command": "transfer.+preview",
    "db_env": "test",
    "db_name": "hrmv9"
  }
}
```

约定：

- stdout 只放数据。
- stderr 放进度、提示、确认和错误。
- `--format json` 给 Agent 和脚本使用。
- `--format table` 给人工查看使用。
- 敏感字段默认脱敏。

## 错误协议

错误也使用稳定 envelope，便于 Agent 判断下一步动作。

```json
{
  "ok": false,
  "error": {
    "type": "authorization",
    "subtype": "field_denied",
    "message": "bank_card requires HR_ADMIN sensitive permission",
    "param": "--set bank_card",
    "hint": "remove this field or rerun as HR_ADMIN with --sensitive"
  }
}
```

错误类型：

| type | 场景 |
|------|------|
| `validation` | 参数错误、字段不允许、preview-id 格式错误 |
| `authentication` | 未登录、session 过期 |
| `authorization` | 无 HR 权限、字段级权限不足 |
| `config` | 缺少配置、DB 环境变量不完整 |
| `network` | Auth Service 或身份服务网络失败 |
| `db` | SQL、事务或存储过程失败 |
| `policy` | 生产保护、raw 写库拦截、高敏字段缺少显式模式 |
| `confirmation` | 高风险操作缺少 `--yes` |
| `internal` | 未分类 bug 或协议不一致 |

## Agent Skills

计划提供以下 skills，让 AI Agent 能按固定规则调用 CLI：

| Skill | 用途 |
|-------|------|
| `hr-shared` | 登录、profile、权限、安全和输出规则 |
| `hr-employee` | 员工查询、多人匹配处理、身份映射说明 |
| `hr-transfer` | 调动 preview/apply 流程和常见错误恢复 |
| `hr-profile-info` | 资料字段白名单、高敏字段、trigger 验证 |
| `hr-approval` | 审批任务查询、审批操作前置核实 |
| `hr-attendance` | 打卡记录和考勤汇总查询 |
| `hr-db-diagnose` | 只读 SQL 排查规则 |

Agent 调用原则：

- 写操作必须先 preview 或 dry-run。
- `apply` 前必须重新鉴权。
- 多人匹配必须停止，不能猜。
- 不要输出、保存或提交 DB 密码、session token、DingTalk secret。
- `DB_ENV != test` 时不要尝试写操作。

## 安全与风险提示

使用前请确认：

- 不在代码、文档、日志中保存数据库密码、session token 或企业应用 secret。
- 所有业务 SQL 使用参数化。
- 只读 raw query 不允许 `CALL` 存储过程。
- 高风险操作必须写审计日志。
- 调动必须优先走系统原生链路，不能手写多表同步。
- 审批写操作必须先核实状态机，不能只改任务状态字段。
- 打卡查询默认按权限范围过滤，输出脱敏。

当前最核心的安全挑战有两类：

1. **权限边界与数据可见性**：任何查询和写入都不能绕过人力系统原有权限体系。非授权人员不能看到权限范围外的数据，更不能通过自然语言让 Agent 间接查询到全员薪酬、高敏个人信息、绩效等敏感数据。CLI 必须在动作权限、目标范围、字段权限、环境保护和审计链路上形成硬约束，而不是依赖提示词约束。
2. **AI 链路的数据安全**：HR 数据在进入 AI 工作流时，可能经过公司 AI 中转站、模型供应商、日志系统和 Agent 运行环境。项目需要明确哪些数据可以发送给模型，哪些字段必须脱敏、摘要化或禁止外发；同时要避免把数据库连接、token、员工敏感信息写入 prompt、日志、缓存、训练数据或第三方平台。

因此，`hr-cli` 的安全原则是：AI 可以理解意图、编排步骤和生成参数，但真正的数据访问、权限判断、敏感字段处理、写操作确认和审计必须由 CLI 侧的确定性规则执行。

## 项目结构规划

```text
hr-cli/
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
├── internal/
│   ├── capability/
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
├── docs/
└── tests/
```

## 开发计划

### M0：数据库能力盘点

- 输出 `docs/db-capability-inventory.md`。
- 核实员工、调动、资料、审批、考勤相关表和存储过程。
- 对审批写操作给出是否可安全落地的结论。

### M1：CLI 框架

- Go module、Cobra root、全局 flag。
- config / profile / credential。
- JSON envelope。
- typed error。
- doctor。

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

## 开发约定

- 命令层不直接拼业务 SQL，必须调用 Capability。
- stdout 是数据，stderr 是提示和错误。
- 新命令必须有 typed error。
- 新写操作必须有 preview 或 dry-run。
- 新业务域必须补 Agent skill。
- 测试优先覆盖权限、脱敏、旧值变化、生产保护和错误 envelope。

## 许可证

本项目采用 [MIT License](LICENSE) 开源协议。
