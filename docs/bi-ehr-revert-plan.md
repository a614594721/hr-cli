# bi_ehr revert plan: hr-cli broker 迁出

> 目的:把 `bi_ehr` 中为 hr-cli 引入的 DingTalk OAuth broker 代码全部撤销,broker 职能由独立仓库 `hr-gateway` 承担。
> 状态:**等待 P3 切默认完成后再合并**(见 hr-cli `docs/hr-cli-architecture-credential-isolation.md` §6 / §9.2)。
> 对应 hr-cli 任务:#4。

## 范围

只撤销 hr-cli broker 相关 commit,**不动**当前 bi_ehr 业务功能。

### 待 revert 的 commit

| Hash | 标题 | 原因 |
|---|---|---|
| `87dccff` | Use browser OAuth for hr-cli DingTalk login | hr-cli broker 浏览器 OAuth 增强 |
| `1810b76` | Add hr-cli DingTalk auth broker | hr-cli broker 主体提交 |

按时间从新到旧 revert(`87dccff` 在前,`1810b76` 在后),避免 reverse-cherry-pick 冲突。

### 待删除文件

- `backend/app/api/routes/hr_cli_auth.py`(整文件,607 行)
- `sql/2026-06-14/权限/20260614_create_hr_cli_auth_tables.sql`

### 待回滚的修改

回滚到 `1810b76^` 的状态:

| 文件 | 改动 |
|---|---|
| `backend/app/main.py` | 去掉 `hr_cli_auth` import 与 `app.include_router(hr_cli_auth.router, prefix="/api")` + `app.include_router(hr_cli_auth.browser_router)` 两行 |
| `backend/app/api/routes/auth.py` | 去掉 `request_dingtalk_json` 的 `headers` 参数支持(回到没有可选 headers 的旧版) |
| `backend/app/core/config.py` | 删除 5 项配置:`hr_cli_auth_enabled`、`hr_cli_auth_public_base_url`、`hr_cli_dingtalk_redirect_uri`、`hr_cli_access_token_expire_minutes`、`hr_cli_refresh_token_expire_days` |
| `backend/app/core/security.py` | 把 `create_token(payload, expire_minutes)` + `create_app_token` 包装函数还原为原来的 `create_app_token(payload)` 单函数 |
| `backend/.env.template` | 去掉 `HR_CLI_*` 段 |
| `.env.docker.example` | 去掉 `HR_CLI_*` 段 |
| `README.md` | 去掉 hr-cli 相关说明 12 行 |

## 数据库

`hr_cli_auth_*` 表(由 `20260614_create_hr_cli_auth_tables.sql` 创建)**保留**,迁移期由 hr-gateway 接管读写,无需 ALTER。

理由(详见 hr-cli 设计文档 §5.4 选项 A):
- 零 schema 变更,迁移期最平滑
- 表名 `hr_cli_*` 与客户端命名一致,无需双仓库 import 同步
- 是否改名 `hr_gateway_*` 待 P4 全切完后再议

## 操作流程

**不能用** `git revert -m` 直接合 master,会留两条 revert commit 干扰主分支历史。

正确做法:

```bash
cd D:/projects/bi_ehr
git fetch origin
git checkout -b revert/hr-cli-broker origin/master

# 反向 cherry-pick,新到旧:
git revert --no-commit 87dccff
git revert --no-commit 1810b76

# 这一步会把上面两条 commit 中改动 auth.py / config.py / security.py / main.py 的部分自动回滚,
# 还会试图删除 hr_cli_auth.py 和 SQL 迁移 + 还原 README / .env 模板。
# 检查 git status,确认范围与本文档一致后:

git commit -m "Revert hr-cli broker (migrated to hr-gateway)"
git push -u origin revert/hr-cli-broker

# 在 GitHub / 内部 GitLab 上发起 PR,reviewer 至少包括:
# - bi_ehr 主仓库 owner
# - hr-cli 架构组(确认 hr-gateway 已上线稳定)
```

**禁止** `git push --force` 重写历史。bi_ehr 是协作仓库。

## 合并时机

按 hr-cli `docs/hr-cli-architecture-credential-isolation.md` §6:

| 阶段 | 状态 | 操作 |
|---|---|---|
| P0 准备 | ✅ 当前 | 仅准备 PR,**不合** |
| P1 只读迁移 | 进行中 | 不合 |
| P2 写迁移 | 待开始 | 不合 |
| P3 切默认 | 待开始 | **合并 revert PR**,broker 流量切到 hr-gateway |
| P4 删除直连 | 待开始 | 已合,no-op |

## 风险

| 风险 | 缓解 |
|---|---|
| revert 后 bi_ehr 部署失败 | revert 仅删 broker,不动业务,部署链路不变;在 staging 先跑 |
| hr-gateway 没起来就合 revert | **PR 描述写明前置条件**:hr-gateway 在生产已上线 + 7 天无故障 + hr-cli 默认走 gateway 已发版 |
| `hr_cli_auth_*` 表数据丢失 | revert PR 不删表,只删 SQL 迁移文件;现有数据完整 |
| Python 版 broker 删除后 hr-cli 旧版本还在用 | 不存在 — npm 包从未 publish 过(见 #11),没有外部使用方 |

## 与 hr-gateway 的协议绑定

合并这个 revert PR 之前,必须确认:

- [ ] hr-gateway 实现的 6 个 OAuth 路由通过端到端测试,且响应字段名与 bi_ehr Python 版 1:1 兼容
  - `POST /api/hr-cli/auth/login/start`
  - `POST /api/hr-cli/auth/login/poll`
  - `POST /api/hr-cli/auth/refresh`
  - `POST /api/hr-cli/auth/logout`
  - `GET /auth/hr-cli/start`
  - `GET /auth/hr-cli/callback`
- [ ] DingTalk 应用的 redirect_uri 已切换到 hr-gateway 的 public URL(原指向 bi_ehr)
- [ ] `hr_cli_auth_login_session`、`hr_cli_auth_refresh_token` 两张表的写入流量从 bi_ehr 切到 hr-gateway 完成,流量旁路一周无异常
