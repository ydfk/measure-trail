# 开发约定

本仓库按 [`plan.md`](./plan.md) 的阶段实施。iOS 模拟器应用和 Go API 均已实现并有本机测试；示例配置仍不包含可部署的密钥、SMTP、Apple 或 HTTPS 反向代理设置，不能直接用于生产。

## 范围与阶段

- 只实现已获确认的范围：iOS 26 原生客户端、Go 服务端与 Docker 部署；`web/` 仅预留未来实现。
- 开始每个阶段前，先复核计划中该阶段的目标、文件范围与验收标准；不提前实现后续阶段的功能。
- 业务数据和迁移规则以契约、迁移与测试为准，不能为迁就界面而改变既有 SQLite 数据的语义。

## Git 与变更

- 远程仓库建立后，所有 Git 修改前先同步远程；只在可快进时拉取，避免覆盖他人工作。
- 保留无关的未提交改动；不使用破坏性 Git 命令恢复或清理工作区。
- 每次实现都应运行与风险相称的格式检查、单元测试或构建，并明确区分已验证与待设备、Docker 或生产环境验证的部分。

## 数据与密钥

- `slimtrack.db` 是用户提供的旧数据，仅用于后续迁移验证；它及其 WAL/SHM 文件不得移动、修改或提交。
- `.env`、本机工具配置、SQLite 运行数据、备份和日志均受 `.gitignore` 保护；提交前仍须检查暂存内容。
- `.env.example` 只表达配置名称与安全边界，禁止填写真实密钥、Apple 私钥、SMTP 密码或个人健康数据。

## 本地 API

- 从 `.env.example` 复制 `.env` 后，保持 `MEASURETRAIL_ENV=development`，并替换两个 JWT secret 占位值。
- 可在 `backend/` 中加载 `.env` 后运行 `go run ./cmd`，也可在仓库根目录运行 `docker compose up --build api`；Compose 会读取 `.env` 中的运行环境，不会将开发配置强制覆盖为生产配置。
- 本地 HTTP origin 仅在开发环境允许；生产环境仍要求公网地址和所有 CORS origin 使用 HTTPS。

## iOS 服务环境

- Debug 模拟器默认连接 `http://localhost:21000`；Debug 真机及 Release 默认连接 `https://measure-api.ydfk.site`，登录页无需填写服务器。
- 构建参数 `MEASURETRAIL_API_BASE_URL=https://测试服务域名` 可覆盖打包地址；Debug 也支持同名 Scheme 环境变量，Release 忽略运行时环境覆盖。
- 仅 Debug 允许 localhost、回环或 `.local` 地址的 HTTP；真机连接本地开发机时通过 Scheme 设置开发机的 `.local` 地址，首次连接需允许本地网络。Release 必须使用 HTTPS。旧版 UserDefaults 手填地址不再参与选择。
- 注册默认关闭，开发注册测试需显式设置 `MEASURETRAIL_REGISTRATION_ENABLED=true`；现有账号登录不受影响。

## 代码约定

- Go 与 Swift 代码使用清晰、规模受控的模块；注释使用简体中文，仅解释不能从代码直接看出的意图。
- API 是未来 iOS、Web 与 Android 的共享边界。服务端变更需同步 OpenAPI 契约、客户端生成代码或适配层以及对应测试。
- iOS 界面优先采用系统组件与可访问性语义；Liquid Glass 仅在能强化层级与导航时使用，不为视觉效果牺牲可读性。

## 旧库 dry-run

旧 `slimtrack.db` 只能通过只读工具审计。运行 `cd backend && go run ./cmd/legacy-import --source ../slimtrack.db --dry-run` 会输出 SHA-256、行数、日期范围和不含备注正文的数据质量报告。它不会写入、锁定、导入、迁移或删除源数据库。

只有用户确认 dry-run 报告且提供已验证的目标账号邮箱后，才能运行 `go run ./cmd/legacy-import --source ../slimtrack.db --owner-email user@example.com`。正式导入会在一个事务内校验账号、SHA 去重和所有日期冲突；任一冲突都会回滚整批写入。
