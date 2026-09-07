# MeasureTrail App Store 发布清单

> 状态：发布门禁。所有“未完成”项关闭前不得提交审核。

## 账号、隐私与 HealthKit

- [ ] 在“我的”中提供公开可访问的隐私政策链接；同时在 App Store Connect 设置有效的 Privacy Policy URL。
- [ ] 使用真实部署环境逐项填写 App Privacy。按当前代码至少评估：用户名、体重/腰围（Health & Fitness > Health）、自由备注（Other User Content）、以及与账号关联关系；确认均为“App Functionality”且不用于 Tracking。任何新增 SDK 都需要重新评估。
- [ ] 已添加 `PrivacyInfo.xcprivacy`，声明应用功能所需的账号与记录数据、无追踪，以及 `UserDefaults` 的 `CA92.1` 理由；Release 模拟器产物已确认包含该清单与 HealthKit 用途说明，仍须使用最终 Archive 的 Privacy Report 核对 Required Reason API 和任何未来第三方 SDK。
- [ ] 已添加体重和腰围的 HealthKit 读取/明确启用后写入用途说明、隐私清单、锚点同步及防回环标记；仍须真机验证 entitlement、读取和写入授权拒绝/撤销、跨日样本、替换量迹样本及手工记录优先策略。
- [ ] 若 App Store Connect 的 Health、医疗器械或年龄分级问卷适用，按实际功能完成声明；量迹不能声称提供诊断、治疗或医疗建议。

## 账号生命周期

- [ ] 用生产默认账号完成用户名密码登录、刷新、退出，并在未来 Web 中修改用户名和密码后重新登录。
- [ ] 审核“删除账号”：在应用内可发现、明确不可恢复、删除服务器数据和本机缓存，并在真实服务端验证。自动化测试已验证删号后 access token 与 refresh token 失效，认证、资料、记录、同步、导入关联数据级联删除；iOS 会清理全部 SwiftData 缓存、Keychain、HealthKit 锚点与手工写入偏好。
- [ ] 如允许 Sign in with Apple，完成 Apple 服务 ID、Bundle ID、回调/受众配置，并真机验证首次登录与再次登录。
- [ ] 已实现 Apple authorization code 交换、refresh token 的服务端加密存储与删号撤销；仍须用生产 Apple 配置和真机验证首次登录、再次登录及删号撤销。
- [ ] 确认改用另一个自托管服务地址时会要求重新认证，避免把旧服务的会话发送给新地址。

## 后端与运行

- [ ] 用最终 HTTPS 域名部署 Docker Compose，检查证书、HSTS/反向代理、`MEASURETRAIL_PUBLIC_BASE_URL`、默认账号、JWT 密钥与 Apple 配置。
- [ ] 在独立 Docker Compose 项目验证单个 Go 进程提供 API 与 Web 静态文件、健康检查、SQLite 命名卷重启持久化以及在线备份/恢复；仍须在最终 HTTPS 部署上验证登录、记录同步与生产恢复演练。
- [ ] 验证 iOS“服务器连接”只接受 HTTPS 基础地址，且 `/api/health` 成功后才启用登录。
- [ ] 验证旧 SQLite 正式导入前的 dry-run；确认报告后导入数据库记录的默认账号。

## iOS 质量

- [ ] 使用 Release Archive 在 iOS 26 真机测试：登录、退出、删号、离线新增/编辑/删除、恢复联网、跨设备版本冲突与用户选择。
- [ ] 验证浅色/深色、动态字体、VoiceOver、Reduce Motion、Reduce Transparency、长备注与无数据状态。
- [ ] 已添加并运行 iOS XCTest target，覆盖单位换算、记录输入校验、HealthKit API 映射、同步合并策略、Outbox 重试、服务器地址校验，以及旧版本编辑和离线新设备同日创建收到 `409` 后远端版本的本地冲突落库和决策规则；已有首次引导与 HTTPS 地址校验 UI 测试，仍须补充跨设备端到端冲突和其他 UI 测试。后端 HTTP 已验证同一账号两个设备会话的旧版本编辑和离线同日创建均返回 `409`，且旧会话可读取最新远端版本。
- [ ] 真机验证 HealthKit 授权/撤销与真实 Apple 登录；模拟器构建不能替代这两项。
- [ ] 确认 App Icon、截图、名称、描述、关键词、支持 URL、营销 URL（如填写）和审核联系信息均对应实际产品。

## 审核说明模板

```text
量迹是个人身体指标记录工具，不提供医疗诊断、治疗或建议。

审核路径：首次打开后，应用会自动连接构建配置中的 HTTPS 服务；使用审核用户名和密码登录后，可在“概览”记录体重，在“历史”编辑或删除，在“我的”导出或删除账号。HealthKit 为可选权限，拒绝后手动记录仍可使用。

测试服务器：[HTTPS URL]
审核账号：[username]
临时密码：[password]
```

## 依据

- Apple 要求提供 Privacy Policy URL，并准确申报 App Privacy：[Manage app privacy](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy/)。
- 支持创建账号的 App 必须可在 App 内发起删除账号：[Offering account deletion](https://developer.apple.com/support/offering-account-deletion-in-your-app/)。
- HealthKit 权限应在明确用途时请求，并有清楚的隐私政策：[HealthKit HIG](https://developer.apple.com/design/human-interface-guidelines/healthkit)。
- Required Reason API 与隐私清单以最终 Archive 的实际使用为准：[Privacy manifest files](https://developer.apple.com/documentation/bundleresources/privacy-manifest-files)。
