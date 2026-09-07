# 开发约定

本仓库按 [`plan.md`](./plan.md) 的阶段实施。iOS 模拟器应用和 Go 服务均已实现并有本机测试；示例配置仍不包含可部署的 JWT、Apple 或 HTTPS 反向代理设置，不能直接用于生产。

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
- `.env.example` 只表达配置名称与安全边界，禁止填写真实密钥、Apple 私钥或个人健康数据。

## 本地 API

- 从 `.env.example` 复制 `.env` 后，保持 `MEASURETRAIL_ENV=development`，并替换两个 JWT secret 占位值。
- 可在 `backend/` 中加载 `.env` 后运行 `go run ./cmd`，也可在仓库根目录运行 `docker compose up --build api`；Compose 会读取 `.env` 中的运行环境，不会将开发配置强制覆盖为生产配置。
- 本地 HTTP origin 仅在开发环境允许；生产环境仍要求公网地址和所有 CORS origin 使用 HTTPS。
- 服务首次运行确保默认账号存在。默认值为 `admin` / `111111`，可由 `MEASURETRAIL_DEFAULT_USERNAME` 和 `MEASURETRAIL_DEFAULT_PASSWORD` 在首次创建前覆盖。后续启动不覆盖数据库中已修改的凭证。

## iOS 服务环境

- 默认地址配置在 `ios/MeasureTrail.xcodeproj/project.pbxproj` 的 MeasureTrail target Build Settings：Debug 的 `MEASURETRAIL_API_BASE_URL` 为正式 HTTPS 地址，并通过 `[sdk=iphonesimulator*]` 条件改为 `http://localhost:21000`；Release 固定为 `https://measure-api.ydfk.site`。
- `ios/Configuration/Info.plist` 将该构建参数写入 `MeasureTrailAPIBaseURL`，`ios/MeasureTrail/App/AppConfiguration.swift` 负责读取和安全校验。Debug 也支持同名 Scheme 环境变量，Release 忽略运行时环境覆盖。
- 临时测试环境可在构建命令末尾传入 `MEASURETRAIL_API_BASE_URL=https://测试服务域名`。若需要长期增加 Staging，应在 Xcode 中新增 Staging Build Configuration，并为同名参数配置对应 HTTPS 地址。
- 仅 Debug 允许 localhost、回环或 `.local` 地址的 HTTP；真机连接本地开发机时通过 Scheme 设置开发机的 `.local` 地址，首次连接需允许本地网络。Release 必须使用 HTTPS。旧版 UserDefaults 手填地址不再参与选择。
- iOS 使用用户名和密码，不显示邮箱、注册或邮箱找回入口；Apple 仅作为已绑定身份的次要登录方式。

## Web 静态文件

- 本地 Go 默认从 `../web/dist` 提供静态文件，可通过 `MEASURETRAIL_WEB_ROOT` 覆盖；目录或 `index.html` 不存在时只提供 API。
- 未来 Vue/React 工程必须由 `build` 脚本输出到 `web/dist`。未知前端路由回退到 `index.html`，`/api/*` 和 `/openapi.json` 不参与 SPA 回退。
- Docker 可以在构建阶段使用 Node，但运行镜像只包含 Go 二进制和 `dist` 静态文件，不运行独立 Web 服务器。

## Docker 生产示例

- 运行 `MEASURETRAIL_PUBLIC_BASE_URL=https://你的域名 ./scripts/generate-production-env.sh` 生成权限为 `600` 的 `.env.production`。脚本会生成随机默认密码和两条 JWT secret，且拒绝覆盖已有文件。
- 运行 `docker compose -f docker-compose.production.example.yml up -d` 使用 Docker Hub 镜像启动单实例服务。可通过 shell 变量 `MEASURETRAIL_IMAGE=用户名/measure-trail:版本` 覆盖镜像。
- `.env.production` 包含实际凭证并受 Git 忽略；不要复制回 `.env.example` 或提交到仓库。

## 代码约定

- Go 与 Swift 代码使用清晰、规模受控的模块；注释使用简体中文，仅解释不能从代码直接看出的意图。
- API 是未来 iOS、Web 与 Android 的共享边界。服务端变更需同步 OpenAPI 契约、客户端生成代码或适配层以及对应测试。
- iOS 界面优先采用系统组件与可访问性语义；Liquid Glass 仅在能强化层级与导航时使用，不为视觉效果牺牲可读性。

## 旧库 dry-run

旧 `slimtrack.db` 只能通过只读工具审计。运行 `cd backend && go run ./cmd/legacy-import --source ../slimtrack.db --dry-run` 会输出 SHA-256、行数、日期范围和不含备注正文的数据质量报告。它不会写入、锁定、导入、迁移或删除源数据库。

用户确认 dry-run 报告后，可运行 `go run ./cmd/legacy-import --source ../slimtrack.db`，正式导入默认归属数据库记录的默认账号。若要导入其他已有账号，增加 `--owner-username other-user`。正式导入会在一个事务内校验账号、SHA 去重和所有日期冲突；任一冲突都会回滚整批写入。
