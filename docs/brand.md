# 量迹品牌系统

## 已锁定标志

用户于 2026-09-02 选择方向 B：**测量弧线**。

标志由两项动作组成：上方的弧线与四个节点表示持续、可比较的测量；下方的前进曲线表示趋势，而非“必须下降”的体重叙事。它不使用体重秤、腰线、医疗符号或竞赛隐喻，适用于体重、腰围和未来其他个人指标。

主矢量源位于 [`../brand/source/measuretrail-symbol.svg`](../brand/source/measuretrail-symbol.svg)。它是图形主标志；产品文字在界面中使用系统字体排版，不将不可编辑的字体轮廓混入图形标志。

## 色彩 token

| Token | Hex | 用途 |
| --- | --- | --- |
| `mt-ink` | `#123A70` | 主标志、重要操作、浅色界面主文本附近的高对比元素 |
| `mt-blue` | `#2878CF` | 次级数据节点、链接和活动状态 |
| `mt-sky` | `#5A9DE0` | 趋势线、辅助图表和较轻层级 |
| `mt-mist` | `#F7FAFF` | 浅色图标底、页面冷白基底 |
| `mt-night` | `#0C1830` | 深色图标底和深色层级 |
| `mt-highlight` | `#F1805C` | 最近测量/需要注意的单一强调，不能单独表示健康好坏 |
| `mt-on-dark` | `#E9F1FF` | 深色模式的高对比标志与文字 |

业务状态不能只靠这些颜色表达；增减、同步和错误状态在 iOS 阶段还必须有文字、图标与辅助功能标签。

## 字体与排版

- 英文和数字：SF Pro Display / SF Pro Text，使用系统动态字体。
- 简体中文：PingFang SC，随系统动态字体缩放。
- 品牌文字：优先使用 `MeasureTrail` 与 `量迹` 的系统字体排版；不制造独立的装饰性字标。
- 数据：优先等宽数字特性，避免体重与日期在刷新时产生视觉跳动。

## 图标规则

- 图标只表示“测量的连续性”，不承诺减重、诊断或医疗成效。
- 默认、Dark 与 Mono/Tinted 源都维持相同几何。Dark 仅调整对比度；Mono 保持单色结构。
- 所有导出均为无遮罩的 1024×1024 画布。系统与 Icon Composer 负责平台外形和 Liquid Glass，不能在源图中预烘焙圆角、阴影、模糊或渐变。
- 最小视觉检查尺寸为 32 px；低于此尺寸使用单色 `measuretrail-symbol-mono.svg`。

## 文件交付

| 路径 | 内容 |
| --- | --- |
| `brand/source/measuretrail-symbol.svg` | 主图形标志 SVG |
| `brand/source/measuretrail-symbol-mono.svg` | 单色与 Tinted SVG |
| `brand/icon-composer/AppIcon.icon` | 已完成的 Icon Composer 多外观图标文档 |
| `brand/icon-composer/` | 按 z-order 命名的 Icon Composer 分层导入源 |
| `brand/exports/` | Default、Dark、Mono 三张 1024×1024 扁平 SVG 导出 |

`AppIcon.icon` 已在 Icon Composer 中创建，以 Default SVG 作为共享矢量层，并已成功导出 Default、Dark 和 Mono 三种系统渲染。Dark 与 Mono 的独立 SVG/PNG 作为评审参考和未来按模式细调的可编辑源保留。用户已于 2026-09-03 同意 Icon Composer 许可；该图标文档已加入 iOS Xcode 目标。模拟器已完成构建验证，仍须在真机和真实主屏背景完成最终材质调校。
