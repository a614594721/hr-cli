---
name: hr-shared
version: 1.0.0
description: "Use when first setting up hr-cli, running auth +login, hitting permission/scope errors, doing write operations (transfer/profile-info +apply), or when uncertain which command modifies data vs. previews. Read this BEFORE any other hr-* skill."
metadata:
  requires:
    bins: ["hr"]
  cliHelp: "hr --help"
---

# hr-cli 共享规则

本技能定义 hr-cli 通用规则:profile 配置、认证、错误结构、写操作两阶段安全模式、scope/perm 决策。**任何 hr-* 子技能都假设你已经读过本文件**。

## 架构前置

hr-cli 是**瘦客户端**,自己不持有 DB 凭证,所有 DB 操作经过 hr-gateway。本机看不到 SQL,业务正确性靠 gateway 的 perm + scope + audit 保证。

```
hr-cli (本机)  ──HTTPS──►  hr-gateway  ──MySQL──►  HR DB
```

如果用户问"为什么不直接连 DB"或"能不能跳过 gateway",答案是不能 — 这是架构边界,绕过等于丢失审计 + 角色检查 + scope 过滤。

## Profile 配置

首次使用必须先配 profile。profile 唯一必填字段是 `--auth-base-url`(gateway URL)。

```bash
hr profile add <name> --auth-base-url http://<gateway-host>:<port>
hr profile use <name>
```

profile 之间相互独立(测试 / 生产),`profile use` 切换当前活动 profile。token 也按 profile 隔离存储(操作系统 keychain)。

## 认证

**唯一支持的登录方式是钉钉 OAuth**。

```bash
hr auth +login --dingtalk
```

会做的事:
1. 调 gateway `/auth/login/start` 拿 `loginId / loginSecret / authUrl`
2. 自动打开浏览器到 `authUrl`(钉钉授权页)
3. 用户在浏览器完成授权后,gateway callback 落库
4. CLI 长轮询 `/auth/login/poll` 拿到 access_token + refresh_token

**Agent 规则**:`hr auth +login --dingtalk` 会**阻塞等待用户完成浏览器授权**(默认 180s 超时)。如果你是 agent 在 background 跑命令,提前告诉用户"浏览器会自动打开,请扫码同意"。

```bash
hr auth +me        # 验证当前会话
hr auth status     # 查 token 有效期
hr auth +logout    # 撤销 refresh token
```

token 自动刷新由 hr-cli 处理,无需手动 refresh。

### 切换身份(仅 HR_ADMIN 可用)

```bash
hr auth impersonate --eid <EID>
```

签发 15 分钟 TTL 的短 token,**强制写审计**。用于以指定身份测试 perm/scope 边界。HR_ADMIN 之外的角色调用会被 gateway 拒绝。

## 错误 envelope

所有响应统一 envelope:

```json
// 成功
{"ok": true, "data": {...}, "meta": {"command": "..."}}

// 失败
{"ok": false, "error": {
  "type": "authentication|authorization|validation|db|network|policy|confirmation|config|internal",
  "subtype": "<machine-readable code>",
  "message": "<human-readable text>",
  "param": "<offending field, optional>",
  "hint":  "<recovery suggestion, optional>"
}}
```

**Agent 规则**:错误分发**只看 `subtype`,不要 grep `message`**。`message` 文案可能脱敏(DB host:port 不会暴露给客户端),`subtype` 是稳定契约。

常见 subtype:

| subtype | 含义 | 处理 |
|---|---|---|
| `token_missing` | 没带 Bearer | `auth +login` |
| `token_expired` | access token 过期 | hr-cli 自动 refresh,通常看不到 |
| `action_denied` | 当前角色无权该 action | `perm explain --action <name>` |
| `target_out_of_scope` | 角色有 action 但 target EID 不在范围 | 切上级角色或换目标 |
| `field_denied` | profile-info `--set` 字段不在白名单 | 改字段或加 `--sensitive`(仅 HR_ADMIN) |
| `confirm_header_required` | `+apply --yes` 缺 X-HR-Confirm | hr-cli 自动加 |
| `not_logged_in` | 本机无活动 session | `auth +login --dingtalk` |
| `connect_failed` | gateway 或 DB 不可达 | 检查 gateway 是否在跑 |

## 角色 + Scope 模型

角色优先级(高→低):`HR_ADMIN > SSC > HRBP > MANAGER > SELF`。

| 角色 | scope 范围 |
|---|---|
| HR_ADMIN / SSC | 全员旁路 — 不做 target scope check |
| HRBP / MANAGER | 限本人辐射范围(`psoradiationrangeeidlist`) |
| SELF | 仅本人 |

如果你不确定某个 action + targetEID 是否能通过,**先调 `hr perm explain`**:

```bash
hr perm explain --action transfer.preview --target-eid 94
```

返回包含 `decision: allow|deny`、`reason`、`target_scope`(如果传了 target-eid)。

## 写操作两阶段模式

`transfer / profile-info` 这类**修改库的命令**强制走 preview → apply 两阶段:

```bash
# 阶段 1:dry-run,生成 preview_id
hr transfer +preview --badge P000487 --dept 107 --job 58 --effect-date 2026-07-01 --reason "..."

# 阶段 2:用 preview_id 真正写
hr transfer +apply <preview_id> --yes
```

**Agent 必读规则**:

1. **永远先 +preview 再 +apply**。preview 返回 `plan.changes: [{field, old, new}, ...]`,要让用户**逐字段确认**再 apply。
2. `--yes` 不能省。与服务端 `X-HR-Confirm: yes` header 配对(hr-cli 自动加 header)。
3. preview_id 一次性,apply 后失效。
4. **never `+apply` 写动作而不向用户确认**。即使用户说"直接执行",也要列出 plan changes 让其复核。
5. 测试库(`DB_ENV=test`)允许 no-op 写(old==new)直接 apply 不要求确认;生产环境一律两阶段。

## 测试员工

不要用真实在职员工做写测试。固定的安全测试目标:

- 操作者: **吴邦** (badge `P000487`, EID `94`, role `HR_ADMIN`)
- profile-info no-op 写测试: **袁洁** (`personal_info.user_id=6711`)

吴邦在 `personal_info` 无对应行,**不能用吴邦本人测 profile-info**。

## 命令两类语法

```bash
hr <domain> +<action>     # 写动作或副作用动作 — 加号前缀
hr <domain> <action>      # 只读动作
```

例:`employee +find`(搜索)用加号,`employee get`(按 EID 取详情)不加。

## 输出格式

默认 JSON,所有命令支持 `--format table`。Agent 应保持默认 JSON 解析。

## 读完后

继续读具体 capability 的 SKILL.md:

- [`hr-employee`](../hr-employee/SKILL.md) — 员工查询
- [`hr-attendance`](../hr-attendance/SKILL.md) — 考勤
- [`hr-approval`](../hr-approval/SKILL.md) — 审批(1.0 仅查询)
- [`hr-transfer`](../hr-transfer/SKILL.md) — 调动 (preview/apply)
- [`hr-profile-info`](../hr-profile-info/SKILL.md) — 个人资料 (preview/apply)
- [`hr-perm`](../hr-perm/SKILL.md) — 权限解释
