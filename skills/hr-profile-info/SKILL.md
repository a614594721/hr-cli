---
name: hr-profile-info
version: 1.0.0
description: "HR 个人资料修改 — 改 personal_info 表的字段(昵称/地址/紧急联系人/银行卡等)。强制 preview → apply 两阶段。preview 显示打码值,真值只在 server 端 secret store。"
metadata:
  requires:
    bins: ["hr"]
  cliHelp: "hr profile-info --help"
---

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../hr-shared/SKILL.md`](../hr-shared/SKILL.md)。两阶段写模式 + 测试员工规则在 hr-shared 里。**

## 字段白名单

`--set field=value` 的 field 必须在白名单内,否则 server 返 `authorization/field_denied`。

### 普通字段(任何角色都能改自己 scope 内的)

```
nickname address household_address
emergency_contact emergency_phone emergency_relation
marital_status computer_preference personal_intro
```

### 敏感字段(必须 HR_ADMIN + `--sensitive` 标志)

```
id_number bank_card bank_name branch_name bank_code
provident_fund_account phone
```

⚠️ 试图传任何不在这两个列表里的字段(比如 `EMAIL` / `MOBILE` / 自定义字段)会被直接拒。

## 命令骨架

```bash
# 阶段 1:预览(可叠加多个 --set)
hr profile-info +preview \
  --user-id 6711 \
  --set nickname=test \
  --set address="北京市朝阳区..."

# 敏感字段需要 --sensitive(且当前角色必须 HR_ADMIN)
hr profile-info +preview \
  --user-id 6711 \
  --set bank_card=6228... \
  --sensitive

# 阶段 2:执行
hr profile-info +apply <preview_id> --yes
```

### `+preview` flag
- `--user-id <int>`:**personal_info 表的 user_id**(不是员工 EID!),从 `personal_info.user_id` 字段取
- `--set field=value`:可重复,白名单字段
- `--sensitive`:解锁敏感字段白名单,需要 HR_ADMIN

### `+apply` flag
- 位置参数 `<preview_id>`
- `--yes`

## 输出契约

`+preview` 成功:

```json
{"ok": true, "data": {
  "preview_id": "20260616-023821-4f633b",
  "kind": "profile-info",
  "plan": {
    "operator": {"role":"HR_ADMIN", ...},
    "target":   {"name":"袁洁", "personal_info_id": 21, "user_id": 6711},
    "changes": [{
      "field": "nickname",
      "new":   "test",
      "old":   "袁洁",
      "old_hash": "5b9a38c057f4f87714cc61a2e7c6e321b1b1eee9d7b8e8cb886551e9bc2b814a",
      "sensitive": false
    }],
    "write_path": [
      "personal_info",
      "existing trigger synchronization verification"
    ],
    "status": "preview_only"
  }
}}
```

注意:
- **preview 里 `old / new` 显示的是真值**(对非敏感字段)。Apply 时 server 用 `old_hash` 重新对比 DB 现状,**防并发改写**。
- 对敏感字段,preview 里 `new` 会显示**打码版本**;真值在 gateway server 的 secret store(`/var/lib/hr-gateway/previews/secrets/`,权限 0700),apply 时从 secret store 取。

## Agent 必读规则

1. **`--user-id` 是 personal_info.user_id,不是 EID**。这是 HR 系统两套 ID 的常见混淆点。要改员工 X 的资料,先 `hr employee get --eid X` 拿到 EID,再去 `personal_info` 表查对应的 user_id(目前 hr-cli 不直接做这层映射,需用户/管理员告知 user_id)。
2. **白名单严格**。`EMAIL` 不在 personal_info 白名单(EMAIL 在 `eemployee` 表)。看到用户说"改邮箱"要追问"改 personal_info 还是 eemployee" — 后者不属于本 capability,1.0 不支持。
3. **敏感字段加 `--sensitive` 而且必须 HR_ADMIN**。`MANAGER/HRBP/SELF` 即使加了 `--sensitive` 也会被拒 `authorization/sensitive_field_denied`。
4. **同样的两阶段规则** — preview → user 确认 changes → apply。preview_id 一次性。
5. **吴邦不能用作 profile-info 写测试目标** — 他在 `personal_info` 表无对应行。固定测试目标用 **袁洁(`user_id=6711`)**。
6. **EPRE_STAFFREGISTER 触发器** — apply 后服务端会检查 personal_info 的字段写入是否正确级联到 `staff_register` 表。如果触发器未触发或同步失败,apply 会返回 `internal/trigger_sync_failed`,让 HR 管理员排查触发器状态。

## 常见错误

| subtype | 含义 | 处理 |
|---|---|---|
| `validation/missing_changes` | 没传 `--set` | 加至少一个 |
| `validation/not_found` | user_id 在 personal_info 里没行 | 换 user_id 或先确认目标 |
| `authorization/field_denied` | field 不在白名单 | 看上方白名单表,选合法字段 |
| `authorization/sensitive_field_denied` | 改敏感字段缺 `--sensitive` 或角色不是 HR_ADMIN | 加 `--sensitive`(需 HR_ADMIN) |
| `validation/old_value_mismatch` | apply 时 DB 现状和 preview 时不同 | 重新 preview |
| `internal/trigger_sync_failed` | EPRE_STAFFREGISTER 触发器同步失败 | 找 HR 管理员 |

## 用法实例(袁洁,no-op,测试)

```bash
# 1. preview — old=new,nickname 改成袁洁原值
$ hr profile-info +preview --user-id 6711 --set nickname=袁洁

# 2. apply
$ hr profile-info +apply <preview_id> --yes
```
