# Docker 部署与恢复

生产部署使用单个 Go 容器和 Docker named volume 中的 SQLite。Go 同时提供 API 和 Web 静态文件；运行镜像中没有 Nginx、Caddy 或 Node Web 服务。不要扩容服务副本，也不要把 SQLite 卷放入网络文件系统。

## 启动

1. 查询 Apple Developer Team ID，并与 Bundle ID `com.ydfk.MeasureTrail` 组成 `TEAMID.com.ydfk.MeasureTrail`。
2. 运行生产配置生成脚本，得到权限为 `600` 的 `.env.production`。
3. 在外部反向代理上为 `https://measure-trail.ydfk.site` 终止 TLS，并转发到 Go 服务的 `21000` 端口。
4. 执行 `docker compose -f docker-compose.production.example.yml up -d`，再以 `docker compose -f docker-compose.production.example.yml ps` 和 `docker compose -f docker-compose.production.example.yml logs -f api` 检查状态。
5. 在 Apple Developer 与 Xcode 中为 App 启用 Associated Domains；服务会在 `/.well-known/apple-app-site-association` 提供 Passkey 所需的 `webcredentials` 声明。

容器会在打开数据库前拒绝 HTTP 公网地址、JWT 占位值或短密钥、无效默认凭证、无效 Passkey 配置以及不完整的 Apple 登录配置。修正 `.env.production` 后重新创建容器。不要直接编辑 named volume 内的 SQLite 文件。

Docker 构建会检查 `web/`：存在 `package.json` 时按 pnpm、Yarn 或 npm 锁文件安装并执行 `build`，然后把 `web/dist` 复制到 `/app/web`；尚无前端工程时使用占位页。运行阶段只执行 `/app/measuretrail-api`。前端使用 Vue 或 React 均可，但必须输出 `dist/` 且提供 `build` 脚本。

## 生产配置示例

仓库提供 `docker-compose.production.example.yml`，默认拉取 `ydfk/measure-trail:latest`。先生成本机私密配置，再启动：

```sh
MEASURETRAIL_IOS_APP_ID=你的TeamID.com.ydfk.MeasureTrail \
  ./scripts/generate-production-env.sh
docker compose -f docker-compose.production.example.yml up -d
```

生成脚本创建部署目录下的 `.env.production`，并设置权限为 `600`。目标文件已存在时脚本会终止，避免覆盖正在使用的凭证。除 Apple Team ID 外，首次部署无需手工生成或复制任何密钥。

### 宿主机目录挂载

官方生产示例使用 Docker named volume，不需要手工处理权限。如果自行改为 `./data:/app/data`、`./log:/app/log` 和 `./backups:/app/backups`，镜像入口会先将这三个目录交给容器内的 `measuretrail` 用户，再降权启动 Go。业务进程不会以 root 运行。

旧版镜像尚未包含该入口时，SQLite 可能因为无法在 `./data` 中创建文件而报告 `unable to open database file: no such file or directory`。可先在服务器执行以下命令修复现有目录，再重新创建服务：

```sh
measuretrail_user=$(docker run --rm --entrypoint sh ydfk/measure-trail:latest \
  -c 'printf "%s:%s" "$(id -u measuretrail)" "$(id -g measuretrail)"')
chown -R "$measuretrail_user" data log backups
chmod -R u+rwX,go-rwx data log backups
docker compose up -d --force-recreate measure-trail
```

### `.env.production` 必需项

| 配置项 | 生产值 | 来源或生成方式 |
| --- | --- | --- |
| `MEASURETRAIL_ENV` | `production` | 生成脚本固定写入。 |
| `MEASURETRAIL_PUBLIC_BASE_URL` | `https://measure-trail.ydfk.site` | 生成脚本默认写入；域名或协议变更时通过同名环境变量覆盖。 |
| `MEASURETRAIL_DEFAULT_USERNAME` | 默认 `admin` | 首次建库时创建的用户名；可在运行脚本前通过同名环境变量覆盖。数据库创建后应在“我的”中修改。 |
| `MEASURETRAIL_DEFAULT_PASSWORD` | 默认随机 24 位 | 生成脚本自动生成并写入文件；也接受同名环境变量注入。数据库创建后应在“我的”中修改。 |
| `MEASURETRAIL_JWT_ACCESS_SECRET` | 随机 32-byte 十六进制值 | 生成脚本通过 `openssl rand -hex 32` 自动生成。 |
| `MEASURETRAIL_JWT_REFRESH_SECRET` | 另一条随机 32-byte 十六进制值 | 生成脚本单独生成，不能与 access secret 相同。 |
| `MEASURETRAIL_PASSKEY_CREDENTIAL_ENCRYPTION_KEY` | 随机 32-byte base64 | 生成脚本通过 `openssl rand -base64 32` 自动生成；必须与数据库一起长期备份。 |
| `MEASURETRAIL_IOS_APP_ID` | `TeamID.com.ydfk.MeasureTrail` | Team ID 在 [Apple Developer Membership details](https://developer.apple.com/help/glossary/team-id/) 中查询；Bundle ID 在 Xcode target 的 Signing & Capabilities 中查询，本项目固定为 `com.ydfk.MeasureTrail`。 |

脚本不再重复写入可推导或由容器固定的配置：CORS origin、Passkey RP ID 和 Passkey origin 都从 `MEASURETRAIL_PUBLIC_BASE_URL` 推导；JWT issuer、audience 和 Passkey 显示名称使用代码默认值；API 端口、SQLite 路径、超时和 Web 静态目录由生产 Compose 固定。[Apple Passkey 文档](https://developer.apple.com/documentation/authenticationservices/supporting-passkeys)要求 relying party 使用服务域名，并为 App 配置 `webcredentials` Associated Domain。

### 可选项

只有未来启用“通过 Apple 登录”时，才在 `.env.production` 追加以下五项；当前用户名、密码和 Passkey 登录不需要它们：

| 配置项 | 去哪里找或如何生成 |
| --- | --- |
| `MEASURETRAIL_APPLE_TEAM_ID` | [Apple Developer Membership details](https://developer.apple.com/help/glossary/team-id/)。 |
| `MEASURETRAIL_APPLE_KEY_ID` | Apple Developer → Certificates, Identifiers & Profiles → Keys；按 Apple 的[创建 Sign in with Apple 私钥](https://developer.apple.com/help/account/capabilities/create-a-sign-in-with-apple-private-key/)流程创建后取得。 |
| `MEASURETRAIL_APPLE_CLIENT_ID` | iOS 原生 App 使用 Bundle ID：`com.ydfk.MeasureTrail`。 |
| `MEASURETRAIL_APPLE_PRIVATE_KEY` | 创建上述 key 时下载的 `.p8` 内容；Apple 只允许下载一次，放入环境文件时将换行写成 `\n`。 |
| `MEASURETRAIL_APPLE_CREDENTIAL_ENCRYPTION_KEY` | 单独执行 `openssl rand -base64 32` 生成，必须和数据库一起备份。 |

Apple 的 JWKS、token 和 revoke 地址已有内置默认值，不需要写入环境文件。若未来 Web 放在不同 origin，才添加 `MEASURETRAIL_CORS_ORIGINS=https://新的前端域名`。自定义镜像、环境文件、宿主机端口和旧库目录属于 Compose 启动参数，可在命令前设置 `MEASURETRAIL_IMAGE`、`MEASURETRAIL_ENV_FILE`、`MEASURETRAIL_HOST_PORT` 或 `MEASURETRAIL_LEGACY_IMPORT_DIR`，不属于 Go 服务的 `.env.production` 必需项。

## 首次上线导入旧数据

旧 `slimtrack.db` 是迁移输入，不是新服务的数据库。将它放在部署目录的 `import/slimtrack.db`；该目录受 Git 忽略，并由生产 Compose 只读挂载到容器的 `/import`。不要把旧库复制到 named volume 或改名为 `/app/data/measuretrail.sqlite`。新服务的正式数据由 Go 写入 `measuretrail-data` named volume。

先创建生产配置和导入目录，再只执行 dry-run：

```sh
mkdir -p import
cp /安全位置/slimtrack.db import/slimtrack.db
chmod 600 import/slimtrack.db
docker compose -f docker-compose.production.example.yml run --rm --no-deps \
  legacy-import --dry-run
```

工具入口先以 root 读取权限为 `600` 的只读挂载文件，将副本放进临时容器，再降权为 `measuretrail` 执行迁移器；目标数据库仍由非 root 用户写入。dry-run 只校验源数据，不连接或验证目标账号，也不输出备注正文。核对源 SHA-256、记录数、日期范围、腰围数和备注数后，先在正式环境成功登录目标账号，确认它是本次导入的归属账号。只有目标账号和 dry-run 报告都确认后，才能执行正式导入；省略 `--owner-username` 时会使用数据库记住的默认账号。正式导入会写入同一个 `measuretrail-data` 卷；相同源文件重复导入、目标日期冲突或账号不存在都会使整批导入失败并回滚。上线后应按“在线备份”步骤立即生成首份备份，再将旧库保留在部署目录之外的加密位置。

历史导入会同时登记增量同步变更。若使用过未包含该修复的旧镜像导入，测量记录虽然已在数据库中，但已经登录的 iOS 客户端可能无法通过游标发现它们。此时停止 API，并为尚未登记的 `source = 'legacy'` 记录补写 `measurement_changes`；补写使用 `NOT EXISTS`，重复执行不会产生重复变更。

## Docker Hub 发布

GitHub Actions 仅在推送 `v*.*.*` 标签时构建并推送 `linux/amd64` 镜像。仓库 Settings → Secrets and variables → Actions 需要配置：

- `DOCKERHUB_USERNAME`：Docker Hub 用户名，也是镜像命名空间。
- `DOCKERHUB_TOKEN`：具有该仓库写入权限的 Docker Hub access token。

例如 `v1.2.3` 会发布 `1.2.3`、`1.2`、`1` 和 `latest`；`v1.2.3-rc.1` 不更新 `latest`。当前工作流不创建 GitHub Release，也不运行 iOS 或独立 Go CI。

## 在线备份

`docker compose exec api /app/scripts/backup-sqlite.sh` 使用 SQLite online backup API 写入 `/app/backups` 卷，随后执行完整性检查并生成 SHA-256 文件。备份不会复制正在写入的 WAL 主文件。

将备份复制到卷外的加密位置；备份中包含个人健康数据，不能提交到 Git 或公共对象存储。

## 恢复演练

1. 先运行备份并记录 SHA-256。
2. `docker compose stop api`，确认容器已经停止，避免恢复过程与 SQLite/WAL 并发。
3. 执行 `docker compose run --rm --no-deps -e MEASURETRAIL_RESTORE_CONFIRMED=YES --entrypoint /app/scripts/restore-sqlite.sh api /app/backups/measuretrail-YYYYMMDDTHHMMSSZ.sqlite`。
4. `docker compose up -d api`，检查 `/api/health`、用户登录与记录数量。

恢复命令必须通过 `docker compose run -e` 显式将确认变量传入临时容器。它会在校验备份完整性和 SQLite 完整性后，清理已停止服务留下的同数据库 WAL/SHM 侧车文件，再原子替换主文件；因此必须先确认 API 已停止。先在副本环境演练，确认数据后再处理生产环境。

## 升级与回滚

先执行在线备份，再构建新镜像并运行 `docker compose up -d --build`。迁移在启动事务中执行；若启动失败，保留日志与备份，不要通过修改已应用 migration 文件来回滚。回滚应用镜像前，应确认新 migration 对旧二进制兼容；否则从已验证备份恢复。

## 默认账号与凭证

公开注册、邮箱验证和邮箱找回端点均不存在。服务首次启动创建默认账号，默认用户名 `admin`、密码 `111111`；生产环境应在首次启动前用环境变量替换。创建完成后，数据库通过 `default_user_id` 记住该账号；用户以后在 Web 修改用户名或密码时，容器重启不会恢复旧值或再创建一个 `admin`。

Apple 登录只接受已经绑定到现有账号的身份，不能自动创建用户。未来 Web 客户端通过 `GET` / `PATCH /api/v1/account/credentials` 读取和修改用户名/密码。

Passkey 是现有账号的附加登录凭据。用户先用用户名和密码登录，再在“我的”中添加一个或多个 Passkey；之后可直接通过 Passkey 获取同一套 access/refresh token。服务端加密保存凭据内容，只用 SHA-256 摘要索引凭据 ID。注册和登录挑战保留五分钟并且只能使用一次。

`MEASURETRAIL_PASSKEY_CREDENTIAL_ENCRYPTION_KEY` 必须随数据库一起备份并长期保持不变；更换或丢失该密钥会导致已登记 Passkey 无法读取，需要用户用密码登录后重新登记。
