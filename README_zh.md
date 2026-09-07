# MeasureTrail / 量迹

<!-- README-I18N:START -->

[English](./README.md) | **汉语**

<!-- README-I18N:END -->

量迹是一个以中文为先、尊重隐私的体重追踪产品，正在重建为现代原生 iOS 应用。

> [!WARNING]
> 项目正在持续实施，尚未达到发布标准；已提交的计划仍是范围与验收依据。

## Status

| 范围 | 当前状态 |
| --- | --- |
| 产品标识 | 已确定 MeasureTrail / 量迹 |
| 仓库 | 已配置 GitHub 与 Gitea 远程仓库 |
| iOS 应用 | iOS 26 SwiftUI 应用已可通过模拟器构建；认证、离线缓存/Outbox、概览备注、历史筛选、账号凭证修改、趋势、冲突处理、导出和自动服务环境配置已实现 |
| 后端 | Go API、SQLite migration、认证、数据接口和 Docker 静态配置已实现 |
| Web 应用 | 为未来阶段预留；尚未实现 |

## Scope

- 记录每日体重，以及可选的腰围和备注。
- 提供趋势、目标、BMI、历史与适合手机查看的洞察。
- 使用用户名密码认证，首次启动引导默认账号；已绑定身份可通过 Apple 登录，并支持未来多客户端同步。暂不开放注册和邮箱找回密码。
- 提供可选的 HealthKit 集成，健康数据的读取和写入始终由用户控制。
- 当前产品范围不包含 CSV 导入和提醒。

## Repository layout

```text
measure-trail/
├── backend/       # Go API、SQLite migration 与 Docker 部署文件
├── ios/           # iOS 26 原生 SwiftUI 应用
├── web/           # Future web client placeholder
├── docs/          # Product plan and development conventions
├── .env.example   # 最小本地开发配置，不包含可用密钥
├── README.md
└── README_zh.md
```

## Documentation

- [实施计划](./docs/plan.md)
- [开发约定](./docs/development.md)
- [API 契约](./docs/api-contract.md)
- [隐私政策草案](./docs/privacy.md)
- [App Store 发布清单](./docs/app-store-checklist.md)

## Legacy data

用户提供的 `slimtrack.db` 只作为仓库根目录中的本地迁移输入。它受 Git 忽略规则保护，不能被移动、修改或提交。其结构和迁移路径会在专门的迁移阶段验证。

## Development

iOS 应用自动选择服务器：Debug 模拟器连接 `http://localhost:21000`，真机及 Release 连接 `https://measure-trail.ydfk.site`，地址配置在 Xcode target Build Settings 的 `MEASURETRAIL_API_BASE_URL`。后端首次启动默认创建 `admin` / `111111`，也可由环境变量注入；用户可在“我的”修改用户名和密码。Docker 镜像由 Go 单进程同时提供 `/api` 与未来 Vue/React 的静态构建产物，并提供生产 Compose 示例与安全环境生成脚本。最终 HTTPS 部署、HealthKit 读写、真机通过 Apple 登录、跨设备冲突和 App Store 发布门禁仍未完成。
