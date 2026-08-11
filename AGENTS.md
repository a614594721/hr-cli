# Project Rules

## 架构

hr-cli 已从「胖客户端 + 直连 DB」迁移为「瘦客户端 + hr-gateway」架构。详见 `docs/hr-cli-architecture-credential-isolation.md`。

- **hr-cli**(本仓库):纯 HTTP 客户端,不持有 DB 凭证,不实现 perm/scope/audit。
- **hr-gateway**(`D:\projects\hr-gateway`):承担 DingTalk OAuth/JWT、操作者身份、CLI 动作授权、preview/apply、审计入口和遗留 gateway 能力。
- **hr-openapi-service**(`D:\projects\hr-openapi-service`):承接已迁移的员工读取等可复用 HR API，并执行用户模式数据范围过滤。
- **bi_ehr**:OAuth broker 已迁出,bi_ehr 回到「业务系统」本职。

**红线**:不允许 hr-cli 走 DB 直连旁路。任何「绕过 gateway」的需求都说明 gateway 缺接口,应在 gateway 侧补,而不是回退架构。

工程结构沿用 [larksuite/cli](https://github.com/larksuite/cli) 的命令组织、配置管理、错误 envelope 思路。本地参考源码 `D:\projects\lark-cli`,不要再访问 GitHub。

## 数据库

hr-cli 客户端不持有 DB 凭证。开发或测试需要直接查 DB 时,在 hr-gateway 仓库内操作:gateway `.env` 中的 `DB_*` 指向测试环境(`DB_ENV=test`),可执行查询、写入、迁移、清理。

仅当对话中明确提供生产连接信息,或 `DB_ENV` 不为 `test` 时,才视为生产环境并谨慎处理。

hr-cli 配置只需要 `auth_base_url`,联调流程见下文「hr-gateway 联调」。

## 测试身份

默认操作者为 **吴邦**(HR_ADMIN),所有读/写命令默认以吴邦作为操作者或目标员工。
gateway 虽然存在 impersonate HTTP 接口，但当前 CLI 命令树没有注册
`hr auth impersonate`；除非命令已经实现并通过 `hr auth --help` 验证，否则不要向用户提供该命令。

**不再支持** `HR_OPERATOR_EID=...` 这类本机伪造身份方式:客户端不再读 `HR_OPERATOR_*`,operator 只能由 gateway 颁发的 access_token 解出。

| 姓名 | EID | badge | hr-cli role | scope size | 用途 |
|---|---|---|---|---|---|
| 吴邦 | 94 | P000487 | HR_ADMIN | 8033(白名单全员) | 默认操作者 / 写测试 |
| 曹晓蕾 | 1124 | — | SSC | 不查(旁路) | SSC 旁路验证 |
| 董寰宇 | 1 | P000504 | HRBP | 135(国际HRBP组) | HRBP scope 边界 |
| 王婧 | 684 | — | MANAGER | 108 | MANAGER scope 边界 |
| 邱冠宇 | 426 | — | MANAGER | 8033(PsoRadiationRange 函数白名单) | 不要用于 scope 测试,会假阳性 |

切到董寰宇身份后的 HRBP scope 用例:
- 在范围正向用例:黎子豪(EID `439`,用户产品研发组)。
- 越权负向用例:吴邦(EID `94`),不在董寰宇辐射范围内。

### 写测试约束

- 不得在未获明确授权的前提下用其它真实员工做写操作(`apply --yes`)。如需新增测试员工,先在对话中确认。
- 写操作前必须确认回滚路径;no-op 写测试(如旧值=新值的资料修改)允许直接执行。
- profile-info 写测试沿用 **袁洁**(personal_info `user_id=6711`),no-op 写已审计通过;吴邦在 `personal_info` 无对应行,profile-info 不能用吴邦本人。

## 权限模型

JWT/CLI 动作授权位于 `hr-gateway/internal/auth/` 与 `internal/perm/`；员工、
考勤、审批等已迁移读取的数据范围由 `hr-openapi-service` 执行。**hr-cli 客户端不包含这些权限实现**。

角色优先级(高→低):`HR_ADMIN > SSC > HRBP > MANAGER > SELF`。DB 角色映射:

| `skysecrole.ID` | DB 名称 | hr-cli role |
|---|---|---|
| 181084 | admin-临时 | HR_ADMIN |
| 181090 | SSC | SSC |
| 181089 | HRBP | HRBP |
| 181083 | 一级部门负责人自助 | MANAGER |
| 1001 | 信飞PC-员工自助 | SELF |

- gateway 在签发 access_token 时反查 EID 的角色集合,按优先级取最高写入 JWT claim。
- 目标级 scope:`HR_ADMIN` 与 `SSC` 旁路;其它角色未命中即 `target_out_of_scope`。
- **scope 非实时**:由批跑(`pro_cr_PsoRadiationRangeEidlist`)定时重建,调动后 scope 生效需等下一轮批跑。这点会影响测试预期。
- `employee +find`、考勤读取和审批读取通过 gateway 转发到 HR OpenAPI，上游在 SQL 层加 scope 过滤。
- 客户端要查权限时调 `hr perm explain`；gateway 保持路由契约并由上游返回范围解释。

## 当前测试环境

当前默认 gateway 地址 `http://139.224.32.44:3002` 对应 `2026-08-11` 迁移后的
测试部署。服务运行在 JumpServer 资产 `test-3th-ehr服务`（`172.16.15.133`，
主机名 `qa1-ehr`），不再运行在 `wubang-106`。

hr-cli 本仓库不负责服务端部署。需要查服务状态、日志、重启或发布时：

- 连接规则见 `D:\projects\jumpserver_ssh\AGENTS.md`；
- 部署规则见 `D:\projects\hr-platform-workspace\deploy\qa1-ehr`；
- 不要使用 `ssh wubang-106`，也不要让 hr-cli 获得服务器或数据库凭证。

## 审计

实现在 `hr-gateway/internal/audit/`,**hr-cli 客户端不写审计**。

- 双写:gateway 本机 `.hr-gateway/audit/YYYYMMDD.jsonl` + DB 表 `hr_cli_audit_log`。
- 当前写审计的事件:`transfer.apply.{start,success,failed}`、`profile_info.apply.{start,success,failed}`、`auth.impersonate.start`。
- dry-run 不写审计。

## 数据库业务知识参考

- 涉及数据库表、字段、存储过程、触发器、审批流、考勤、员工、调动、个人资料等业务含义或链路不清楚时,优先参考 `D:\projects\DB-Knowledge`。
- 该目录可作为 HR 数据库业务知识库使用,包括表结构文档、存储过程说明、变更脚本、触发器、索引和历史方案。
- 提取当前功能需要的表/字段/链路结论时,在实际拥有 SQL 的仓库文档中记录必要摘要；已迁移读取通常记录在 hr-openapi-service。
- 如果实际测试库 schema 与 `D:\projects\DB-Knowledge` 文档不一致,以当前测试库只读探查结果为准,并在相关文档中注明差异。

## hr-gateway 联调

本机开发标准流程:

```bash
# 终端 1:启动 gateway
cd D:/projects/hr-gateway
cp .env.template .env   # 首次,填入 DB / DingTalk / JWT
make dev-gateway        # 默认监听 127.0.0.1:18080

# 终端 2:用 hr-cli 调用
cd D:/projects/hr-cli
hr profile add dev --auth-base-url http://127.0.0.1:18080
hr profile use dev
hr auth +login --dingtalk
hr doctor               # 应返回 gateway /health 响应
hr employee +find --badge P000487
```
