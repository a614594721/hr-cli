# npm 1.0.0-rc.1 下线 + 1.0.0-rc.2 发布计划

> 对应 hr-cli 任务:#11。
> 状态:**1.0.0-rc.1 从未实际发布到 npmjs.org**(2026-06-14 14:19 UTC `npm view` 返回 404)。原方案大幅简化。

## 现状

```bash
$ npm view @a614594721/hr-cli time
npm error 404 Not Found - GET https://registry.npmjs.org/@a614594721%2fhr-cli
```

包名在 `package.json` 里声明,GitHub Release 也打了 tag,但没有任何 `npm publish` 历史。

## 简化后的步骤

不需要 unpublish,不需要 deprecate,只需在架构迁移完成后第一次发布:

1. **P4 完成后**,即:
   - hr-gateway 在生产稳定运行
   - hr-cli `--via-gateway` 已反转为默认
   - `internal/{db,perm,audit,capability,preview}` 已从客户端删除
   - `cmd/db.go` 已删除

2. 更新版本号:
   ```bash
   cd D:/projects/hr-cli
   # 改 package.json: "version": "1.0.0-rc.2"
   ```

3. 验证 `scripts/install.js` 与新二进制下载链路一致:
   - GitHub Releases 资产命名 `hr-cli-1.0.0-rc.2-{darwin|linux|windows}-{amd64|arm64}.{tar.gz|zip}`
   - `checksums.txt` 已 sign

4. 打 tag 并推送:
   ```bash
   git tag v1.0.0-rc.2
   git push origin v1.0.0-rc.2
   ```

5. GitHub Actions 构建多平台二进制并发布 Release(若已配置)。手动情况:
   ```bash
   make release  # 或本地 cross-compile
   gh release create v1.0.0-rc.2 ./dist/* --notes-file CHANGELOG.md
   ```

6. 首次 npm publish(需要 npm login + 2FA):
   ```bash
   npm whoami           # 确认账号
   npm publish --access public
   ```

7. 验证:
   ```bash
   # 干净环境
   npm install -g @a614594721/hr-cli
   hr-cli doctor       # 应该 ping gateway,而不是连 DB
   ```

## 旧 GitHub Release `v1.0.0-rc.1` 处理

不需要硬删。建议在 Release 描述里加一段:

> ⚠️ **DEPRECATED.** This release shipped a fat-client architecture that
> required end-user MySQL credentials. It is incompatible with the
> server-side hr-gateway architecture introduced in `v1.0.0-rc.2`.
> Please install `v1.0.0-rc.2` or later.

## 对外沟通

由于 npm 没有任何用户:

- 不需要破坏性变更声明
- 不需要 migration guide
- 不需要 changelog 中标注「breaking」(因为没有「previous」)
- README 第一段直接写当前架构,无须解释历史

## 何时执行

按 hr-cli `docs/hr-cli-architecture-credential-isolation.md` §6:

| 阶段 | 状态 | npm 操作 |
|---|---|---|
| P0–P3 | 进行中 | 不发包(client 还在用 DB 直连) |
| P4 完成 | — | 发 1.0.0-rc.2 |
| 生产稳定一周 | — | 升 GA → 1.0.0 |
