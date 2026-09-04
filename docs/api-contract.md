# API 契约

## 当前阶段

Phase 2 至 Phase 4 的服务基础设施、认证、资料、记录、增量同步和导出端点已实现；客户端不得依赖尚未声明的字段或路径。

## 基础约定

- API 基地址由 `MEASURETRAIL_PUBLIC_BASE_URL` 配置。
- 业务 API 使用 `/api/v1` 前缀；基础健康检查保留在 `/api/health`。
- OpenAPI 3.1 由 Huma 在运行时提供；3.0 兼容快照在服务端契约脚本生成后提交，供 iOS 使用 Swift OpenAPI Generator。
- 错误使用 RFC 7807 Problem JSON，至少包含 `type`、`title` 与 `status`。
- 受保护操作在 OpenAPI 3.1 与 3.0 快照中声明 `bearerAuth`（JWT）。缺失或无效的 `Authorization: Bearer <access-token>` 会返回 `401` Problem JSON，而不是参数校验错误。
- 浏览器客户端 CORS 来源由 `MEASURETRAIL_CORS_ORIGINS`（逗号分隔）独立配置，未设置时默认使用 `MEASURETRAIL_PUBLIC_BASE_URL`。生产环境只接受 HTTPS origin；预检允许 `GET`、`POST`、`PUT`、`PATCH`、`DELETE` 与 `OPTIONS`，其中 `PUT` 用于按日期创建或更新记录。
- iOS 首次登录前由用户填写自托管 HTTPS 基础地址（不可包含路径、查询参数或账号信息），客户端只有在 `GET /api/health` 成功后才保存并启用认证入口。

## 已实现端点

### `POST /api/v1/measurements/healthkit`

导入某一天由 HealthKit 读取到的体重和可选腰围。该操作要求 Bearer access token；请求带 `recordedOn`、`weightG`、可选 `waistMm`、稳定的 `healthkitUuid` 与 `clientMutationId`。服务端按用户和 `healthkitUuid` 去重；当同日记录源为 `manual` 或 `legacy` 时，返回原记录而不覆盖其内容。只有原本来自 `healthkit` 的记录才会由更新的 HealthKit 样本推进版本。

### `GET /api/health`

该端点检查服务能够执行 SQLite 查询；成功时返回：

```json
{
  "status": "ok",
  "service": "measuretrail-api",
  "version": "0.1.0"
}
```

数据库不可用时返回 `503` Problem JSON。

### 认证与账号

认证端点以 [`../backend/openapi/openapi-3.0.json`](../backend/openapi/openapi-3.0.json) 为跨端稳定快照：包括邮箱注册/验证/重置、密码登录、会话刷新与撤销、Sign in with Apple 一次性 nonce、登录与绑定、以及账号删除。

Apple 登录由服务端校验 Apple 签名、发行方、受众、过期时间和一次性 nonce。生产部署必须配置 `MEASURETRAIL_APPLE_CLIENT_ID`；未配置时端点以 `503` 明确拒绝，不会降级为不验证的客户端登录。

### 资料、记录与统计

- `GET` / `PATCH /api/v1/profile`
- `GET /api/v1/measurements?from=&to=&limit=`
- `GET /api/v1/measurement-changes?cursor=&limit=`
- `PUT /api/v1/measurements/by-date/{date}`
- `GET` / `PATCH` / `DELETE /api/v1/measurements/{id}`
- `GET /api/v1/statistics?range=7d|30d|90d|all`

资料、记录、统计、导出、会话管理、Apple 绑定和账号删除端点均要求 Bearer access token，并始终由服务端从 token 推导用户身份；注册、登录、刷新、邮箱验证和健康检查等公开端点不要求该 token。记录采用克和毫米作为存储单位；编辑与删除携带 `expectedVersion`，并在版本冲突时返回 `409`。客户端收到 `409` 后以 `GET /api/v1/measurements/{id}` 读取当前云端版本，再让用户决定采用云端内容或基于该版本重试本机修改。短暂 SQLite 锁冲突会在服务端有界重试；重试耗尽时资料和记录写端点返回可重试的 `503` Problem JSON，客户端应保留 Outbox，稍后重试而不是提示字段错误。`PUT /api/v1/measurements/by-date/{date}` 只用于尚无服务端 ID 的新建：同日非删除记录已经存在且 mutation id 未处理时必须返回 `409`，不得通过 `PUT` 静默覆盖另一设备的更新；已同步记录的编辑必须使用 `PATCH`。

### 增量同步

首次同步不带 `cursor`。服务端响应中的 `nextCursor` 是不透明值，客户端只能在已将该页的所有变更持久化到本地后保存它；使用该游标继续请求直到 `hasMore` 为 `false`。

软删除记录同样会出现在变更流中，并带有非空 `deletedAt`。客户端应据此删除或标记本地项，而不是把它重新上传。由于同一记录在一个分页窗口中可能有多个变更事件，客户端按记录 ID 和 `version` 保留最新状态即可。
