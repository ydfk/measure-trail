# Docker 部署与恢复

生产部署使用单个 Go 容器和 Docker named volume 中的 SQLite。Go 同时提供 API 和 Web 静态文件；运行镜像中没有 Nginx、Caddy 或 Node Web 服务。不要扩容服务副本，也不要把 SQLite 卷放入网络文件系统。

## 启动

1. 从 `.env.example` 创建受限权限的 `.env`，将 `MEASURETRAIL_ENV` 改为 `production`，替换 JWT secrets，并配置 HTTPS 公网地址。首次启动可通过 `MEASURETRAIL_DEFAULT_USERNAME` 与 `MEASURETRAIL_DEFAULT_PASSWORD` 覆盖默认的 `admin` / `111111`。
2. 在外部反向代理上终止 TLS，并将同一个 HTTPS 地址配置为 `MEASURETRAIL_PUBLIC_BASE_URL`。Go 在同一 origin 下提供 Web 和 `/api`；若另有独立浏览器来源，再通过 `MEASURETRAIL_CORS_ORIGINS` 明确允许，不使用通配符。
3. 执行 `docker compose up -d --build`，再以 `docker compose ps` 和 `docker compose logs -f api` 检查状态。

容器会在打开数据库前拒绝 HTTP 公网地址、JWT 占位值/短密钥、无效默认凭证或部分 Apple 配置；若启用 Apple，还会校验 P-256 PEM 私钥、32-byte 凭据加密密钥（标准或无填充 base64），以及 JWKS、token、revoke 端点均为 HTTPS。修正 `.env` 后重新创建容器。不要尝试直接编辑 named volume 内的 SQLite 文件。

Docker 构建会检查 `web/`：存在 `package.json` 时按 pnpm、Yarn 或 npm 锁文件安装并执行 `build`，然后把 `web/dist` 复制到 `/app/web`；尚无前端工程时使用占位页。运行阶段只执行 `/app/measuretrail-api`。前端使用 Vue 或 React 均可，但必须输出 `dist/` 且提供 `build` 脚本。

## 生产配置示例

仓库提供 `docker-compose.production.example.yml`，默认拉取 `ydfk/measure-trail:latest`。先生成本机私密配置，再启动：

```sh
MEASURETRAIL_PUBLIC_BASE_URL=https://measure-api.example.com \
  ./scripts/generate-production-env.sh
docker compose -f docker-compose.production.example.yml up -d
```

生成脚本创建 `.env.production`，权限为 `600`，其中默认账号密码和 JWT secrets 均为随机值。目标文件已存在时脚本会终止，避免覆盖正在使用的凭证。自定义镜像可在启动命令前设置 `MEASURETRAIL_IMAGE=your-name/measure-trail:1.2.3`；如需使用其他环境文件或宿主机端口，可设置 `MEASURETRAIL_ENV_FILE=/path/to/file`、`MEASURETRAIL_HOST_PORT=22100`。

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
