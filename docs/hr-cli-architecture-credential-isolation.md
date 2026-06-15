# hr-cli 架构迁移:客户端凭证隔离方案

状态:草稿 v0.1
作者:hr-cli 架构组
日期:2026-06-14
目标版本:`@a614594721/hr-cli@1.0.0-rc.2`(取代已发布的 `1.0.0-rc.1`)

## 1. 背景与问题

### 1.1 当前架构的硬伤

`@a614594721/hr-cli@1.0.0-rc.1` 是「胖客户端 + 直连 DB」形态:用户机器上的 Go 二进制持有完整的 MySQL 连接、库表 schema 知识、所有业务 SQL,以及 `internal/perm`、`internal/scope`、`internal/audit`、`internal/capability` 的完整实现。

这意味着只要 `DB_PASSWORD` 落在用户机器上(无论是环境变量、profile 还是 OS keychain),用户就可以:

1. 绕过 CLI 直接 `mysql -h ... -u ...` 连库,perm / scope 全部失效。
2. 不通过 `transfer +apply` 也能执行 `UPDATE eemployee SET DPID=...`,审计日志里完全没有这条记录。
3. 即使凭证存 keychain,二进制运行时仍要把明文密码读到内存里发给 MySQL,任何 dump 内存或 attach debugger 的人都能拿到。

CLAUDE.md / AGENTS.md 把「鉴权优先 / 高风险双阶段 / 可审计 / 生产保护」列为核心原则,但这些原则只在「用户老老实实通过 CLI 调用」的前提下成立。给了 DB 凭证就等于给了一把可以绕过所有闸门的总钥匙——**安全模型与部署模型互相矛盾**。

对一个号称要给 AI Agent 用的 HR 工具,这个矛盾尤其严重:Agent 出错和被 prompt injection 的概率远高于人,客户端自我约束完全靠不住。

### 1.2 为什么不能直接照搬 lark-cli 的 sidecar

`lark-cli` 用的是同机 sidecar 模式(`sidecar/protocol.go:96-104` 强制 sidecar 地址必须是 loopback 或 docker/lima 等同机别名,跨机部署直接报错)。它解决的核心问题是「**同一台机器上,沙盒环境(容器 / CI runner / Agent 工作区)拿不到 `app_secret`**」,而不是「服务端能否阻止用户绕过 CLI」。

lark-cli 之所以这套就够用,是因为飞书有 OpenAPI 这层天然的服务端权限闸门——哪怕用户拿到 `access_token`,scope / 限流 / 审计也都是飞书后端在管。sidecar 只是把 `app_secret` 这个用来换 token 的特别敏感凭证从沙盒里抽走。

`hr-cli` 处境完全不同:**下层没有 OpenAPI,直接是 MySQL**。如果照搬 sidecar 模式,把 `DB_PASSWORD` 放进同机 sidecar 进程,沙盒里的 hr-cli 通过 sidecar 转发 SQL,问题是:**sidecar 转发 SQL 等于在客户端机器上开了一个无脑 SQL 代理**,用户依然可以构造任意 SQL 让 sidecar 帮忙发——除非 sidecar 自己实现 perm/scope/audit。但那一刻 sidecar 就不再是 sidecar 了,它就是 hr-gateway。

结论:**hr-cli 必须做一个真正的服务端网关,不是同机代理**。

### 1.3 三种方案对比

| 方案 | 客户端持有 | 鉴权位置 | 用户能否绕过 | 工程量 |
|---|---|---|---|---|
| A. 现状直连 DB | `DB_PASSWORD` | 客户端 perm.go | **能,直连 mysql 即可** | 0 |
| B. lark-cli 同机 sidecar | HMAC key | sidecar 进程(同机) | 能,sidecar 不懂 perm 即 SQL 代理 | 中 |
| C. hr-gateway 真后端 | OAuth access_token + URL | gateway 服务端 | 不能 | 大 |

本文档落地方案 C。

## 2. 目标架构

### 2.1 架构总览

```
┌───────────────────────────────┐         ┌──────────────────────────────────┐
│  用户机器 (人 / AI Agent / CI)  │  HTTPS  │  hr-gateway (独立仓库 + 独立部署)  │
│                               │ ──────► │                                  │
│  hr-cli (瘦客户端)              │         │  入站:                            │
│  ─ cmd/* 参数解析               │         │   /api/hr-cli/auth/* (DingTalk OAuth) │
│  ─ internal/output 渲染          │         │   /api/hr-cli/v1/* (业务路由)     │
│  ─ internal/runtime/session      │         │                                  │
│  ─ internal/gateway/client (新)  │         │  内部:                            │
│  ─ access_token (OS keychain)    │         │   ─ 鉴权中间件 (Bearer + refresh) │
│                               │         │   ─ perm + scope                  │
│  本地不再持有:                   │         │   ─ audit (JSONL + DB 双写)       │
│   ✗ DB 凭证                    │         │   ─ capability/* (业务实现)        │
│   ✗ 业务 SQL                    │         │   ─ preview store                 │
│   ✗ perm/scope 实现             │         │   ─ DB 连接池 (DB 凭证仅此持有)    │
│   ✗ audit 实现                  │         │                                  │
│   ✗ preview store              │         └─────────────────┬────────────────┘
└───────────────────────────────┘                           │
                                                            ▼
                                                       MySQL (HR DB)
```

**核心边界**:DB 凭证、业务 SQL、perm/scope 决策、审计写入,全部只在 hr-gateway 进程里。客户端机器上任何文件、任何环境变量、任何内存,都不存在 `DB_PASSWORD`、`DB_HOST`、`DB_NAME`。

### 2.2 客户端职责(收敛后)

仅保留:

- `cmd/*`:cobra 命令树、flag 定义、参数校验
- `internal/output`:JSON / table / csv 等输出渲染
- `internal/runtime/session.go`:profile + access_token 持久化(token 走 OS keychain,session 元数据走 `.hr-cli/session.json`)
- `internal/auth/dingtalk.go`:OAuth 设备流程,与 broker 交互
- `internal/auth/token_store.go`:token 存取、自动刷新
- `internal/keychain/*`:OS keychain 适配
- `internal/gateway/client.go`(**新增**):统一的 HTTP client,封装签名、超时、重试、错误反序列化
- `internal/errs`:错误类型(envelope 反序列化用)

### 2.3 gateway 职责(承接后)

承接当前 hr-cli 客户端的:

- `internal/db/*` → gateway 仓库 `internal/db`
- `internal/perm/*` → gateway 仓库 `internal/perm`
- `internal/audit/*` → gateway 仓库 `internal/audit`
- `internal/capability/*` → gateway 仓库 `internal/capability`
- `internal/preview/*` → gateway 仓库 `internal/preview`(改用 DB 持久化或 redis,不再用本地文件)

承接当前 bi_ehr 中已有的:

- `/api/hr-cli/auth/*` 路由 + DingTalk OAuth broker 实现
- `/auth/hr-cli/*` 浏览器回调路由
- broker 侧 access_token / refresh_token 签发与轮换

> **技术栈说明**:bi_ehr 是 FastAPI (Python) 项目,而 hr-cli 现有 capability 是 Go。hr-gateway 选 Go 重写,理由:① 直接复用 hr-cli 现有 `internal/{db,perm,audit,capability,preview}` 包,迁移成本最低;② 单二进制部署、运维简单;③ DingTalk OAuth broker 部分用 Go 重写约 600 行代码,可控。bi_ehr 的 Python 实现仅作为参考。

## 3. Gateway API 草稿

### 3.1 路由总览

复用 bi_ehr 已占用的 `/api/hr-cli/*` 前缀(迁移后 bi_ehr 该前缀被 revert,由 hr-gateway 独占)。

| 客户端命令 | Gateway 端点 | 鉴权 | 关键 Body |
|---|---|---|---|
| `auth +login --dingtalk`(start) | `POST /api/hr-cli/auth/login/start` | 无 | `{}` → `{login_id, auth_url}` |
| `auth +login --dingtalk`(poll) | `POST /api/hr-cli/auth/login/poll` | 无 | `{login_id, login_secret}` → `{access_token, refresh_token, ...}` |
| `auth +login`(浏览器回调) | `GET /auth/hr-cli/start`、`GET /auth/hr-cli/callback` | DingTalk OAuth | 浏览器跳转 |
| `auth refresh` | `POST /api/hr-cli/auth/refresh` | RefreshToken | `{refresh_token}` |
| `auth +logout` | `POST /api/hr-cli/auth/logout` | Bearer | `{refresh_token?}` |
| `auth +me` | `GET  /api/hr-cli/v1/auth/me` | Bearer | - |
| `auth status --verify` | `GET  /api/hr-cli/v1/auth/verify` | Bearer | - |
| `employee +find` | `POST /api/hr-cli/v1/employee/find` | Bearer | `{name?, badge?, eid?, page?}` |
| `employee get` | `POST /api/hr-cli/v1/employee/get` | Bearer | `{eid \| badge}` |
| `attendance +records` | `POST /api/hr-cli/v1/attendance/records` | Bearer | `{badge \| eid, from, to}` |
| `attendance +summary` | `POST /api/hr-cli/v1/attendance/summary` | Bearer | `{dept, date}` |
| `attendance +exceptions` | `POST /api/hr-cli/v1/attendance/exceptions` | Bearer | `{...}` |
| `approval +tasks` | `POST /api/hr-cli/v1/approval/tasks` | Bearer | `{assignee?, status?}` |
| `approval +task` | `POST /api/hr-cli/v1/approval/task` | Bearer | `{task_id}` |
| `approval +instances` | `POST /api/hr-cli/v1/approval/instances` | Bearer | `{employee?, status?}` |
| `transfer +preview` | `POST /api/hr-cli/v1/transfer/preview` | Bearer | `{badge \| eid, dept, job, effect_date, reason}` |
| `transfer +apply` | `POST /api/hr-cli/v1/transfer/apply` | Bearer + `X-HR-Confirm: yes` | `{preview_id}` |
| `profile-info +preview` | `POST /api/hr-cli/v1/profile-info/preview` | Bearer | `{user_id, sets: {...}}` |
| `profile-info +apply` | `POST /api/hr-cli/v1/profile-info/apply` | Bearer + `X-HR-Confirm: yes` | `{preview_id}` |
| `perm explain` | `POST /api/hr-cli/v1/perm/explain` | Bearer | `{action, target_eid?}` |
| `doctor` | `GET  /api/hr-cli/v1/health` | 无(运行时) | - |
| `db query` | **删除**,客户端不再提供 | - | - |

### 3.2 鉴权与 token

- **access_token**:JWT,服务端签名,默认 30 分钟有效,Body claim 至少包含 `eid / badge / name / role / role_set / issued_at / expires_at`。
- **refresh_token**:不透明字符串(`hrcli_rt_<random>`),服务端持有签发记录,可单独撤销。默认 60 天。
- **客户端持久化**:`access_token` / `refresh_token` 存 OS keychain;`.hr-cli/session.json` 仅存非敏感元数据(EID、name、broker URL、expires_at)。
- **自动刷新**:客户端发请求前若 `expires_at - now < 5 min` 自动调 `/auth/refresh`,加文件锁避免并发刷。
- **operator 身份**:gateway 中间件解 JWT,得到 `(eid, role)`,作为本次调用的 operator。**`HR_OPERATOR_*` 环境变量协议消失**,客户端不再有伪造身份的能力。
- **测试 impersonate**:gateway 提供 `POST /api/hr-cli/v1/auth/impersonate` 接口,**仅 HR_ADMIN 可用,且强制写审计**,签发短 TTL(15 分钟)且 claim 中带 `impersonated_from` 标记的 access_token。CLI 侧 `hr auth impersonate --eid 1` 调用此接口。

### 3.3 错误 envelope

沿用现有契约,不变:

```json
{ "ok": false, "error": { "type": "...", "subtype": "...", "message": "...", "param": "...", "hint": "..." } }
```

新增 / 调整的 error type:

| type | subtype | 触发场景 |
|---|---|---|
| `authentication` | `token_missing` | 没传 Authorization 头 |
| `authentication` | `token_expired` | JWT exp < now,客户端应自动 refresh |
| `authentication` | `token_invalid` | 签名不对、claim 缺失 |
| `authentication` | `refresh_invalid` | refresh_token 过期或被撤销 |
| `network` | `gateway_unreachable` | 客户端连不上 gateway,展示 hint 引导用户 ping `/health` |
| `policy` | `confirm_header_required` | 写命令缺 `X-HR-Confirm: yes` |
| `policy` | `direct_db_disabled` | (P4 后) 客户端检测到 `DB_*` 环境变量主动报错,提示走 gateway |

### 3.4 preview 跨进程持久化

当前客户端 `internal/preview/preview.go` 把 preview 写到 `~/.hr-cli/preview/<id>.json`。迁移到服务端后:

- preview 表 `hr_cli_preview`(字段:`preview_id`、`operator_eid`、`command`、`payload_json`、`created_at`、`expires_at`、`consumed_at`、`consumed_by_eid`)。
- 默认 TTL 24 小时,超时拒绝 apply。
- 同一个 preview_id 只能 consume 一次,apply 成功后写 `consumed_at`,再次 apply 返回 `validation/preview_already_consumed`。
- apply 时重新校验旧值(从数据库读取并对比 preview 中的 old),旧值变化则返回 `validation/stale_preview`。

## 4. 客户端代码增删清单

### 4.1 hr-cli 仓库要删除的代码(P4 阶段)

- `internal/db/*`(整个包)
- `internal/perm/*`(整个包)
- `internal/audit/*`(整个包)
- `internal/capability/*`(整个包,所有 capability 实现)
- `internal/preview/*`(整个包)
- `internal/auth/role.go`(DB 反查角色,改由 gateway 在 token 签发时写入 claim)
- `cmd/db.go`(`hr db query` 命令整体删除)
- `cmd/perm.go` 中直接读 DB 的部分(改为调 gateway)
- `cmd/doctor.go` 中 DB 相关检查(改为 ping gateway `/health`)
- `runtime.Profile` 中 DB 相关字段:`DBEnv`、`DBHost`、`DBPort`、`DBName`、`DBUser`、`CredentialTarget`
- `cmd/config.go` 中 `--db-*` flag 全部删除

### 4.2 hr-cli 仓库要新增的代码

- `internal/gateway/client.go`:统一 HTTP client。
  - 构造函数读 profile 中 `auth_base_url`。
  - `Do(ctx, method, path, body, out)`:自动注入 `Authorization: Bearer <access_token>`、`User-Agent: hr-cli/<version>`、`X-HR-Confirm` 头透传。
  - 错误反序列化:HTTP 4xx/5xx 解 envelope 转成 `*errs.Error`。
  - 401 + `token_expired` 自动调 refresh 一次重试,失败再返错。
  - 默认超时 30s(`transfer/apply` 60s),不重试 5xx(避免重复写)。
- `internal/gateway/types.go`:请求 / 响应 struct 定义,与 gateway 仓库的 types 保持字段对齐(由 `docs/protocol.md` 双方共同维护)。
- `cmd/auth/impersonate.go`:`hr auth impersonate --eid <eid>` 调 gateway 切换身份。

### 4.3 命令实现改造

每个业务命令的 RunE 从「调 capability → 输出」改为「构造请求 → 调 gateway client → 输出」。例如:

```go
// 改造前(D:\projects\hr-cli\cmd\employee.go 大致样式)
func runEmployeeFind(cmd *cobra.Command, args []string) error {
    op, _ := runtime.CurrentOperator()
    if err := perm.Require("employee.find", 0, op); err != nil { return err }
    rows, err := employee.Find(name, badge, eid)
    if err != nil { return err }
    return emit(cmd, rows)
}

// 改造后
func runEmployeeFind(cmd *cobra.Command, args []string) error {
    var resp employeeFindResponse
    err := gw.Do(ctx, "POST", "/api/hr-cli/v1/employee/find",
        employeeFindRequest{Name: name, Badge: badge, EID: eid}, &resp)
    if err != nil { return err }
    return emit(cmd, resp)
}
```

perm / scope / capability / audit 全部消失在客户端代码里。

## 5. bi_ehr revert + hr-gateway 初始化清单

### 5.1 bi_ehr 待 revert 的 commit

经 `git log` 核查,bi_ehr 中为支持 hr-cli 引入的 commit:

- `1810b76` Add hr-cli DingTalk auth broker(主提交,新增 `hr_cli_auth.py`、配置项、main.py 注册路由、`.env` 模板、SQL 迁移)
- `87dccff` Use browser OAuth for hr-cli DingTalk login(在 broker 上叠加浏览器 OAuth 流程)

revert 范围:

- 新增文件直接删:
  - `backend/app/api/routes/hr_cli_auth.py`
  - SQL 迁移 `backend/sql/migrations/20260614_create_hr_cli_auth_tables.sql`(注意:**生产 DB 上对应表暂不删除**,等 hr-gateway 上线后由 gateway 接管这两张表;若已有数据,直接 ALTER 改名为 `hr_gateway_*` 前缀,详见 §5.4)
- 修改的文件回滚到 `1810b76^` 状态:
  - `backend/app/main.py`(去掉 `hr_cli_auth` import 与 router 注册)
  - `backend/app/api/routes/auth.py`(`request_dingtalk_json` 的 `headers` 参数还原)
  - `backend/app/core/config.py`(去掉 `hr_cli_*` 5 项配置)
  - `backend/app/core/security.py`(`create_token` 函数还原为 `create_app_token`,移除 expire_minutes 参数版本)
  - `backend/.env.template` / `.env.docker.example`(去掉 `HR_CLI_*` 段)
  - `README.md`(去掉 hr-cli 相关说明 12 行)

### 5.2 bi_ehr revert 操作建议

**不能用 `git revert`**(会留下两条新 commit 干扰主分支历史)。建议方式:

1. 在 bi_ehr 上开一个 `revert/hr-cli-broker` 分支。
2. `git revert --no-commit 87dccff 1810b76`,合并解冲突,最终一条 commit `Revert hr-cli broker (migrated to hr-gateway)`。
3. 走正常 PR 流程合入 master,由 bi_ehr owner review。

**不要 force push 重写历史**——bi_ehr 是有协作者的业务系统。

### 5.3 hr-gateway 仓库初始化清单

物理路径建议:`D:\projects\hr-gateway`(本机)。

目录骨架:

```text
hr-gateway/
├── cmd/
│   └── hr-gateway/
│       └── main.go                    # 服务入口
├── internal/
│   ├── server/                        # HTTP 路由 + 中间件
│   │   ├── router.go
│   │   ├── middleware_auth.go         # Bearer 验证、token 解码
│   │   ├── middleware_audit.go        # 审计中间件
│   │   ├── middleware_recover.go      # panic 兜底
│   │   └── envelope.go                # 错误 envelope 输出
│   ├── auth/                          # 从 bi_ehr 迁移 + Go 重写
│   │   ├── dingtalk_oauth.go          # DingTalk OAuth (server 侧)
│   │   ├── token.go                   # JWT 签发 / 解析
│   │   ├── refresh.go                 # refresh_token 存取 + 撤销
│   │   ├── login_session.go           # login_id / login_secret 配对
│   │   └── impersonate.go             # HR_ADMIN 切身份接口
│   ├── db/                            # 从 hr-cli 迁
│   ├── perm/                          # 从 hr-cli 迁
│   ├── scope/                         # 从 hr-cli 迁(scope.go 拆出)
│   ├── audit/                         # 从 hr-cli 迁
│   ├── capability/                    # 从 hr-cli 迁
│   │   ├── employee/
│   │   ├── attendance/
│   │   ├── approval/
│   │   ├── transfer/
│   │   └── profileinfo/
│   ├── preview/                       # 从 hr-cli 迁,改为 DB 持久化
│   ├── handler/                       # 每个 capability 一个 handler 文件
│   │   ├── employee.go
│   │   ├── attendance.go
│   │   ├── approval.go
│   │   ├── transfer.go
│   │   ├── profileinfo.go
│   │   ├── perm.go
│   │   └── auth.go
│   └── runtime/
│       └── config.go                  # gateway 配置加载(不是 hr-cli profile)
├── sql/
│   └── migrations/
│       ├── 0001_hr_cli_auth_tables.sql      # 从 bi_ehr 搬,改前缀 hr_gateway
│       └── 0002_hr_cli_preview.sql          # 新增:服务端 preview 表
├── deploy/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── systemd/hr-gateway.service
├── docs/
│   ├── protocol.md                    # 与 hr-cli 双方共同维护的协议
│   └── deployment.md
├── Makefile                           # make dev-gateway 一键起本地
├── .env.template
├── .gitignore
├── go.mod
└── README.md
```

### 5.4 数据库表迁移

bi_ehr 中已经创建的表(见 `20260614_create_hr_cli_auth_tables.sql`)需要决策:

- 选项 A:**保留表名,gateway 接手读写**。零 schema 变更,迁移期最平滑。
- 选项 B:**ALTER 改名为 `hr_gateway_*`**,与 gateway 服务名一致。需要切换窗口,期间登录功能不可用。

推荐 A,等 P4 全切完后是否改名再议。

## 6. 4 阶段迁移路线

| 阶段 | 涉及仓库 | 内容 | 客户端能力 | gateway 能力 |
|---|---|---|---|---|
| **P0 准备** | hr-gateway, bi_ehr | hr-gateway 仓库初始化,从 hr-cli 复制 internal/{db,perm,audit,capability,preview};bi_ehr OAuth broker 端口 Go 重写并迁入 hr-gateway;**bi_ehr revert PR 提出但暂不合**。 | 不变,仍直连 DB | 自检通过,无对外 API |
| **P1 只读迁移** | hr-gateway, hr-cli | gateway 上线 `auth/*` + `employee/*` + `attendance/*` + `approval/*` 只读路由;hr-cli 新增 `internal/gateway/client.go` 与 `--via-gateway` flag,只读命令支持双路径 | 默认仍直连,`--via-gateway` 开关启用新路径 | 只读 API ready |
| **P2 写迁移** | hr-gateway, hr-cli | gateway 上线 `transfer/*`、`profile-info/*`、`perm explain`、`auth impersonate`;hr-cli 写命令支持 `--via-gateway` | 写命令默认仍直连 | 写 API ready,审计在服务端 |
| **P3 切默认** | hr-cli | `--via-gateway` 反转为默认,客户端调用全部走 gateway;**bi_ehr revert PR 此时合入** | 默认走 gateway | 全量(承担线上流量) |
| **P4 删除直连** | hr-cli | 删除 `internal/{db,perm,audit,capability,preview}`、`cmd/db.go`、profile DB flag;**npm 下线 1.0.0-rc.1,发 1.0.0-rc.2** | 纯瘦客户端 | 全量 |

每个阶段都可独立验证、独立回滚,关键节点:

- P0 → P1:gateway `/health`、`/auth/login/start` 自冒烟通过。
- P1 → P2:吴邦(HR_ADMIN)、董寰宇(HRBP)、王婧(MANAGER)三个角色的只读命令双路径输出对比一致。
- P2 → P3:transfer / profile-info 在测试库 dry-run 双路径对比一致;`hr_cli_audit_log` 表中 gateway 写入的审计行格式与原客户端写入兼容。
- P3 → P4:全部业务命令默认走 gateway 一周无故障,bi_ehr revert PR 合入后 broker 路径自然失效。

## 7. CLAUDE.md / AGENTS.md 更新

迁移后必须更新的规则:

- **删除**「环境变量 `DB_HOST`/`DB_USER`/`DB_PASSWORD`/`DB_NAME` 等均指向测试环境,可直接执行查询、写入、迁移、清理等操作」—— 客户端不再读 DB 环境变量。
- **删除**「`HR_OPERATOR_*` 环境变量仅在 `DB_ENV=test` 下被 `CurrentOperator` 接受」—— 客户端不再支持 `HR_OPERATOR_*`,改为 `hr auth impersonate` 走 gateway 接口。
- **改写**「测试员工」段:测试 impersonate 必须经由 gateway,只 HR_ADMIN 可用且写审计;不再有「`HR_OPERATOR_EID=1 ./hr.exe ...`」这类本机伪造身份命令。
- **改写**「权限模型」段:perm/scope/审计实现位置注明在 `hr-gateway/internal/{perm,scope,audit}`,客户端无对应代码;CLI 错误时给的 hint 是「请联系 gateway 管理员」而非「检查 DB 凭证」。
- **新增**「gateway 调试」段:本地开发用 `cd D:\projects\hr-gateway && make dev-gateway`,默认监听 `127.0.0.1:18080`;hr-cli 通过 `hr profile add dev --auth-base-url http://127.0.0.1:18080` 接入。
- **保留**「数据库业务知识参考 `D:\projects\DB-Knowledge`」—— 仍然有用,但读者从 hr-cli 开发者扩展到 hr-gateway 开发者。

## 8. npm 包下线步骤

1. 查发布时间:`npm view @a614594721/hr-cli time`
2. 若 `1.0.0-rc.1` 发布在 72 小时内 → `npm unpublish @a614594721/hr-cli@1.0.0-rc.1`
3. 若超过 72 小时 → `npm deprecate '@a614594721/hr-cli@1.0.0-rc.1' "Architecture migrated to client-server. Install 1.0.0-rc.2 or later — old version requires direct DB access and is no longer supported."`
4. P4 完成后,bump `package.json` version → `1.0.0-rc.2`,更新 `scripts/install.js` 中的二进制下载逻辑(若架构变更),`git tag v1.0.0-rc.2 && git push --tags`,GitHub Actions 构建多平台二进制并发布到 GitHub Releases,`npm publish`。
5. 旧 `v1.0.0-rc.1` 的 GitHub Release 资产保留(下载量低,不影响),但 README 中明确标注「1.0.0-rc.1 已废弃,需要 DB 凭证,不再支持」。

## 9. 风险与未决问题

### 9.1 已通过决策(本次确认)

- ✅ gateway 独立仓库,不合并到 bi_ehr。
- ✅ gateway 用 Go 重写(复用 hr-cli 现有包),bi_ehr Python 实现仅作参考。
- ✅ 客户端不保留 DB 直连 fallback;P4 后彻底删除 `internal/{db,perm,audit,capability,preview}`。
- ✅ 测试也强制走 gateway,本地用 `make dev-gateway` 一键起。
- ✅ `HR_OPERATOR_*` 协议废除,改为 gateway `auth impersonate` 接口。
- ✅ 1.0.0-rc.1 直接 unpublish 或 deprecate,不为旧版做兼容。

### 9.2 实施期风险

- **gateway 单点**:gateway 故障 = 所有 HR 命令停摆。缓解:① gateway 部署至少 2 实例 + 内网 SLB;② DB 管理员保留手工 SQL 应急通道(不在 CLI 内);③ `/health` 由独立监控告警。
- **token 撤销延迟**:JWT 默认无服务端注销能力。缓解:JWT TTL ≤ 30 分钟,refresh_token 服务端可撤销,被撤销后 30 分钟内 access 失效。重大事件可强制把所有现有 refresh token 一次性撤销。
- **preview_id 安全**:preview_id 是 UUID,泄漏即可被人提交 apply。缓解:① preview 与 operator 绑定,apply 时校验 operator EID 与 preview operator 一致;② preview TTL 24h;③ apply 必须 `X-HR-Confirm: yes`。
- **bi_ehr revert 与 hr-gateway 上线时序**:若 revert 先合入而 gateway 还没上线,所有用户无法登录。缓解:bi_ehr revert PR 提出后**挂起**,等 P3 切默认完成且观察一周再合。
- **DingTalk OAuth Go 重写正确性**:bi_ehr Python 版 600 行,涉及签名、回调、token 配对等细节,Go 重写需要 1:1 对照测试。建议:① 在 P0 阶段单独跑端到端 OAuth 流程测试;② 保留 bi_ehr Python 版直至 hr-gateway 上线稳定一周。

### 9.3 待后续决策

- gateway 监控接什么(Prometheus / 内部监控)?
- 内部网络是否需要在 hr-gateway 前再加一层 nginx / API Gateway?
- access_token JWT 签名密钥是否走 vault / KMS?当前 bi_ehr 用 `app_jwt_secret` 环境变量,迁移后是否升级。
- preview_id 跨 gateway 实例共享:DB 持久化够用,还是需要 redis?
- gateway 扩容到多实例时,refresh_token 撤销列表如何同步(DB 共享 vs 内存广播)。

## 10. 接下来的行动

按 §6 阶段推进。本文档作为后续每个 PR 的依据;任何偏离本文档的实现都需要在 PR 描述中说明并更新本文档。

文档维护人:hr-cli 架构组。每完成一个阶段,在 §6 表格对应行后追加 ✅ 与日期。
