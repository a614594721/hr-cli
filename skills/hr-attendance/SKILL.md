---
name: hr-attendance
version: 1.0.0
description: "HR 考勤查询。员工打卡明细、月度/单日汇总、异常记录。读操作,1.0 无写动作。"
metadata:
  requires:
    bins: ["hr"]
  cliHelp: "hr attendance --help"
---

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../hr-shared/SKILL.md`](../hr-shared/SKILL.md)。**

## 选哪个命令

| 想做什么 | 命令 |
|---|---|
| 某员工日打卡明细(每天一行) | `attendance +records` |
| 某员工某日汇总 / 部门当日汇总 | `attendance +summary` |
| 异常记录(REMARK 非空) | `attendance +exceptions` |

## 命令骨架

```bash
hr attendance +records --eid 94 --from 2026-05-01 --to 2026-05-31 --limit 100
hr attendance +records --badge P000487 --from 2026-05-01 --to 2026-05-31

hr attendance +summary --badge P000487 --date 2026-05-15
hr attendance +summary --dept 107 --date 2026-05-15

hr attendance +exceptions --from 2026-05-01 --to 2026-05-31
```

### `+records` flag
- `--eid` 或 `--badge`(任选一个,不能同传)
- `--from` / `--to`:`YYYY-MM-DD`,闭区间
- `--limit`:默认 100

### `+summary` flag
- `--badge`(单人)或 `--dept <int>`(部门维度)
- `--date`:`YYYY-MM-DD`,单日

### `+exceptions` flag
- `--from` / `--to`,无 EID 限制时返回操作者 scope 内全部异常

## 输出契约

成功(records / exceptions):

```json
{"ok": true, "data": {
  "count": 2,
  "rows": [{
    "EID": 94, "NAME": "吴邦", "BADGE": "P000487",
    "TERM": "2026-05-31T00:00:00Z",
    "attend_total": "0.0000",
    "card_times": null, "cardbegintime": null, "cardendtime": null,
    "REMARK": null,
    "AMOUNT_1477": null, "AMOUNT_1478": null, "...": "金额项按企业字典展开"
  }]
}}
```

成功(summary):同 envelope,`rows` 是单条聚合或部门聚合(几行)。

## Agent 规则

1. **日期窗口要合理**。一次 `+records` 跨度别超过 3 个月,数据量大会撞 limit;需要更长窗口让用户分段查。
2. **异常 vs 缺勤 vs 加班** 这三种语义在 `REMARK` 文案里,字段里没有结构化 flag。读完用户问题再去 `REMARK` 字段里 substr 判断。
3. **`AMOUNT_<num>` 这堆字段是公司假别字典 ID** — `1477/1478/...` 各对应"年假/调休/事假"等。Agent 不要瞎猜映射;如果用户问字段含义,告诉他这是 HR 字典 ID,具体含义看 `D:\projects\DB-Knowledge` 或问 HR 管理员。
4. **scope** — 与 employee 同样,HR_ADMIN/SSC 旁路;HRBP/MANAGER 仅看本人辐射。`+records --eid X` 如果目标不在 scope,会 `authorization/target_out_of_scope`。`+exceptions` 不带 EID 时 gateway 自动按 scope 过滤,不会泄露范围外数据。
5. **`count: 0` 不一定是 bug** — 可能那天确实没打卡(假期/未在职)。结合 `employee get` 的 `JOINDATE / LEAVEDATE` 判断。

## 常见错误

| subtype | 含义 | 处理 |
|---|---|---|
| `validation/missing_period` | 没传 `--from/--to` | 补上日期 |
| `validation/invalid_date` | 日期格式不对 | 改成 `YYYY-MM-DD` |
| `authorization/action_denied` | 角色无 `attendance.records.query` 等 | 用 `perm explain` 查 |
| `authorization/target_out_of_scope` | 目标不在 scope | 切上级角色 |

## 用法实例

```bash
# 吴邦 5 月明细(HR_ADMIN 旁路)
$ hr attendance +records --eid 94 --from 2026-05-01 --to 2026-05-31 --limit 5

# 共享服务组 5/15 当日汇总
$ hr attendance +summary --dept 107 --date 2026-05-15

# 5 月所有异常(scope 内)
$ hr attendance +exceptions --from 2026-05-01 --to 2026-05-31
```
