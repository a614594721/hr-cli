# hr-cli

> 面向 HR 运维、HRBP 和 AI Agent 的人力命令行工具,通过 hr-gateway 安全访问公司 HR 数据。

`hr-cli` 是一个**纯 HTTP 瘦客户端**,本身不持有任何数据库凭证、不实现权限决策、不写审计。所有数据库连接、perm/scope 决策、审计写入都发生在 hr-gateway 服务端;客户端只持有钉钉 OAuth 颁发的短期 access_token,通过 HTTPS 调用 gateway。

[安装](#安装) · [项目背景](#项目背景与愿景) · [快速开始](#快速开始-用户) · [AI Agent 快速开始](#快速开始-ai-agent) · [命令体系](#命令体系) · [输出协议](#输出协议) · [安全](#安全与边界) · [架构](#架构)

## 项目背景与愿景

项目发起背景是公司正在系统性推进 AI 工具在业务中的落地。作为金融互联网公司的 HRIS,当前可以在工作中充分使用各类顶级 AI 模型,公司层面也在人力、工具和氛围上持续投入。HR 部门管理层同样支持 AI 推广:在公司 AI 大赛之后,人力部门内部也组织了多轮 AI 使用培训和部门 AI 大赛,目标是让 AI 真正进入工作流程,为日常 HR 业务提效。

`hr-cli` 正是在这个背景下产生的项目。它不是单纯做一个命令行工具,而是希望把人力系统中原本需要登录系统、跨页面操作、人工核验的能力,封装成一组安全、可审计、适合 AI Agent 调用的业务入口。最终用户可以通过自然语言对话,例如和 Codex 这样的 AI Agent 对话,完成原本需要在人力系统中手动执行的查询、预览和操作。

典型目标场景包括:

- HRBP 无需登录人力系统,通过自然语言完成员工调动预览与执行,例如把员工从 A 部门调到 B 部门。
- HRBP 快速更新人员信息,例如将某员工的直接上级从张三调整为李四。
- SSC 快速查询员工当天打卡记录,并筛选存在打卡异常的人员。
- 部门负责人查询权限范围内员工的学历、工作履历、历史绩效等信息。
- 员工通过对话快速发起请假、加班等流程。
- 直接上级查询当前待审批流程,整体预览后进行快捷审批。
- 后续持续扩展更多 HR 业务场景,让人力服务更贴近自然语言工作流。

近期目标是把 `hr-cli` 打造成一个可参赛、可演示、可真实落地的人力 AI 赋能项目:先覆盖更多 HR 场景,再沉淀权限、审计、预览、脱敏和 Agent 调用协议。长期愿景则不止于 HR CLI,而是逐步扩展为企业级 CLI / Agent Gateway,覆盖公司更多中后台系统,让员工、管理者和职能团队都能通过自然语言安全地调用企业能力。

## 为什么用 hr-cli

- **Agent-Native 设计** — 7 份结构化 [Skills](skills/) 开箱即用,Claude / Cursor 等 AI Agent 零额外配置可上手。
- **零凭证**:用户机器上没有 `DB_PASSWORD`、没有数据库主机、没有任何业务 SQL。
- **AI Agent 友好**:命令、参数、输出、错误全部结构化,适合自然语言驱动。
- **权限不可绕过**:鉴权、scope、字段权限、生产保护全部在 gateway 服务端执行,客户端无法跳过。
- **可审计**:所有写操作在服务端写双份审计(本机 JSONL + DB);客户端无审计权限。
- **高风险双阶段**:`transfer` 和 `profile-info` 强制 preview → apply 两步走,apply 必须 `--yes` + `X-HR-Confirm` 头。

## 安装

### 前置要求

- Node.js 16+(npm)
- 公司内网可达的 hr-gateway 实例

### 一键安装(推荐)

```bash
npm install -g @wubang9527/hr-cli
hr-cli --version
```

`postinstall` 会从 GitHub Releases 下载对应平台(darwin/linux/windows × amd64/arm64)的预编译二进制并校验 SHA256。装包后 `hr-cli` 在 PATH 中。

### 直接下载二进制

到 [Releases](https://github.com/a614594721/hr-cli/releases) 下载对应平台压缩包,解压后把 `hr-cli` 放到 PATH。

### 源码构建

```bash
git clone https://github.com/a614594721/hr-cli.git
cd hr-cli
go build -o hr-cli .
./hr-cli --version
```

依赖 Go 1.26+。

## 快速开始 (用户)

> **AI Agent 看这里**:跳到 [快速开始 (AI Agent)](#快速开始-ai-agent),那里写好了所有 Agent 需要的步骤。

2 步上手:

```bash
# 1. 钉钉 OAuth 登录(浏览器跳转)
hr auth +login --dingtalk

# 2. 验证 + 用起来
hr auth +me
hr employee +find --badge P000487
```

`auth +login --dingtalk` 会打开默认浏览器到钉钉授权页;授权完成后 access_token / refresh_token 自动写入 OS 安全凭证存储,session 元数据写到 `.hr-cli/session.json`(不含敏感字段)。

后续命令自动用 access_token,过期前 5 分钟自动刷新。

## 快速开始 (AI Agent)

> 给协助用户的 AI Agent。**hr-cli 二进制内嵌了所有 skills**,装完包后用 `hr-cli skills` 直接读取,不需要克隆仓库或浏览 gitee。**先读 hr-shared** —— 它定义了认证、错误 envelope、scope 模型、写操作两阶段安全模式;每个 capability skill 都假设你已经读过 hr-shared。

```bash
hr-cli skills list                  # 列出所有 skill 及 description
hr-cli skills read hr-shared        # 读 hr-shared SKILL.md (必读)
hr-cli skills read hr-employee      # 读员工查询 skill
```

`skills read` 默认输出 JSON envelope (`data.content` 是 markdown 原文),Agent 直接解析;人工查可加 `--format table` 拿 raw markdown。

### Skills 索引

每个业务域一份 SKILL.md,Claude Skill 风格的 frontmatter + 命令骨架 + 输出契约 + Agent 规则:

| Skill | 用途 |
|---|---|
| [`hr-shared`](skills/hr-shared/SKILL.md) | **必读**。profile/auth/error/scope/写两阶段 |
| [`hr-employee`](skills/hr-employee/SKILL.md) | 员工查询(find / get) |
| [`hr-attendance`](skills/hr-attendance/SKILL.md) | 考勤查询(records / summary / exceptions) |
| [`hr-approval`](skills/hr-approval/SKILL.md) | 审批查询(1.0 仅查,不批) |
| [`hr-transfer`](skills/hr-transfer/SKILL.md) | 调动 preview/apply |
| [`hr-profile-info`](skills/hr-profile-info/SKILL.md) | 个人资料 preview/apply |
| [`hr-perm`](skills/hr-perm/SKILL.md) | 权限解释 / 写动作 dry-check |

### 安装与首次登录(每步只一条命令,顺序执行)

**Step 1 — 安装**

```bash
npm install -g @wubang9527/hr-cli
```

**Step 2 — 钉钉 OAuth 登录**

> 这一步必须在后台运行,因为命令会一直等浏览器授权完成。提取输出中的 `auth_url` 字段发给用户,告诉用户在浏览器中完成授权。

```bash
hr auth +login --dingtalk --no-wait
```

输出中的 `login_id` 字段记下。用户授权完成后,继续:

```bash
hr auth +login --dingtalk --login-id <login_id>
```

**Step 3 — 验证**

```bash
hr auth +me
```

返回 `{"data": {"eid": "...", "name": "...", "role": "..."}}` 即成功。

后续业务命令(`employee +find`、`attendance +records` 等)直接调用即可,access_token 自动注入。**遇到 `authorization/action_denied` 或 `target_out_of_scope`**,先用 `hr perm explain --action <name> --target-eid <X>` 诊断。

## 命令体系

```text
hr <domain> +<shortcut>     # 业务快捷命令(human + AI 友好)
hr auth        # 登录、身份、登出、权限解释
hr employee    # 员工查询
hr attendance  # 考勤
hr approval    # 审批查询(写操作 1.0 仅 dry-run)
hr transfer    # 调动 preview / apply
hr profile-info # 个人资料 preview / apply
hr perm        # 权限解释
hr doctor      # gateway 连通性检查
hr profile     # 本地 profile 管理
hr config      # 本地 config 初始化
```

详细命令矩阵见 [`docs/command-contract.md`](docs/command-contract.md)。

### 示例:员工查询

```bash
hr employee +find --name 张三 --format table
hr employee +find --badge A00123 --format json
hr employee get --eid 12345
```

### 示例:调动 preview / apply

```bash
hr transfer +preview \
  --badge A00123 \
  --dept 1001 \
  --job 2002 \
  --effect-date 2026-06-20 \
  --reason "组织调整"

hr transfer +apply 20260613-213000-abcdef --yes
```

### 示例:考勤查询

```bash
hr attendance +records --badge A00123 --from 2026-06-01 --to 2026-06-13
hr attendance +summary --dept 1001 --date 2026-06-13
hr attendance +exceptions --dept 1001 --from 2026-06-01 --to 2026-06-13
```

### 示例:审批查询

```bash
hr approval +tasks --assignee me
hr approval +task --task-id 10086
hr approval +instances --employee 12345 --status pending
```

## 输出协议

### 成功 envelope

```json
{
  "ok": true,
  "data": {
    "preview_id": "20260613-213000-abcdef",
    "target": { "eid": 67890, "badge": "B00999", "name": "李四" },
    "changes": [{ "field": "DPID", "old": 100, "new": 200 }]
  },
  "meta": { "command": "transfer.+preview" }
}
```

### 错误 envelope

```json
{
  "ok": false,
  "error": {
    "type": "authorization",
    "subtype": "target_out_of_scope",
    "message": "eid 94 not in HRBP scope",
    "param": "--badge",
    "hint": "use --eid or contact HR_ADMIN"
  }
}
```

### 错误类型

| type | 触发场景 | 退出码 |
|---|---|---|
| `validation` | 参数错误、字段不允许 | 2 |
| `config` | 缺少配置(如 `auth_base_url` 未设置) | 2 |
| `authentication` | 未登录、token 过期、refresh 失败 | 3 |
| `authorization` | 无操作权限、scope 越权、字段权限不足 | 3 |
| `policy` | 生产保护、raw 写库拦截、高敏字段缺少显式模式 | 3 |
| `confirmation` | 高风险操作缺少 `--yes` | 3 |
| `network` | gateway 不可达、超时 | 4 |
| `db` | gateway 报告的数据库错误(只读穿透) | 4 |
| `internal` | 协议不一致、未分类 bug | 5 |

约定:

- `stdout` 只放数据,`stderr` 放进度、提示、确认和错误。
- `--format json` 给 Agent 和脚本。
- `--format table` 给人工查看。
- 敏感字段默认脱敏。

## 安全与边界

`hr-cli` 不持有数据库凭证。所有安全约束在 gateway 服务端实施:

- DB 凭证不出 gateway 服务器。
- 客户端持有的 access_token TTL ≤ 30 分钟,refresh_token 服务端可撤销。
- 操作者身份(`eid` / `role`)由 gateway 在 token 中签名,客户端无法伪造或修改。
- 写操作必须 `X-HR-Confirm: yes` + 服务端 perm/scope 双闸 + 服务端审计。
- 用户机器丢失或被入侵,损失上限 = TTL 内的事;refresh_token 撤销后立即失效。

不持有凭证带来的好处:

- AI Agent 哪怕被 prompt injection 也无法越权操作 —— gateway 不认 token 即拒绝。
- 客户端泄漏 session 文件不等于泄漏数据库密码。
- perm/scope 升级是 gateway 发版,无需让所有用户更新 hr-cli。

## 架构

```
┌────────────────────────────┐         ┌────────────────────────────────┐
│  用户机器 (人 / AI / CI)     │  HTTPS  │  hr-gateway (内网)              │
│                            │ ──────► │                                │
│  hr-cli (瘦客户端)            │         │  - 鉴权(JWT)                  │
│  - cobra 命令解析            │         │  - perm + scope 决策           │
│  - HTTP client + token       │         │  - audit 双写                  │
│  - access_token (keychain)   │         │  - capability 实现              │
│                            │         │  - DB 连接 (凭证仅此持有)        │
└────────────────────────────┘         └─────────────┬──────────────────┘
                                                    ▼
                                              MySQL (HR DB)
```

详细设计见 [`docs/hr-cli-architecture-credential-isolation.md`](docs/hr-cli-architecture-credential-isolation.md)。

相关文档:
- [钉钉 OAuth 登录](docs/dingtalk-oauth-auth.md)
- [命令契约](docs/command-contract.md)
- [错误契约](docs/error-contract.md)
- [npm 发布计划](docs/npm-publish-plan.md)

## 许可证

[MIT License](LICENSE)
