# Docker 部署与恢复

生产部署使用单个 API 容器和 Docker named volume 中的 SQLite。不要扩容 API 副本，也不要把 SQLite 卷放入网络文件系统。

## 启动

1. 从 `.env.example` 创建受限权限的 `.env`，将 `MEASURETRAIL_ENV` 改为 `production`，替换 JWT secrets，配置 HTTPS 公网地址与 SMTP。生产环境不能使用 `MEASURETRAIL_MAIL_MODE=log`。
2. 在反向代理上终止 TLS，并只将 HTTPS API 地址配置为 `MEASURETRAIL_PUBLIC_BASE_URL`。若未来 Web 客户端使用独立域名，在 `MEASURETRAIL_CORS_ORIGINS` 以逗号分隔的 HTTPS origin 列表明确允许它；不要使用通配符，也不要把 Web 地址填入 API 公网地址。
3. 执行 `docker compose up -d --build`，再以 `docker compose ps` 和 `docker compose logs -f api` 检查状态。

容器会在打开数据库前拒绝 HTTP 公网地址、JWT 占位值/短密钥、不完整 SMTP 或部分 Apple 配置；若启用 Apple，还会校验 P-256 PEM 私钥、32-byte 凭据加密密钥（标准或无填充 base64），以及 JWKS、token、revoke 端点均为 HTTPS。修正 `.env` 后重新创建容器。不要尝试直接编辑 named volume 内的 SQLite 文件。

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

## 注册开关

`MEASURETRAIL_REGISTRATION_ENABLED` 默认 `false`，邮箱注册及 Apple 首次自动开户均返回 `403`，已有账号可继续登录与重置密码。部署新后端后生效；如需重新开放，显式设为 `true` 并重启服务。
