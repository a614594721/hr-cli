# Project Rules

## 架构

hr-cli 已从「胖客户端 + 直连 DB」迁移为「瘦客户端 + hr-gateway」架构。详见 `docs/hr-cli-architecture-credential-isolation.md`。

- **hr-cli**(本仓库):纯 HTTP 客户端,不持有 DB 凭证,不实现 perm/scope/audit。
- **hr-gateway**(`D:\projects\hr-gateway`):承担 DB 直连 + perm/scope 决策 + 审计 + DingTalk OAuth broker。
- **bi_ehr**:OAuth broker 已迁出,bi_ehr 回到「业务系统」本职。

工程结构沿用 [larksuite/cli](https://github.com/larksuite/cli) 的命令组织、配置管理、错误 envelope 思路。本地参考源码 `D:\projects\lark-cli`,不要再访问 GitHub。

## 数据库

**hr-cli 客户端不再持有 DB 凭证**。开发或测试需要直接查 DB 时,在 hr-gateway 仓库内操作:

- hr-gateway 仓库路径:`D:\projects\hr-gateway`
- 启动本地 gateway:`cd D:\projects\hr-gateway && make dev-gateway`
- gateway 默认监听 `127.0.0.1:18080`
- gateway `.env` 中的 `DB_HOST` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` 指向测试环境(`DB_ENV=test`),可执行查询、写入、迁移、清理等

hr-cli 配置只需要 `auth_base_url`:

```bash
hr profile add dev --auth-base-url http://127.0.0.1:18080
hr profile use dev
hr auth +login --dingtalk
```

仅当对话中明确提供生产连接信息,或 `DB_ENV` 不为 `test` 时,才视为生产环境并谨慎处理。

## 测试员工

- 一次性测试员工固定为 **吴邦**(badge `P000487`,EID/URID `94`,role `HR_ADMIN`)。
- hr-cli 的所有冒烟测试、`auth +login --dingtalk`、`employee +find/get`、`attendance +records/+summary/+exceptions`、`approval +tasks/+task/+instances`、`transfer +preview/+apply`、`profile-info +preview/+apply` 默认以吴邦作为操作者或目标员工。
- 不得在未获明确授权的前提下用其它真实员工做写操作(`apply --yes`)。如需新增测试员工,先在对话中确认。
- 写操作(`transfer/profile-info --yes`)前必须确认回滚路径;no-op 写测试(如旧值=新值的资料修改)允许直接执行。

### HRBP 测试

- 测试操作者固定为 **董寰宇**(badge `P000504`,EID `1`,role `HRBP`,国际HRBP组)。
- **不再支持** `HR_OPERATOR_EID=1 ./hr.exe ...` 这类本机伪造身份方式 —— 客户端不再读 `HR_OPERATOR_*`,operator 只能由 gateway 颁发的 access_token 解出。
- 切换到 HRBP 身份的方法:`hr auth impersonate --eid 1`,该接口仅 HR_ADMIN 可用且强制写审计,签发 15 分钟 TTL 的短 token。
  - 在范围正向用例:黎子豪(EID `439`,用户产品研发组)。
  - 越权负向用例:吴邦(EID `94`),不在董寰宇辐射范围内。
- profile-info 写测试沿用 **袁洁**(personal_info `user_id=6711`),no-op 写已审计通过;吴邦在 `personal_info` 无对应行,profile-info 不能用吴邦本人。

## 权限模型

- 实现位置已迁移至 `hr-gateway/internal/perm/` 与 `hr-gateway/internal/auth/`,**hr-cli 客户端不再有这些代码**。
- hr-cli 角色优先级(高→低):`HR_ADMIN > SSC > HRBP > MANAGER > SELF`。
- DB 角色映射(`skysecrole.ID → hr-cli role`):
  - `181084 admin-临时` → `HR_ADMIN`
  - `181090 SSC` → `SSC`
  - `181089 HRBP` → `HRBP`
  - `181083 一级部门负责人自助` → `MANAGER`
  - `1001 信飞PC-员工自助` → `SELF`
- gateway 在签发 access_token 时,从 `skysecrolemember JOIN skysecuser` 反查 EID 的角色集合,按优先级取最高写入 JWT claim。
- 目标级 scope 由 gateway 直接读 `psoradiationrangeeidlist (ueid, eid)`:`HR_ADMIN` 与 `SSC` 旁路(与 PsoRadiationRange_new 超管白名单语义一致),其它角色未命中即 `target_out_of_scope`。
- `psoradiationrangeeidlist` 由 `pro_cr_PsoRadiationRangeEidlist` 批量重建(`TRUNCATE + INSERT`),非实时维护。gateway 不主动刷;调动后 scope 生效需等下一轮批跑(缓存延迟可接受)。
- `perm.Require(action, targetEID)`:传 targetEID 时同时执行动作 + scope 双闸;未传 targetEID 时仅动作级。
- `employee +find` 在 SQL 层加 scope 过滤子句(`EID IN (SELECT eid FROM psoradiationrangeeidlist WHERE ueid=?)`),HR_ADMIN/SSC 旁路;响应包含 `scope` 元数据用于排错。
- 客户端要查权限时调 `POST /api/hr-cli/v1/perm/explain`(对应命令 `hr perm explain`)。

## 审计

- 实现位置:`hr-gateway/internal/audit/audit.go`,**hr-cli 客户端不写审计**。
- 双写:gateway 服务器本机 `.hr-gateway/audit/YYYYMMDD.jsonl` + DB 表 `hr_cli_audit_log`。
- 表 schema:`id / created_at / event / operator_eid / operator_name / operator_role / target_eid / preview_id / payload_json / client_host / client_user / cli_version / db_env`,4 个索引(operator/event/target/preview)。
- 表自动创建:`audit.Write` 在每个进程首次写入时执行 `CREATE TABLE IF NOT EXISTS`。
- 失败行为:DB 写失败降级为 stderr 警告,不阻塞业务;gateway 本机 JSONL 仍然写入作为兜底。
- 当前写审计的路径:`transfer.apply.{start,success,failed}`、`profile_info.apply.{start,success,failed}`、`auth.impersonate.start`。dry-run 不写审计。

## 真实角色样本(用于回归测试)

| 姓名 | EID | DB 角色 | hr-cli role | scope size | 用途 |
|---|---|---|---|---|---|
| 吴邦 | 94 | admin-临时 (181084) | HR_ADMIN | 8033(白名单全员) | 默认操作者 / 写测试 |
| 曹晓蕾 | 1124 | SSC (181090) | SSC | 不查(旁路) | SSC 旁路验证 |
| 董寰宇 | 1 | HRBP (181089) | HRBP | 135 | HRBP scope 边界 |
| 王婧 | 684 | 一级部门负责人自助 (181083) | MANAGER | 108 | MANAGER scope 边界 |
| 邱冠宇 | 426 | 一级部门负责人自助 (181083) | MANAGER | 8033(PsoRadiationRange 函数白名单) | 不要用于 scope 测试,会假阳性 |

切换到上述角色身份用 `hr auth impersonate --eid <EID>`。

## 数据库业务知识参考

- 涉及数据库表、字段、存储过程、触发器、审批流、考勤、员工、调动、个人资料等业务含义或链路不清楚时,优先参考 `D:\projects\DB-Knowledge`。
- 该目录可作为 HR 数据库业务知识库使用,包括表结构文档、存储过程说明、变更脚本、触发器、索引和历史方案。
- 提取当前功能需要的表/字段/链路结论时,在 hr-gateway 项目文档中记录必要摘要(因为业务 SQL 现在写在 gateway 里)。
- 如果实际测试库 schema 与 `D:\projects\DB-Knowledge` 文档不一致,以当前测试库只读探查结果为准,并在相关文档中注明差异。

## hr-gateway 联调

本机开发标准流程:

```bash
# 终端 1:启动 gateway
cd D:/projects/hr-gateway
cp .env.template .env   # 首次,填入 DB / DingTalk / JWT
make dev-gateway

# 终端 2:用 hr-cli 调用
cd D:/projects/hr-cli
hr profile add dev --auth-base-url http://127.0.0.1:18080
hr profile use dev
hr auth +login --dingtalk
hr doctor   # 应返回 gateway /health 响应
hr employee +find --badge P000487
```

不允许 hr-cli 在测试中再走 DB 直连旁路 —— 任何「绕过 gateway」的需求都说明 gateway 缺接口,应在 gateway 侧补,而不是回退架构。
