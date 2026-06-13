# hr-cli 方案 v3.1：补 approval / attendance / +login 协议

> 本文档是 `hr-cli-plan-lark-v3.md` 的增量补丁，不替换 v3。
> 写作日期：2026-06-13。

## 0. 为什么需要 v3.1

v3 在命令域上漏了用户最初要求的两块能力：

- **审批流（approval）**：v3 命令树里没有 `hr approval`。
- **打卡（attendance）**：v3 命令树里没有 `hr attendance`。

另外 v3 的 `+login` 留白，没说清协议。v3.1 把这三件事补齐，并列出本次需要追加的 DB probe 清单。

业务安全闭环（preview/apply、env-gate、字段权限、审计、脱敏）一律沿用 v3，不重述。

## 1. +login 鉴权协议 v1：钉钉扫码 + 本地 Auth Service

### 1.1 拓扑

```
HR CLI  ──HTTP──▶  Auth Service (127.0.0.1:8787)  ──OAuth──▶  钉钉开放平台
   ▲                      │
   │ session token        │ unionid → eid (employee_dingding)
   └────── keychain ──────┘
```

- HR CLI 不直接和钉钉通信，DingTalk app secret 只在 Auth Service。
- Auth Service 是本地 127.0.0.1 单进程服务。V1 建议同一个 Go 二进制，子命令 `hr auth-server start` 起。
- session token 存 OS 凭证管理器（Windows Credential Manager / macOS Keychain），配置文件里不出现。

### 1.2 登录流程

```
hr auth +login
  1. CLI  POST /auth/login/start
  2. Auth Service 走钉钉扫码 OAuth 流程，返回 {login_id, qrcode_url, expires_at}
  3. CLI 在 stderr 打印二维码 / 链接（不污染 stdout）
  4. CLI 轮询 GET /auth/login/poll?login_id=...
        → status: pending | ok | expired | denied
  5. 钉钉回调到达 Auth Service 后：
        a. 拿到 unionid
        b. 查 employee_dingding 映射到 eid
           查不到 → authentication.no_employee_mapping
        c. 颁发 session token，写自身 sessions 表
  6. CLI 把 token 存 keychain，stdout 输出 envelope
```

### 1.3 接口契约（最小集）

| 路径 | 方法 | 说明 |
|------|------|------|
| `/auth/login/start` | POST | 触发扫码，返回 login_id 和二维码 |
| `/auth/login/poll`  | GET  | 轮询登录状态 |
| `/auth/me`          | GET  | 当前 session 对应的 eid / 角色 |
| `/auth/logout`      | POST | 主动登出，撤销 token |

session token 形如 `hrcli_v1_<random>`，7 天过期；V1 不实现刷新，过期重新 `+login`。

### 1.4 V1 简化点

- 不实现 SSO；不实现 token 刷新；不实现多设备并发。
- 二维码失败时提供 `--device-code` 兜底：用户手动打开 Auth Service 给的链接，CLI 仍轮询同一 `login_id`。
- Auth Service 自己的 sessions 表先用 SQLite，避免污染 hrmv9。

## 2. approval 域：全套 CRUD

确认范围：list / get / submit / approve / reject / withdraw 全支持，写操作走 preview/apply。

### 2.1 命令树

```text
hr approval
  +list                  --status pending|approved|rejected|withdrawn
                         --mine | --assigned-to-me | --as-admin
  +get                   --instance-id <id>
  +submit                --template <code> --params @file.json
  +submit-apply          <preview-id>
  +approve               --instance-id <id> --node <node-id> --comment "..."
  +reject                --instance-id <id> --node <node-id> --comment "..."
  +withdraw              --instance-id <id>
  preview show
  preview list
  preview revoke
```

### 2.2 写操作语义

- `+submit` 只产生 preview，不落库；`+submit-apply <preview-id>` 才真正写。和 transfer 一致。
- `+approve / +reject / +withdraw` 是原子单步动作，不走 preview/apply，但仍必须：
  - 默认交互二次确认；Agent 用 `--yes` 显式跳过。
  - 进入事务前重新查 instance 当前状态。状态被别人改过 → `validation.stale_state`。
  - 当前 operator 不在节点 candidate 列表 → `authorization.not_approver`。

### 2.3 权限矩阵

| 命令 | 默认权限 | 加固 |
|------|---------|------|
| `+list --mine` / `+get`（自己提交的） | 本人 session | — |
| `+list --assigned-to-me` | 本人 session | — |
| `+submit*` / `+withdraw` | 本人 session | 模板级 ACL（部分模板需特定角色） |
| `+approve` / `+reject` | 必须在当前节点 candidate 列表 | 单独审计字段 |
| `+list --as-admin` / 看任意 instance | HR_ADMIN | 审计标记 admin_override=true |

权限判断集中在 `internal/perm`，命令层只声明 action 名：`approval.read.self` / `approval.write.submit` / `approval.write.act` / `approval.admin`。

### 2.4 数据库链路（待 probe）

具体表名、字段、存储过程在 v3.1 中先 TBD。M1.5 probe 后填实。需要核实的内容见 §5.1。

调动（transfer）和审批的关系特别需要核实：当前 v3 的 transfer 走 `eEmployee_Work` + `eSP_EmpChangeStart`，**不经过审批表**。如果实际业务流程是"调动必须先审批"，那 v3 的 transfer 命令需要叠加 approval 前置依赖。这个先 probe 再决定。

## 3. attendance 域：权限范围内查他人

确认范围：本人 + 上下级链 + HRBP 部门 + HR_ADMIN 全量。只读。

### 3.1 命令树

```text
hr attendance
  my             --from <date> --to <date>
  get            --eid <id> --from <date> --to <date>
  list           --dept <id> --from <date> --to <date> --format json|csv|table
  exception      --eid <id> --from <date> --to <date>      # 异常打卡（迟到/早退/缺卡）
  summary        --eid <id> --month 2026-06                # 月度汇总
```

### 3.2 权限模型

- `my`：任何已登录用户。
- `get --eid X`：operator 满足以下任一：
  - operator == X 本人
  - operator 是 X 的直接/间接上级（走 `eEmployee_Work` 上级链，深度上限默认 5，可配）
  - operator 是 X 所在部门的 HRBP
  - operator 是 HR_ADMIN
- `list --dept`：HR_ADMIN 或该部门 HRBP。

action 命名：`attendance.read.self` / `attendance.read.subordinate` / `attendance.read.dept` / `attendance.read.admin`。

### 3.3 输出与脱敏

- `--format json` 给 Agent 和脚本；`--format csv` 给 HR 导表；`--format table` 给人看。
- 默认输出脱敏：设备 ID、IP、定位坐标默认隐藏。`--show-device` / `--show-geo` 显式打开，且仅 HR_ADMIN。
- `--output <path>` 导出文件时：
  - 路径必须在工作目录或 HR CLI 数据目录之内（防越权写）。
  - 写一条审计：导出对象、行数、操作人、时间戳。

### 3.4 不走 preview/apply

attendance 是只读域，preview/apply 不适用。导出操作的安全边界靠路径校验 + 审计实现。

### 3.5 数据库链路（待 probe）

见 §5.2。

## 4. 命令总览增量（叠加 v3 § 4）

在 v3 命令树基础上追加：

```text
hr
  approval
    +list
    +get
    +submit
    +submit-apply
    +approve
    +reject
    +withdraw
    preview show
    preview list
    preview revoke
  attendance
    my
    get
    list
    exception
    summary
```

其余 v3 命令保持不变。

## 5. Probe TODO（M1.5 必做）

v3.1 不假定具体表名 / 字段 / 存储过程。需要把 `probe.py` 扩到第二阶段，输出 `docs/hr-cli-probe-v2.md`，然后回头把 v3.1 里的 TBD 填实。

### 5.1 approval

- [ ] 审批模板表：表名、template_code 列名、模板字段定义（JSON or 关系表）
- [ ] 审批实例表：表名、状态机字段、与 operator/template 的外键
- [ ] 审批节点 / 审批人表：节点状态流转、candidate 列表如何存
- [ ] 存储过程：是否存在 `eSP_ApprovalSubmit` / `eSP_ApprovalApprove` / `eSP_ApprovalReject` / `eSP_ApprovalWithdraw`
- [ ] 调动是否走审批流：`eEmployee_Work` 写入前是否必须有 approved 实例
- [ ] 撤回语义：软删除（status=withdrawn）还是硬删除

### 5.2 attendance

- [ ] 打卡明细表：表名、主键、日期/时间字段、班次字段
- [ ] 班次 / 排班表：异常判定依赖
- [ ] 异常打卡：是直接查明细还是有 summary/view
- [ ] 月度汇总表是否存在
- [ ] 设备 / 定位 / IP 字段命名，决定脱敏白名单

### 5.3 鉴权 ID 字段

- [ ] `employee_dingding` 表里到底有 unionid / userid / openid 哪些
- [ ] 钉钉 OAuth 实际能拿到哪个，决定 Auth Service 用哪个字段做 lookup
- [ ] 是否有现成 sessions 表，还是 Auth Service 自建 SQLite

## 6. 里程碑调整

v3 的 M1–M5 不变，插入：

- **M1.5　probe 第二阶段**：按 §5 执行，更新本文档把 TBD 填实。
- **M3.5　approval / attendance 只读**：在 transfer/profile-info preview 之后，先加 `hr approval +list/+get` 和 `hr attendance my/get/list`。这两个域只读阶段没有数据库写风险，可以先于 transfer apply 上线。
- **M4.5　approval 写**：`+submit / +submit-apply / +approve / +reject / +withdraw` 在 transfer apply 稳定后接入。

## 7. 与 v3 差异速查

| 主题 | v3 | v3.1 |
|------|----|------|
| 命令域 | auth / transfer / profile-info / ehr / perm / db / doctor | + approval（全 CRUD）+ attendance（按权限查他人） |
| `+login` 协议 | 留白 | 钉钉扫码 + 本地 Auth Service + keychain |
| probe 范围 | 仅 transfer / profile-info | + approval / attendance / 鉴权 ID 字段 |
| 里程碑 | M1–M5 | 插入 M1.5 / M3.5 / M4.5 |
| 输出脱敏 | 手机/证件/银行卡/token | + 设备 ID / IP / 定位 |

## 8. 落地建议

1. v3 已有结论保持不变，v3.1 作为增量补丁与 v3 一起作为后续实现基线。
2. 立刻执行 M1.5 probe：扩 `probe.py`，输出 `hr-cli-probe-v2.md`，回填 v3.1 的 TBD。
3. M1（CLI 框架）开工时把 `approval` / `attendance` 命令骨架一起注册，但实现先 stub，正式实现按 M3.5 / M4.5 顺序推进。
4. Auth Service 单独立项：先实现 `/auth/login/start` `/auth/login/poll` `/auth/me` `/auth/logout` 四个端点，钉钉 OAuth 用 mock 跑通后再接真实开放平台。
