# App 信息展示、账号设置与镜像发布计划

## 范围

- 概览在最近一次记录中展示备注；无备注时给出明确但安静的空状态。
- 历史支持按全部、近 30 天、有腰围和有备注筛选，并支持备注搜索；列表直接展示体重、腰围、备注和同步状态。
- “我的”增加登录凭证页面，读取当前用户名并允许验证当前密码后修改用户名和/或密码；成功后清除已失效会话并要求重新登录。
- 增加生产 Docker Compose 示例和安全环境文件生成脚本，继续由单个 Go 进程提供 API 与 Web 静态文件。
- 明确 Debug 模拟器、Debug 真机和 Release 的 API 地址配置位置及覆盖规则。
- GitHub Actions 暂时只在 `v*.*.*` 标签推送时构建并推送 Docker Hub 镜像，不执行 iOS、Go CI 或 GitHub Release。

## 实施步骤

1. 调整概览和历史 SwiftUI 信息层级，确保筛选后的删除操作使用筛选结果索引。
2. 扩展 iOS APIClient 并新增账号凭证页面，接入后端 `GET/PATCH /api/v1/account/credentials`。
3. 增加筛选逻辑测试和凭证界面可访问性标识，执行 iOS 单元与 UI 测试、Release 构建。
4. 增加 `.env.production` 生成脚本与生产 Compose 示例，验证生成文件权限、Compose 配置和容器启动。
5. 将现有工作流替换为仅发布 Docker Hub 的版本标签工作流，并用本地 YAML 解析及 Docker 构建验证。
6. 更新开发、部署和 README 文档，记录各环境 API 地址与 Docker Hub Secrets。

## 验收

- 最新备注可在概览直接读到，历史列表不进入详情也能看到腰围和备注。
- 历史筛选、搜索、空结果和删除映射正确。
- 用户可在“我的”修改用户名或密码，当前密码必填，两次新密码必须一致；成功后返回登录页。
- 生产示例不包含真实 secret，生成脚本默认拒绝覆盖并创建权限为 `600` 的环境文件。
- Debug 模拟器默认 `http://localhost:21000`，Debug 真机和 Release 默认 `https://measure-trail.ydfk.site`；构建覆盖方式有文档说明。
- `v1.2.3` 或预发布版本标签可生成 Docker Hub semver 标签；稳定版额外生成 `latest`。

## 验证结果（2026-09-07）

- iPhone 17 Pro Max / iOS 26.5 模拟器下 29 个单元测试和 2 个 UI 测试通过；无签名 Release 模拟器构建通过。
- Go 全量测试和 `go vet ./...` 通过；`git diff --check` 通过。
- 环境生成脚本通过语法、文件权限、随机值长度和拒绝覆盖检查；生成文件权限为 `600`。
- `linux/amd64` 镜像按发布工作流参数构建成功；生产 Compose 示例隔离启动后，静态首页、健康接口和随机初始密码登录均返回 200，容器内仅运行 Go 进程，测试容器及卷已清理。
- Docker 发布工作流通过本地 YAML 解析和版本标签正反例检查；未向 Docker Hub 推送，因为仓库 Secrets 和版本标签属于远端发布步骤。
