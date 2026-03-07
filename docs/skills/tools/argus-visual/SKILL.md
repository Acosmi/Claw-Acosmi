---
name: argus-visual
description: Argus 视觉子 Agent：通过 argus_* 工具进行屏幕感知 + UI 自动化
tools: spawn_argus_agent
---

# Argus 视觉子 Agent

本技能用于**决定何时把任务委托给 Argus**，以及**怎样写出高质量的 `task_brief`**。  
Argus 当前的主路径是 `spawn_argus_agent`，不是让主智能体长时间直接手搓整套 `argus_*` 操作循环。

## 何时使用 Argus（vs browser）

| 场景 | 工具 | 原因 |
|------|------|------|
| 桌面应用（Finder、Terminal、Xcode、系统设置） | `spawn_argus_agent` | 原生 UI 无 CSS 选择器 |
| 多步视觉任务（找按钮、填表、处理弹窗） | `spawn_argus_agent` | 子智能体更适合观察-操作-验证循环 |
| 网页 DOM 自动化 | `browser` | CSS/DOM 定位通常更快更稳 |
| 快速看一眼当前屏幕 | `argus_capture_screen`（直接） | 单步观测无需启动子智能体 |

规则：

- 有明确 DOM / CSS 选择器的网页任务优先 `browser`
- 原生桌面窗口优先 Argus
- 多步视觉任务默认委托 Argus 子智能体

## 委托给 Argus 时，`task_brief` 应写什么

尽量把以下信息写进 `task_brief`，减少子智能体走“全屏截图 -> 全局理解 -> 再猜按钮”的低效路径：

- 目标应用或窗口名
- 已知快捷键、Spotlight 入口或菜单路径
- 要找的准确文本、按钮名或字段名
- 成功标志（看到什么算完成）
- 禁止动作（不要点击删除、不要提交、不要关闭窗口）

好的例子：

- “切到 Terminal，优先用 Spotlight 或现成快捷键打开；找到包含 `build ok` 的最新输出并回报。”
- “在系统设置里打开蓝牙页，优先使用已知导航入口；找到 `AirPods Pro` 是否已连接，不要改设置。”
- “在桌面应用里找到 `Submit` 按钮并点击；如果弹窗出现，先处理弹窗再继续。”

## 优先路径

给 Argus 任务时，优先鼓励它走最短路径：

1. 已知应用入口：先快捷键 / Spotlight / 明确导航入口
2. 已知目标文本：先 OCR / `read_text`
3. 已知按钮或字段：先局部定位 / `locate_element`
4. 只有状态不清时，才做全局截图或场景描述

不要默认把“全屏截图 + 全局理解”写成每一步的硬前置。

## 常见工具族

- 观测：`argus_capture_screen`、`argus_describe_scene`、`argus_read_text`
- 定位：`argus_locate_element`、`argus_detect_dialog`
- 操作：`argus_click`、`argus_type_text`、`argus_press_key`、`argus_hotkey`
- macOS 入口：`argus_macos_shortcut`、`argus_open_url`
- 验证：`argus_watch_for_change`、必要时再次截图

对应的低层专属技能已经拆分到：

- `argus-app-launch`
- `argus-screen-reading`
- `argus-target-locate`
- `argus-ui-actions`
- `argus-verify-retry`
- `argus-high-risk`

上述 6 个子技能已各自补齐首轮的故障树、回滚步骤和验收清单。

## 建议工作流

### 打开或切换应用

- 优先 `argus_macos_shortcut` / `argus_hotkey`
- 如果应用入口明确，优先 Spotlight
- 只有入口不明时，再观察屏幕找应用

### 找信息

- 优先 `argus_read_text`
- 目标文本不明确时，再用 `argus_describe_scene`
- 不要为了“读一行字”先做整屏复杂定位

### 找按钮或字段

- 优先写清按钮文案、区域、相邻文本
- 已知大致位置时，优先局部定位
- 点击后只在状态变化不明显时再截图验证

## 权限与边界

- Argus 需要 macOS 的屏幕录制和辅助功能权限
- Argus 操作真实屏幕，鼠标和键盘对用户可见
- 遇到权限弹窗、系统安全提示或高风险动作时，应停止并回报
- 不要把网页 DOM 任务强行交给 Argus
- 不要把编程任务交给 Argus；它是视觉执行子智能体，不是 coder

## 主智能体与 Argus 的分工

- 主智能体：判断、委托、收敛结果
- Argus：执行多步视觉任务
- 主智能体直控只适合少量单步动作，例如一次截图或一次明确快捷键

## 职责矩阵

- 主智能体可直接做：一次 `argus_capture_screen`、一次明确无歧义的快捷键、一次已知目标的快速打开动作
- 应默认委托 Argus：打开应用后再找信息、按钮定位与点击、表单填写、弹窗处理、需要观察-操作-验证循环的任务
- 网页且有 DOM/CSS 线索：优先 `browser`，不要强行改走 Argus
