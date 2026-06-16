---
name: hr-employee
version: 1.0.0
description: "HR 员工目录查询。按 name/badge/phone/email 模糊搜索员工(列表),或按 EID 取员工详情(单条)。读操作,无写动作。"
metadata:
  requires:
    bins: ["hr"]
  cliHelp: "hr employee --help"
---

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../hr-shared/SKILL.md`](../hr-shared/SKILL.md),其中包含认证、scope、错误 envelope 处理。**

## 选哪个命令

| 想做什么 | 命令 |
|---|---|
| 已知 EID,要详情 | `employee get --eid <int>` |
| 按工号查 | `employee +find --badge <string>` |
| 按姓名模糊查 | `employee +find --name <string>` |
| 按手机号查 | `employee +find --phone <string>` |
| 按邮箱查 | `employee +find --email <string>` |

## 命令骨架

```bash
hr employee +find --badge P000487
hr employee +find --name 吴邦 --limit 20
hr employee get --eid 94
```

flag:

- `+find`:`--name | --badge | --phone | --email`(任选一个或多个),`--limit`(默认 100)
- `get`:`--eid <int>` 必填

## 输出契约

`+find` 成功:

```json
{"ok": true, "data": {
  "count": 1,
  "rows": [{
    "EID": 94, "NAME": "吴邦", "badge": "P000487",
    "DPID": 107, "DPTITLE": "共享服务组",
    "JBID": 58,  "JBTITLE": "HRIS",
    "EMAIL": "...", "MOBILE": "159****7653",
    "REPORTTONAME": "...", "STATUS": 1,
    "actions": {"can_edit_profile": true, "can_query_attendance": true, "can_transfer": true}
  }],
  "scope": {"applied": false, "bypass_role": "HR_ADMIN", "reason": "..."},
  "truncated": false
}}
```

`get` 成功:同 row 结构,直接放 `data` 顶层(不含 rows 数组),且字段更全(包含 `CERTNO` 已脱敏)。

## Agent 规则

1. **多人匹配时停下来问用户**,不要假设第一个就是目标。`+find` 返回 `count > 1` 时,把姓名/部门列出来让用户挑。
2. **后续操作用 EID 或 badge 而不是 name**。后续命令(如 transfer)接受的是 badge / EID,不接姓名。
3. **redact 已经在服务端做了** — `MOBILE` 显示 `159****7653`、`CERTNO` 显示 `4211********1911`。**不要尝试**绕过获取原值;真正需要原值的写操作应通过 `transfer / profile-info` 而非读路径。
4. **scope 元数据用于排错** — `data.scope` 告诉你 gateway 是否对结果做了 scope 过滤。`applied: false` + `bypass_role` 表示当前角色旁路;`applied: true` + `count` 偏少时,说明实际匹配项被过滤掉了 — 此时让用户确认是否需要更高角色。
5. **`+find` 不带任何 flag 会拒**(gateway validation),至少传一个识别字段。

## 常见错误

| subtype | 含义 | 处理 |
|---|---|---|
| `validation/missing_query` | 没有任何 find 条件 | 加 `--name / --badge / ...` 之一 |
| `authorization/action_denied` | 当前角色没有 `employee.find` 权限 | `perm explain --action employee.find` 看根因 |
| `authorization/target_out_of_scope` | `get --eid X` 的目标不在范围 | 让操作者切到上级角色 |

## 用法实例(吴邦,HR_ADMIN)

```bash
$ hr employee +find --badge P000487
{"ok":true,"data":{"count":1,"rows":[{"EID":94,"NAME":"吴邦",...}],
 "scope":{"bypass_role":"HR_ADMIN","reason":"role HR_ADMIN bypasses scope filter"}}}

$ hr employee get --eid 94
{"ok":true,"data":{"CERTNO":"4211********1911","EID":94,"NAME":"吴邦",...}}
```
