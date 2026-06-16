---
name: hr-transfer
version: 1.0.0
description: "HR 调动 — 修改员工部门或岗位。强制 preview → apply 两阶段。preview 是 dry-run,apply 是真实写库,会触发存储过程链 eSP_EmpChangeAdd/Check/Start。"
metadata:
  requires:
    bins: ["hr"]
  cliHelp: "hr transfer --help"
---

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../hr-shared/SKILL.md`](../hr-shared/SKILL.md)。两阶段写模式 + 测试员工规则在 hr-shared 里。**

## 写动作两阶段

```
+preview (dry-run, 落 preview_id)  ──►  +apply <preview_id> --yes (真写库)
```

**永远先 preview,review changes,再 apply**。preview 输出包含完整 plan,要让用户**逐字段确认**。

## 命令骨架

```bash
# 阶段 1:预览
hr transfer +preview \
  --badge P000487 \
  --dept 107 \
  --job 58 \
  --effect-date 2026-07-01 \
  --reason "组织调整"

# 阶段 2:执行
hr transfer +apply <preview_id> --yes
```

### `+preview` flag(全部必填)
- `--badge <string>` 或 `--eid <int>`:目标员工
- `--dept <int>`:新部门 DPID(传现部门 = no-op)
- `--job <int>`:新岗位 JBID
- `--effect-date YYYY-MM-DD`:生效日
- `--reason "..."`:必填,会写入审计

### `+apply` flag
- 位置参数 `<preview_id>`(从 +preview 输出取)
- `--yes`:**显式确认**,不能省

## 输出契约

`+preview` 成功:

```json
{"ok": true, "data": {
  "preview_id": "20260616-024259-b537c7",
  "kind": "transfer",
  "created_at": "...",
  "plan": {
    "capability": "transfer.plan",
    "operator": {"eid":"94", "role":"HR_ADMIN", ...},
    "target":   {"EID":94, "NAME":"吴邦", "DPID":107, "JBID":58, ...},
    "changes": [
      {"field":"DPID", "new":107, "old":107},
      {"field":"JBID", "new":58,  "old":58},
      {"field":"EFFECTDATE", "new":"2026-07-01", "old":null},
      {"field":"reason", "new":"组织调整", "old":null}
    ],
    "transfer_type": 12,
    "native_chain": [
      "CALL eSP_EmpChangeAdd(...)",
      "UPDATE eEmployee_Work ...",
      "CALL eSP_EmpChangeCheck(...)",
      "CALL eSP_EmpChangeStart(...)"
    ],
    "write_path": [...],
    "status": "preview_only"
  }
}}
```

`+apply` 成功:返回 `affected_rows` + 涉及的 `work_id` + audit 写入确认。

## Agent 必读规则

1. **永远先 preview,把 `plan.changes` 完整列给用户**。每个 `{field, old, new}` 行,告诉用户什么字段从什么改成什么,**等用户明确说"执行"才 apply**。
2. **`old==new` 是 no-op 写**。preview 里 `changes` 大部分行 old=new 表示这次调动其实没改变量。在测试库(`DB_ENV=test`)下 no-op 允许直接 apply 不要求复确认;生产环境一律两阶段。
3. **preview_id 一次性**。apply 后失效,如果用户中途犹豫然后又想 apply,**重新 preview**(get fresh data + fresh preview_id)。
4. **存储过程链不可绕过**。看到 `native_chain` 里 `eSP_EmpChangeAdd / Check / Start` 这套是必经路径 — 不要试图跳过任何一步直接 update `eemployee_work`,会丢审批触发器和状态机。
5. **写测试用吴邦**(badge `P000487`)。不要用真实在职员工调动测,即使是 no-op:no-op 写也会写入审计 `transfer.apply.start/success`,生产环境会污染调动日志。
6. **`reason` 不要造假**。会写入 `eemployee_work.reason`,影响后续审批。"测试"、"smoke"等占位词适合测试库,生产应有真实理由。

## 常见错误

| subtype | 含义 | 处理 |
|---|---|---|
| `validation/missing_required` | `--dept/--job/--effect-date/--reason` 缺一 | 补全 |
| `validation/employee_not_found` | badge 或 EID 查不到 | 先 `employee +find` |
| `authorization/action_denied` | 角色无 `transfer.preview` | `perm explain` 查 |
| `authorization/target_out_of_scope` | 目标 EID 不在操作者 scope | 切上级角色 |
| `policy/concurrent_change_pending` | 该员工已有 open 状态的调动单 | 等前一个 apply 完或 cancel |
| `validation/old_value_mismatch` | apply 时 DB 现状和 preview 时不同(并发改) | 重新 preview |
| `confirmation/confirm_header_required` | apply 没带 `--yes` | 加 `--yes` |

## 用法实例(吴邦,no-op,测试)

```bash
# 1. preview — old=new,纯 sanity check
$ hr transfer +preview --badge P000487 --dept 107 --job 58 \
    --effect-date 2026-07-01 --reason "smoke test"
# 取出 preview_id: 20260616-024259-b537c7
# 检查 plan.changes:DPID/JBID old==new(no-op)

# 2. 真正写(测试库 + no-op 允许)
$ hr transfer +apply 20260616-024259-b537c7 --yes
```
