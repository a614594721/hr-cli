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

## 数据库业务知识参考

- 涉及数据库表、字段、存储过程、触发器、审批流、考勤、员工、调动、个人资料等业务含义或链路不清楚时，优先参考 `D:\projects\DB-Knowledge`。
- `D:\projects\DB-Knowledge` 可作为 HR 数据库业务知识库使用，包括表结构文档、存储过程说明、变更脚本、触发器、索引和历史方案。
- 不要把 `D:\projects\DB-Knowledge` 中的内容机械复制进本项目；只提取当前功能需要的表/字段/链路结论，并在本项目文档中记录必要摘要。
- 如果实际测试库 schema 与 `D:\projects\DB-Knowledge` 文档不一致，以当前测试库只读探查结果为准，并在相关文档中注明差异。
