# Project Rules

## 架构参考

- 本项目的架构模式完全借鉴 [larksuite/cli](https://github.com/larksuite/cli)（飞书 CLI），采用其工程级别的设计框架来实现。
- 涉及架构、模块划分、命令组织、配置管理、认证流程、扩展机制等设计方案时，重点参考飞书 CLI 项目的实现思路。
- 在新增功能或重构时，优先沿用飞书 CLI 的目录结构与抽象层次，保持工程风格一致。
- 飞书 CLI 源码已克隆到本地 `D:\projects\lark-cli`，需要查阅设计实现时直接读取该目录，不要再去访问 GitHub。
  - 顶层结构：`cmd/`（命令注册）、`internal/`（核心逻辑）、`errs/`、`events/`、`extension/`、`sidecar/`、`skills/`、`skill-template/`、`shortcuts/`、`tests/`、`scripts/`、`lint/` 等。
  - 入口：`main.go` 与按 build tag 区分的 `main_authsidecar.go` / `main_noauthsidecar.go`。

## 数据库

- 环境变量 `DB_HOST` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` 等均指向测试环境（`DB_ENV=test`），可直接执行查询、写入、迁移、清理等操作，无需逐条确认。
- 项目代码中如未配置数据库连接，优先使用上述环境变量，不要硬编码连接信息。
- 仅当对话中明确提供生产连接信息，或 `DB_ENV` 不为 `test` 时，才视为生产环境并谨慎处理。

## 测试员工

- 一次性测试员工固定为 **吴邦**（badge `P000487`，EID/URID `94`，role `HR_ADMIN`）。
- hr-cli 的所有冒烟测试、`auth +login`、`employee +find/get`、`attendance +records/+summary/+exceptions`、`approval +tasks/+task/+instances`、`transfer +preview/+apply`、`profile-info +preview/+apply` 默认以吴邦作为操作者或目标员工。
- 不得在未获明确授权的前提下用其它真实员工做写操作（apply/--yes）。如需新增测试员工，先在对话中确认。
- 写操作（transfer/profile-info `--yes`）前必须确认回滚路径；no-op 写测试（如旧值=新值的资料修改）允许直接执行。
- HRBP 测试操作者固定为 **董寰宇**（badge `P000504`，EID `1`，role `HRBP`，国际HRBP组）。
  - 用法：`HR_OPERATOR_EID=1 HR_OPERATOR_ROLE=HRBP ./hr.exe ...`
  - 在范围正向用例：黎子豪（EID `439`，用户产品研发组）。
  - 越权负向用例：吴邦（EID `94`），不在董寰宇辐射范围内。
- profile-info 写测试沿用 **袁洁**（personal_info `user_id=6711`），no-op 写已审计通过；吴邦在 `personal_info` 无对应行，profile-info 不能用吴邦本人。

## 权限模型

- 动作级权限位图在 `internal/perm/perm.go`，目标级 scope 闸门在 `internal/perm/scope.go`，DB 角色解析在 `internal/auth/role.go`。
- hr-cli 角色优先级（高→低）：`HR_ADMIN > SSC > HRBP > MANAGER > SELF`。
- DB 角色映射（`skysecrole.ID → hr-cli role`）：
  - `181084 admin-临时` → `HR_ADMIN`
  - `181090 SSC` → `SSC`
  - `181089 HRBP` → `HRBP`
  - `181083 一级部门负责人自助` → `MANAGER`
  - `1001 信飞PC-员工自助` → `SELF`
- `auth +login` 默认从 `skysecrolemember JOIN skysecuser` 反查 EID 的角色集合，按优先级取最高写入 session；未传 `--role` 时此结果生效，传了则以 flag 为准（仅测试场景）。
- `HR_OPERATOR_*` 环境变量仅在 `DB_ENV=test` 下被 `CurrentOperator` 接受；其它环境一律忽略以避免本机伪造身份。
- 目标级 scope 直接读 `psoradiationrangeeidlist (ueid, eid)`：`HR_ADMIN` 与 `SSC` 旁路（与 PsoRadiationRange_new 超管白名单语义一致），其它角色未命中即 `target_out_of_scope`。
- `psoradiationrangeeidlist` 由 `pro_cr_PsoRadiationRangeEidlist` 批量重建（`TRUNCATE + INSERT`），不是实时维护。CLI 不主动刷该表；调动后 scope 生效需等下一轮批跑（缓存延迟可接受）。
- `perm.Require(action, targetEID)`：传 targetEID 时同时执行动作 + scope 双闸；未传 targetEID 时仅动作级（list/search 类）。
- `employee +find` 在 SQL 层加 scope 过滤子句（`EID IN (SELECT eid FROM psoradiationrangeeidlist WHERE ueid=?)`），HR_ADMIN/SSC 旁路；响应包含 `scope` 元数据用于排错。

## 审计

- `internal/audit/audit.go` 双写：本机 `.hr-cli/audit/YYYYMMDD.jsonl` + DB 表 `hr_cli_audit_log`。
- 表 schema：`id / created_at / event / operator_eid / operator_name / operator_role / target_eid / preview_id / payload_json / client_host / client_user / cli_version / db_env`，4 个索引（operator/event/target/preview）。
- 表自动创建：`audit.Write` 在每个进程首次写入时执行 `CREATE TABLE IF NOT EXISTS`。
- 失败行为：DB 写失败降级为 stderr 警告，不阻塞业务；本机 JSONL 仍然写入作为兜底。
- 当前写审计的路径：`transfer.apply.{start,success,failed}`、`profile_info.apply.{start,success,failed}`。dry-run 不写审计。

## 真实角色样本（用于回归测试）

| 姓名 | EID | DB 角色 | hr-cli role | scope size | 用途 |
|---|---|---|---|---|---|
| 吴邦 | 94 | admin-临时 (181084) | HR_ADMIN | 8033（白名单全员）| 默认操作者 / 写测试 |
| 曹晓蕾 | 1124 | SSC (181090) | SSC | 不查（旁路） | SSC 旁路验证 |
| 董寰宇 | 1 | HRBP (181089) | HRBP | 135 | HRBP scope 边界 |
| 王婧 | 684 | 一级部门负责人自助 (181083) | MANAGER | 108 | MANAGER scope 边界 |
| 邱冠宇 | 426 | 一级部门负责人自助 (181083) | MANAGER | 8033（PsoRadiationRange 函数白名单） | 不要用于 scope 测试，会假阳性 |

## 数据库业务知识参考

- 涉及数据库表、字段、存储过程、触发器、审批流、考勤、员工、调动、个人资料等业务含义或链路不清楚时，优先参考 `D:\projects\DB-Knowledge`。
- `D:\projects\DB-Knowledge` 可作为 HR 数据库业务知识库使用，包括表结构文档、存储过程说明、变更脚本、触发器、索引和历史方案。
- 不要把 `D:\projects\DB-Knowledge` 中的内容机械复制进本项目；只提取当前功能需要的表/字段/链路结论，并在本项目文档中记录必要摘要。
- 如果实际测试库 schema 与 `D:\projects\DB-Knowledge` 文档不一致，以当前测试库只读探查结果为准，并在相关文档中注明差异。
