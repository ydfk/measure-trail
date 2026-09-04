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
| 仓库 | 已初始化本地 Git 仓库；尚未配置远程仓库 |
| iOS 应用 | iOS 26 SwiftUI 应用已可通过模拟器构建；认证、离线缓存/Outbox、资料、记录、趋势、冲突处理、导出和服务连接设置已实现 |
| 后端 | Go API、SQLite migration、认证、数据接口和 Docker 静态配置已实现 |
| Web 应用 | 为未来阶段预留；尚未实现 |

## Scope

- 记录每日体重，以及可选的腰围和备注。
- 提供趋势、目标、BMI、历史与适合手机查看的洞察。
- 支持账户注册、邮箱密码认证、通过 Apple 登录，以及未来多客户端同步。
- 提供可选的 HealthKit 集成，健康数据的读取和写入始终由用户控制。
- 当前产品范围不包含 CSV 导入和提醒。

## Repository layout

```text
measure-trail/
├── backend/       # Go API、SQLite migration 与 Docker 部署文件
├── ios/           # iOS 26 原生 SwiftUI 应用
├── web/           # Future web client placeholder
├── docs/          # Product plan and development conventions
├── .env.example   # Configuration boundary only; contains no usable secrets
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

iOS 应用会在登录前要求填写自托管 HTTPS 服务地址，并在保存前验证 `/api/health`。模拟器构建与 26 个 XCTest/UI 测试已通过，其中包含旧版本编辑与离线新设备同日记录的本地冲突捕获、退出及删号时完整清理本机缓存、决策规则、HTTPS 服务连接门禁、概览记录数渲染和网络恢复后自动同步；Docker smoke 验证已完成，最终 HTTPS 部署、HealthKit 读写、真机通过 Apple 登录、跨设备冲突与其余端到端 UI 测试和 App Store 发布门禁仍未完成。
