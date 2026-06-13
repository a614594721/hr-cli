# hr-cli 方案 v3：借鉴 Lark-CLI 框架

> 本文档在 `hr-cli-product-plan.md` 和 `hr-cli-plan-claude-v2.md` 基础上修订。
> 参考对象：`larksuite/cli`，当前核对到 `HEAD=c0730b46bf315ecd3683165279b2590530725c72`。

## 0. 结论

方案需要调整，但不是推翻。

不变的是业务与安全闭环：

- 调动仍走 `eEmployee_Work` + `eSP_EmpChangeStart`。
- 个人资料仍以 `personal_info` 为写入口。
- 钉钉身份仍只负责证明操作人是谁，HR CLI 自己做权限判断。
- 所有写操作仍必须 `preview` 后 `apply`，且 `apply` 必须重新鉴权、重新读取旧值、重新做权限判断。
- `DB_ENV=test` 下允许按权限执行测试库写操作；`DB_ENV != test` 默认禁止写。

需要调整的是工程方案：

- 从普通业务 CLI 改成 Lark-CLI 风格的三层命令系统。
- CLI 骨架建议从 Python Click 调整为 Go + Cobra。
- 输出、错误、配置、凭证、dry-run、Agent Skill 都作为一等能力设计。
- 低层数据库操作不直接暴露成随意写库命令，而是受策略闸门保护的 API 命令或诊断命令。

## 1. 为什么要借鉴 Lark-CLI

`larksuite/cli` 的价值不在于飞书业务本身，而在于它把 CLI 当成“人和 AI Agent 都能稳定调用的能力网关”来设计。对 HR CLI 更关键的点有：

- 三层命令体系：快捷命令、API 命令、原始调用。
- Agent-native：命令参数、输出、错误、帮助文本都为自动化调用优化。
- `stdout` 放结构化数据，`stderr` 放错误、进度和提示。
- 所有错误有稳定类型、子类型、参数名、恢复建议。
- `--dry-run` / preview 是高风险动作的默认工程能力。
- OS 原生密钥链存储凭证，不把 token、密钥、密码写进项目文件。
- 多 profile、身份选择、严格模式，适合测试/生产隔离。
- commands、shortcuts、internal runtime 分层清晰，便于测试和扩展。

HR CLI 比 Lark-CLI 更高风险，因为它会写人事数据库。所以应该借鉴它的框架风格，但收紧 raw 能力，不能照搬“任意 API 都可调用”的开放度。

## 2. 技术栈调整

### 2.1 推荐技术栈

```text
Go 1.23+
Cobra/pflag                 命令注册、帮助、补全
database/sql + go-sql-driver/mysql
go-keyring 或 Windows Credential Manager wrapper
go-pretty / lipgloss 可选     表格输出
encoding/json               JSON envelope
net/http                    Auth Service 客户端，必要时也可内置轻量 server
```

### 2.2 为什么从 Python 调整为 Go

当前仓库还没有正式 CLI 代码，只有探查脚本和方案文档，此时切技术栈成本低。Go 的收益是：

- 单二进制分发，适合 HR/运维本地使用。
- Cobra 与 Lark-CLI 架构一致，后续借鉴目录和命令注册方式更直接。
- 更容易做统一 exit code、错误 envelope、命令补全、跨平台密钥链。
- 数据库事务和 typed error 可以收在明确的 internal 包里。

保留 Python 的场景：

- 一次性数据库探查脚本。
- 数据迁移或报表脚本。
- 后续如确实需要 FastAPI Auth Service，也可以单独服务化。

V1 不建议 Python 和 Go 混着实现主 CLI，否则 preview/apply、错误协议、测试夹具会分散。

## 3. Lark-CLI 三层命令在 HR CLI 的映射

### 3.1 第一层：快捷命令 Shortcuts

面向 HRBP、HR_ADMIN、普通操作人和 AI Agent 的高频安全命令。命令以 `+` 标识，封装业务校验、权限、preview、审计和友好输出。

```bash
hr auth +login
hr auth +me
hr person +find --name 张三
hr transfer +preview --badge A00123 --dept 1001 --job 2002 --effect-date 2026-06-20 --reason "组织调整"
hr transfer +apply <preview-id>
hr profile +preview --user-id 6094 --set emergency_contact=李四
hr profile +apply <preview-id>
```

设计规则：

- V1 用户文档主要推荐 shortcuts。
- 写操作 shortcuts 必须有 preview/apply 双阶段。
- preview 本质上就是 HR 场景的 `--dry-run`，但要落本地 preview 文件。
- `+apply` 永远不接受一堆字段参数，只接受 preview-id。
- `+apply` 必须支持 `--yes`，无 `--yes` 时二次确认。

### 3.2 第二层：API 命令

面向开发、测试、排障。它们一一对应内部服务或数据库业务动作，但仍受权限、环境和策略控制。

```bash
hr ehr employee get --eid 12345
hr ehr transfer-work create --params preview.json --dry-run
hr ehr emp-change-start call --work-id 456 --operator-eid 12345 --dry-run
hr profile personal-info get --user-id 6094
hr profile personal-info update --user-id 6094 --params changes.json --dry-run
hr perm authorize --action transfer.apply --target-eid 67890 --changes changes.json
```

设计规则：

- 默认只开放读命令和 `--dry-run` 写命令。
- 真实写入 API 命令必须要求 `--enable-low-level-write --yes`，且仅 `DB_ENV=test` 可用。
- API 命令不能绕过审计、权限和 typed error。
- 低层命令的存在是为了测试和诊断，不作为业务人员入口。

### 3.3 第三层：受限 Raw 调用

Lark-CLI 的 raw API 用于覆盖全部开放平台 API。HR CLI 不能提供任意 raw 写库能力。可提供两个受限入口：

```bash
hr db query --sql "select EID,badge,NAME from eemployee where badge=?" --arg A00123
hr service call POST /auth/me --data '{}'
```

设计规则：

- `hr db query` 仅允许 SELECT / SHOW / DESCRIBE / EXPLAIN。
- 禁止 `UPDATE`、`DELETE`、`INSERT`、`CALL`、DDL。
- `hr service call` 只面向 Auth / Permission / Diagnose 服务，不直接调用数据库写入。
- raw 命令默认输出 JSON，适合 Agent 或脚本排障。

## 4. 命令总览

```text
hr
  auth
    +login
    +me
    +logout
    status
  config
    init
    show
    doctor
    strict-mode
  profile
    list
    use
    add
    remove
  person
    +find
    get
  transfer
    +preview
    +apply
    preview show
    preview list
    preview revoke
  profile-info
    +preview
    +apply
    preview show
  ehr
    employee get
    dept get
    job get
    transfer-work create
    emp-change-start call
  perm
    authorize
    roles explain
  db
    query
  doctor
```

说明：

- `profile` 作为 CLI 多配置身份管理，避免和个人资料修改混淆。
- 个人资料修改域改名为 `profile-info`，减少歧义。
- 兼容旧方案命令可做 alias：`hr transfer preview` -> `hr transfer +preview`。

## 5. 输出协议

### 5.1 stdout 是数据

所有成功结果默认输出结构化 envelope。人类表格可通过 `--format table`，Agent 和脚本用 `--format json`。

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
    "db_name": "hrmv9",
    "operator_eid": 12345,
    "created_at": "2026-06-13T21:30:00+08:00"
  }
}
```

### 5.2 stderr 是提示和错误

- 进度、确认提示、二维码提示、风险提示写 stderr。
- 错误写 stderr JSON envelope。
- 不把日志、提示混入 stdout，避免破坏管道和 Agent 解析。

## 6. 错误协议

参考 Lark-CLI typed error contract，为 HR CLI 定义稳定错误类型。

| type | 场景 | exit |
|------|------|------|
| `validation` | 参数错误、字段不允许、preview-id 格式错误 | 2 |
| `authentication` | 未登录、session 过期 | 3 |
| `authorization` | 无 HR 权限、字段级权限不足 | 3 |
| `config` | 缺少配置、DB 环境变量不完整 | 3 |
| `network` | Auth Service / DingTalk 网络失败 | 4 |
| `db` | SQL 执行失败、事务失败、存储过程返回异常 | 5 |
| `policy` | 生产保护、raw 写库被拦截、高敏字段缺少显式模式 | 6 |
| `confirmation` | 高风险操作缺少 `--yes` | 10 |
| `internal` | 未分类 bug、解析失败、协议不一致 | 5 |

错误 envelope：

```json
{
  "ok": false,
  "error": {
    "type": "authorization",
    "subtype": "field_denied",
    "message": "bank_card requires HR_ADMIN sensitive permission",
    "param": "--set bank_card",
    "hint": "remove this field or rerun as HR_ADMIN with --sensitive",
    "action": "profile-info.apply",
    "target_eid": 67890
  }
}
```

要求：

- 业务代码不得最终返回裸 `fmt.Errorf`。
- 每个错误必须能让 Agent 判断下一步：改参数、登录、补权限、重新 preview、停止操作。
- 测试必须断言 `type/subtype/param/hint`，不能只断言 message 文本。

## 7. 配置、profile 与凭证

### 7.1 多 profile

```bash
hr config init
hr profile add test
hr profile use test
hr profile list
hr config show --redact
```

配置文件只保存非密信息：

```json
{
  "current_profile": "test",
  "profiles": {
    "test": {
      "db_env": "test",
      "db_name": "hrmv9",
      "auth_service_url": "http://127.0.0.1:8787",
      "strict_mode": "test_only"
    }
  }
}
```

密钥和 token：

- DB 密码优先来自环境变量 `DB_PASSWORD`。
- HR CLI session token 存 Windows Credential Manager。
- DingTalk app secret 只存在 Auth Service，不进 CLI 配置。
- `config show` 默认脱敏。

### 7.2 strict mode

```text
test_only    仅 DB_ENV=test 允许写
read_only    所有写操作禁止，只允许查询和 preview
prod_locked  生产模式锁定，V1 默认不支持生产写
```

V1 默认 `test_only`。

## 8. Runtime / Factory 分层

借鉴 Lark-CLI 的 `Factory` 思路，每个命令不直接创建外部依赖，而是通过运行时注入。

```text
internal/runtime
  Factory
    Config()
    DB()
    AuthClient()
    PermissionEngine()
    CredentialStore()
    PreviewStore()
    AuditLogger()
    IOStreams
```

收益：

- 命令层只负责参数、输出和流程编排。
- 单元测试可替换 DB、Auth、Permission、Credential。
- preview/apply 的核心流程可做纯逻辑测试。
- 后续替换 Auth Service 或审计存储不影响命令入口。

## 9. 推荐目录结构

```text
D:\projects\hr-cli\
├── go.mod
├── main.go
├── cmd/
│   ├── root.go
│   ├── auth/
│   ├── config/
│   ├── profile/
│   ├── person/
│   ├── transfer/
│   ├── profileinfo/
│   ├── ehr/
│   ├── perm/
│   ├── db/
│   └── doctor/
├── shortcuts/
│   ├── common/
│   ├── auth/
│   ├── person/
│   ├── transfer/
│   └── profileinfo/
├── internal/
│   ├── runtime/
│   ├── config/
│   ├── credential/
│   ├── output/
│   ├── errs/
│   ├── db/
│   ├── auth/
│   ├── perm/
│   ├── preview/
│   ├── audit/
│   ├── ehr/
│   ├── person/
│   ├── transfer/
│   ├── profileinfo/
│   ├── validate/
│   └── redact/
├── skills/
│   ├── hr-shared/
│   ├── hr-transfer/
│   ├── hr-profile-info/
│   └── hr-db-diagnose/
├── tests/
│   ├── cli_e2e/
│   └── fixtures/
├── docs/
└── scripts/
```

## 10. Agent Skills 设计

Lark-CLI 把 skills 作为 Agent 使用 CLI 的关键入口，HR CLI 也应该内置或随仓库提供。

### 10.1 `hr-shared`

内容：

- 永远先确认 `hr auth +me`。
- 写操作必须先 `+preview`，再 `+apply`。
- `DB_ENV != test` 时不要尝试写。
- 不要把 DB 密码、session token、DingTalk secret 写进文件或回复。
- `stdout` JSON 可解析，`stderr` 是错误和提示。

### 10.2 `hr-transfer`

内容：

- 单人调动参数规则。
- 部门、岗位校验顺序。
- `preview_id` 使用方式。
- 常见错误恢复：多人匹配、岗位部门不匹配、旧值变化、无 HRBP 权限。

### 10.3 `hr-profile-info`

内容：

- 定位优先级：`--user-id` > `--phone` > `--name`。
- 禁止用 badge 直接定位 `personal_info`，除非后续验证稳定映射。
- 普通字段和高敏字段矩阵。
- trigger 验证失败时如何解释 partial failure。

### 10.4 `hr-db-diagnose`

内容：

- 只读 SQL 限制。
- 如何用 `hr db query` 排查人员、部门、岗位、preview。
- 禁止 raw 写库、禁止 CALL 存储过程。

## 11. Preview / Apply 在新框架中的位置

Preview 是 shortcut 层的核心产物，同时也是 dry-run 的增强版。

```text
transfer +preview
  参数解析
  operator 解析
  目标人员查询
  业务校验
  权限校验 transfer.preview
  生成 diff
  写 .hr-cli/previews/<id>.json
  stdout 输出 preview envelope
```

```text
transfer +apply <preview-id>
  读取 preview
  检查过期
  operator 重新解析
  目标人员重新读取
  权限重新校验 transfer.apply
  old_values 并发校验
  confirmation 检查
  开事务
  写 eEmployee_Work
  CALL eSP_EmpChangeStart
  验证结果
  写审计
  stdout 输出 apply envelope
```

`profile-info +apply` 同理，但执行路径是 `personal_info` 参数化更新和 trigger 验证。

## 12. 安全策略补强

在 v2 基础上增加这些 Lark-CLI 风格的框架级保护：

- 输出净化：错误、表格和 JSON 中默认脱敏手机号、证件号、银行卡、token。
- 路径校验：`--params @file`、`--output`、preview 文件访问必须限制在工作目录或 HR CLI 数据目录。
- 命令注入保护：不拼接 shell，不把用户输入转为 shell 命令。
- SQL 策略：业务 SQL 使用参数化；`hr db query` 做只读语句解析和关键字拦截。
- confirmation error：缺少 `--yes` 时返回 `confirmation` 类型，而不是让命令半交互卡住 Agent。
- doctor 命令：集中检查 DB_ENV、DB 连接、Auth Service、Credential Manager、skills 版本。

## 13. 测试策略

### 13.1 单元测试

- typed error 构造和 envelope。
- 参数校验和字段权限矩阵。
- preview 生成、过期、旧值变化。
- redaction。
- SQL 只读策略。

### 13.2 dry-run E2E

每个 shortcut 都要有 dry-run 或 preview E2E：

```bash
hr transfer +preview ... --format json
hr profile-info +preview ... --format json
```

断言：

- `ok=true`
- `data.preview_id` 存在
- `meta.db_env=test`
- diff 字段正确
- 没有敏感明文

### 13.3 测试库 live E2E

在 `DB_ENV=test` 下跑可回滚或可清理的真实测试：

- profile-info 单字段更新，验证 trigger。
- transfer 使用专用测试员工，验证存储过程返回和历史表。
- apply 后重复 apply 应失败。
- preview 后旧值变化应失败。

## 14. V1 里程碑

### M1：CLI 框架

- Go module、Cobra root、全局 flag。
- `--format json|table`。
- typed error envelope。
- config/profile/credential 基础。
- doctor。

### M2：只读能力

- DB 连接。
- person find/get。
- dept/job get。
- auth me mock 或接 Auth Service。
- permission explain。

### M3：preview 能力

- transfer +preview。
- profile-info +preview。
- preview store。
- diff 输出和脱敏。

### M4：apply 能力

- transfer +apply 测试库。
- profile-info +apply 测试库。
- 审计 JSONL。
- 并发旧值校验。

### M5：Agent 化

- skills。
- dry-run E2E。
- JSON 输出契约文档。
- 常见错误恢复文档。

## 15. 和 v2 的差异摘要

| 主题 | v2 | v3 |
|------|----|----|
| CLI 技术栈 | Python Click | Go + Cobra |
| 命令模型 | 普通业务命令 | Shortcuts / API commands / restricted raw |
| 输出 | Rich + 文本为主 | stdout JSON envelope，table 可选 |
| 错误 | 未定义统一协议 | typed error + exit code |
| Agent 支持 | 文档层面 | skills + 结构化输出 + 可恢复错误 |
| raw 能力 | 未设计 | 只读 DB query + service call |
| 配置 | pydantic-settings | profile + strict mode + keychain |
| 测试 | 功能测试为主 | unit + dry-run E2E + live test |

## 16. 最终建议

采用 v3 作为后续实现基线。

具体执行策略：

1. 不改业务结论，继续沿用 v2 已核实的数据库链路。
2. 新建 Go/Cobra CLI 骨架，先实现 root/config/profile/output/errs/doctor。
3. 先做只读查询和 preview，不急于 apply。
4. 每个 shortcut 从第一天就支持 JSON envelope 和 typed error。
5. 低层写库能力只作为受控 API 命令存在，默认 `--dry-run`，不作为用户主入口。

这样既保留 HR 写操作的安全闭环，又吸收 Lark-CLI 在 Agent 友好、可测试、可分发、可恢复错误方面的成熟框架。
