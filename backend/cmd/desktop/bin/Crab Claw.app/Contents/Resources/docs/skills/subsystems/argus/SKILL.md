---
name: argus
description: "Argus 视觉子系统：桌面屏幕感知 + UI 自动化，通过 spawn_argus_agent 委托多步视觉任务"
---

# Argus 视觉子系统

## 何时使用 Argus vs Browser

| 场景 | 工具 | 原因 |
|------|------|------|
| 桌面原生应用（Finder、Terminal、系统设置） | `spawn_argus_agent` | 原生 UI 无 CSS 选择器 |
| 多步视觉任务（定位→点击→验证循环） | `spawn_argus_agent` | 子智能体更适合观察-操作-验证循环 |
| 网页 DOM 自动化 | `browser` | CSS/DOM 定位更快更稳 |
| 快速看一眼当前屏幕 | `argus_capture_screen`（直接） | 单步观测无需启动子智能体 |

**规则**: 有 DOM/CSS 选择器 → `browser`；原生桌面 → Argus；多步视觉 → 默认委托。

## 委托 task_brief 要写什么

- 目标应用或窗口名
- 已知快捷键、Spotlight 入口或菜单路径
- 要找的准确文本、按钮名或字段名
- 成功标志（看到什么算完成）
- 禁止动作（不要点击删除、不要提交、不要关闭窗口）

## Argus 工具族

### 观测
- `argus_read_text` — 优先用于文本目标
- `argus_describe_scene` — 状态不清时补上下文
- `argus_capture_screen` — 最后手段，保留视觉上下文

### 定位
- `argus_locate_element` — 按文案/相邻文本定位控件
- `argus_detect_dialog` — 检测弹窗/确认框

### 操作
- `argus_click` / `argus_double_click` — 点击目标
- `argus_type_text` — 文本输入
- `argus_press_key` / `argus_hotkey` — 按键/快捷键
- `argus_scroll` — 滚动
- `argus_mouse_position` — 确认鼠标位置

### 应用入口
- `argus_macos_shortcut` — macOS 快捷指令
- `argus_open_url` — 打开 URL（高风险）
- `argus_run_shell` — 执行 shell 命令（高风险）

### 验证
- `argus_watch_for_change` — 等待界面变化

## 优先路径（避免低效循环）

1. **已知入口** → 快捷键 / Spotlight / 明确导航
2. **已知目标文本** → `read_text` 先读
3. **已知按钮/字段** → `locate_element` 局部定位
4. **状态不清** → 才做全局截图或场景描述

**禁止**: 把"全屏截图 + 全局理解"作为每步的硬前置。

## 操作规范

### 操作前
- 先定位，再点击或输入
- 输入前确认焦点在目标字段

### 操作后
- 优先 `watch_for_change` 验证
- 只在变化不明显时补局部读屏
- 未确认结果时不重复执行

### 高风险边界
- `open_url` / `run_shell` 仅在任务明确需要且审批允许时使用
- 被拒绝或阻断时先终止，不做静默重试

## 权限与边界

- 需要 macOS 屏幕录制和辅助功能权限
- Argus 操作真实屏幕，对用户可见
- 遇到权限弹窗或高风险动作时停止并回报
- 不处理编程任务（用 `spawn_coder_agent`）
- 不处理网页 DOM 任务（用 `browser`）

## 主智能体与 Argus 分工

| 角色 | 职责 |
|------|------|
| 主智能体 | 判断、委托、收敛结果 |
| Argus 子智能体 | 执行多步视觉任务 |

主智能体直控仅限：一次截图、一次无歧义快捷键、一次已知目标快速打开。
