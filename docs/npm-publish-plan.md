# npm 发布流程

`@wubang9527/hr-cli` 是 hr-cli 的正式分发包。Gitee 仓库保存源码,
预编译二进制由发布流程生成并装入 npm tarball;不支持直接通过 Gitee Git
地址执行 `npm install`。

## 发布前提

- npm 登录账号必须是 `wubang9527`。
- 工作区必须基于最新 `origin/master`,且除本次发布文件外没有其它改动。
- `package.json` 版本尚未出现在 npm registry。
- 发布版本必须创建同名 Git 标签,例如 `v1.0.0-rc.5`。

## 自动化命令

```bash
# 生成 darwin/linux/windows × amd64/arm64 六个平台二进制。
# 版本、提交和构建日期会通过 Go ldflags 注入。
npm run build:npm

# 生成 npm tarball,安装到临时目录,验证文件矩阵及 --version。
npm run test:release

# 发布时 prepublishOnly 会再次运行上述隔离验证。
npm publish --access public
```

`npm run test:release` 成功后会自动清理临时安装目录和测试 tarball。
任一平台文件缺失、安装脚本失败、包版本与二进制版本不一致时都会返回非零状态,
阻止 `npm publish`。

## 完整发布顺序

1. 更新 `package.json` 和 `CHANGELOG.md`。
2. 运行 `go test ./...`。
3. 运行 `npm run test:release`。
4. 提交发布变更。
5. 在提交上创建带说明的标签 `v<package-version>`。
6. 执行 `npm publish --access public`。
7. 推送发布提交和标签到 Gitee。
8. 在干净目录执行最终验证:

   ```bash
   npm install -g @wubang9527/hr-cli@<version>
   npm ls -g --depth=0 @wubang9527/hr-cli
   hr-cli --version
   ```

## 历史说明

- 旧作用域 `@a614594721/hr-cli` 从未发布到 npmjs.org,不要再查询或安装。
- `1.0.0-rc.4` 的 npm 元数据是 rc.4,但包内二进制没有注入版本号,
  `hr-cli --version` 会显示 `0.0.0-dev`;该问题从 rc.5 起修复。
- 旧 `v1.0.0-rc.2` 标签与包元数据不一致,不要移动或复用历史标签;
  后续版本以 npm 版本、发布提交和同名不可变标签三者一致为准。
