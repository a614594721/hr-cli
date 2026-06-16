---
name: hr-approval
version: 1.0.0
description: "HR 审批查询 — 查待办任务、审批实例详情。1.0 不支持 approve/reject/transfer 写动作(server 主动拒绝),写动作请引导用户去钉钉 App 操作。"
metadata:
  requires:
    bins: ["hr"]
  cliHelp: "hr approval --help"
---

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../hr-shared/SKILL.md`](../hr-shared/SKILL.md)。**

## 1.0 范围限制

**写动作(approve / reject / transfer)在 1.0 不可用**。审批状态机未在 hr-gateway 验证,server 端会返回 `policy/approval_write_not_verified`。

如果用户问"为什么我用 hr-cli 批不动审批",回答:

> hr-cli 1.0 只覆盖审批查询。批准/拒绝/转交请去钉钉 App 走原生审批流。后续版本会接入。

不要尝试用 dry-run 绕过,也不要试图直接写 `skywftask` / `skywfinstance` — 这等于绕过 audit + 状态机,严禁。

## 选哪个命令

| 想做什么 | 命令 |
|---|---|
| 看某人待审批的任务列表 | `approval +tasks --assignee <badge>` |
| 看某条任务详情(含表单字段) | `approval +task --task-id <id>` |
| 看某员工发起的实例(已发起的审批) | `approval +instances --employee <eid>` |

## 命令骨架

```bash
hr approval +tasks --assignee P000487 --limit 20
hr approval +task --task-id <task-id>
hr approval +instances --employee 94 --status pending --limit 50
```

### `+tasks` flag
- `--assignee <badge>`:看谁的待办,默认当前操作者
- `--limit`:默认 50

### `+task` flag
- `--task-id <string>`:必填,从 `+tasks` 结果里拿

### `+instances` flag
- `--employee <eid>`:看谁发起的
- `--status pending|approved|rejected|all`(可选,过滤)
- `--limit`

## 输出契约

`+tasks` 成功:

```json
{"ok": true, "data": {
  "count": 3,
  "rows": [{
    "task_id": "...",
    "instance_code": "...",
    "title": "调动审批 - 张三",
    "assignee_eid": 94,
    "created_at": "...",
    "status": "pending"
  }]
}}
```

`+task` 详情会包含表单字段(`form: [...]`)和审批链路(`flow: [{node, approver, ...}]`)。

## Agent 规则

1. **写动作不要试**。看到用户说"批准这条审批" → 告诉他去钉钉,不要尝试调用任何 `+approve/+reject` 命令。
2. **task_id 是查任务详情的唯一钥匙** — 从 `+tasks` 里取,不要让用户自己拼。
3. **instance_code 是审批实例 ID**,用于 `+instances` 跨任务追踪同一条审批的全链路。
4. **count: 0 是常态** — 大多数员工大多数时候没有待办,不要把"空"当 bug。

## 常见错误

| subtype | 含义 | 处理 |
|---|---|---|
| `policy/approval_write_not_verified` | 调了 1.0 不支持的写动作 | 引导去钉钉 |
| `validation/missing_assignee` | `+tasks` 没传 `--assignee` 也没默认 | 加 `--assignee <badge>` |
| `validation/missing_task_id` | `+task` 没带 task_id | 先 `+tasks` 拿 task_id |
| `authorization/action_denied` | 当前角色无 `approval.task.list` | `perm explain` 查 |

## 用法实例

```bash
# 吴邦的待办
$ hr approval +tasks --assignee P000487

# 看某条任务详情
$ hr approval +task --task-id 11223344

# 吴邦发起过的所有 pending 审批
$ hr approval +instances --employee 94 --status pending
```
