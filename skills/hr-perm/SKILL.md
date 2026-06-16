---
name: hr-perm
version: 1.0.0
description: "HR 权限解释 — 调用前先用它查 action+target 是否会通过 perm/scope 检查。诊断 action_denied / target_out_of_scope 错误的第一步。读操作,无副作用。"
metadata:
  requires:
    bins: ["hr"]
  cliHelp: "hr perm explain --help"
---

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../hr-shared/SKILL.md`](../hr-shared/SKILL.md)。角色优先级 + scope 模型在 hr-shared 里。**

## 何时用

1. **写动作 dry-check**:在 `transfer/profile-info +preview` 之前,先 `perm explain` 确认目标会通过。
2. **错误诊断**:看到 `authorization/action_denied` 或 `target_out_of_scope`,用它定位是 action 没权限还是 scope 没覆盖。
3. **角色边界探索**:HRBP/MANAGER 不确定能不能查某员工时,先 explain。

## 命令骨架

```bash
hr perm explain --action <name>
hr perm explain --action <name> --target-eid <int>
```

### flag
- `--action <string>`:必填。capability vocab,例 `transfer.preview / employee.find / profile_info.apply`
- `--target-eid <int>`:可选。**没传时只查 action 级权限**;传了同时查 action + target scope。

## action 字典

| 域 | actions |
|---|---|
| auth | `auth.me` |
| employee | `employee.find` `employee.get` |
| attendance | `attendance.records.query` `attendance.summary.query` `attendance.exception.query` |
| approval | `approval.task.list` `approval.task.get` `approval.task.approve` `approval.task.reject` `approval.task.transfer` |
| transfer | `transfer.preview` `transfer.apply` |
| profile_info | `profile_info.preview` `profile_info.apply` |

注意:perm 决策表中,**HR_ADMIN/SSC 是通配 `*: true`**,任意 action 都允许。

## 输出契约

```json
{"ok": true, "data": {
  "action": "transfer.preview",
  "target_eid": "94",
  "operator": {"eid":"94", "role":"HR_ADMIN", ...},
  "decision": "allow",
  "reason": "action is allowed for role HR_ADMIN",
  "scope": "all",
  "target_scope": {
    "source": "psoradiationrangeeidlist",
    "decision": "allow",
    "reason": "role HR_ADMIN bypasses target-scope check",
    "operator_eid": "94",
    "target_eid": "94"
  }
}}
```

字段含义:

- **`decision`** — 最终结论:`allow / deny`。**双闸**:action 必须 allow **且**(如果传了 target-eid)target_scope 也必须 allow。
- **`reason`** — 人话解释。
- **`scope`** — 操作者的 scope 类型:`all / hrbp_scope / direct_reports / self / none`。
- **`target_scope`**(仅当 `--target-eid` 传时存在):
  - `source: psoradiationrangeeidlist` — 数据来源,该表由批跑维护,**非实时**
  - `decision: allow / deny / error` — error 表示 scope 查询本身失败(DB 不可达等)

## Agent 规则

1. **写动作前 explain**。`transfer/profile-info +apply` 前,先 `perm explain --action xxx.apply --target-eid <X>`。如果 explain 已经 deny,**根本别 preview**,省时间。
2. **deny 不一定永久** — `psoradiationrangeeidlist` 由 `pro_cr_PsoRadiationRangeEidlist` 批跑(TRUNCATE+INSERT)维护,新调动后 scope 生效要等下一轮批。如果用户刚调动完发现某员工不在 scope 内,告诉他**等下一轮批跑后再查**(批跑频率向 HR 管理员确认)。
3. **`scope: hrbp_scope` 但 size = 8033** — 这是批跑函数把超管白名单也写进了 `eidlist`,产生**假阳性大 scope**。邱冠宇(EID 426)就是已知样本。perm explain 看到这种情况(role 不是 HR_ADMIN/SSC 但 target 几乎无所不包),应**怀疑数据**而不是怀疑权限。
4. **HR_ADMIN/SSC 永远 allow** — `bypass_role` 字段会出现,这是预期。
5. **`target_scope.decision = "error"` 不是 deny** — 是查 scope 时 DB 失败。提醒用户 gateway/DB 状态,不要把它当成"无权"。

## 常见错误

| subtype | 含义 | 处理 |
|---|---|---|
| `validation/missing_action` | `--action` 没传 | 必填 |
| `validation/invalid_action` | action 名不在字典 | 看上方表 |

## 用法实例

```bash
# action 级:吴邦能不能 transfer 任何人(不指定目标)
$ hr perm explain --action transfer.apply
# decision: allow (HR_ADMIN 通配)

# 双闸:吴邦能不能改袁洁的 personal_info
$ hr perm explain --action profile_info.apply --target-eid <袁洁 EID>

# HRBP 边界探索(先 impersonate 切到 HRBP,再 explain)
$ hr auth impersonate --eid 1            # 切到董寰宇 (HRBP)
$ hr perm explain --action transfer.preview --target-eid 439   # 黎子豪 in scope
$ hr perm explain --action transfer.preview --target-eid 94    # 吴邦 out of scope
```
