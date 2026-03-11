---
name: browser
description: "通过 CDP 执行网页自动化：ARIA 观察与 ref 交互、视觉标注、截图/GIF、标签页管理与意图级浏览"
tools: browser
metadata:
  tree_id: "ui/browser"
  tree_group: "ui"
  min_tier: "task_light"
  approval_type: "none"
  openacosmi:
    tools: ["browser"]
    emoji: "🌐"
---

# Browser - 浏览器自动化

本文档以 `BrowserController` 与 `executeBrowserTool` 的实际能力为准，覆盖当前浏览器自动化工具支持的全部 action。

## 能力树对齐

- `id`: `ui/browser`
- `kind`: `tool`
- `parent`: `ui`
- `runtime.owner`: `attempt_runner`
- `runtime.enabled_when`: `BrowserController != nil`
- `routing.min_tier`: `task_light`
- `routing.exclude_from`: `task_delete`
- `routing.intent_keywords`: `浏览器自动化`、`网页`、`open website`
- `routing.intent_priority`: `20`
- `perms.min_security_level`: `sandboxed`
- `perms.file_access`: `none`
- `perms.approval_type`: `none`
- `perms.scope_check`: `none`
- `skills.bindable`: `true`
- `policy.policy_groups`: `group:ui`
- `policy.profiles`: `full`
- `policy.wizard_group`: `web`

## 工具接口边界

当前 `browser` 工具只接受以下顶层参数，不要臆造其他字段：

| 参数 | 用于哪些 action | 说明 |
|------|------------------|------|
| `action` | 全部 | 必填，浏览器动作名 |
| `url` | `navigate`、`create_tab` | URL |
| `selector` | `click`、`type`、`wait_for` | CSS selector |
| `text` | `type`、`fill_ref` | 输入文本 |
| `script` | `evaluate` | JavaScript 脚本 |
| `ref` | `click_ref`、`fill_ref` | `observe` 返回的 ARIA ref，如 `e1` |
| `goal` | `ai_browse` | 自然语言目标 |
| `target_id` | `close_tab`、`switch_tab` | 标签页 target ID |

不要传不存在的参数，例如 `profile`、`target`、`headers`、`timeout`。

## 适用范围

- 网页导航、表单填写、按钮点击、搜索、结果核验
- 需要先看页面结构再行动的任务
- 需要在浏览器里完成多步操作的任务
- 需要页面截图、SOM 标注图或操作 GIF 的任务
- 需要在多个标签页之间查看、切换、创建的任务

不适用：

- 原生桌面应用或系统设置界面，这类任务交给 Argus
- 仅需抓取静态网页文本时，优先更轻量的抓取工具
- 系统级文件、窗口、键鼠控制

## 推荐工作流

1. `observe`
先获取 ARIA 树、元素 ref 和页面截图，理解页面结构。

2. `click_ref` / `fill_ref`
优先使用 `observe` 返回的 ref 做交互，比 CSS selector 更稳。

3. `wait_for`
遇到页面加载、动画、延迟渲染时，用 selector 等待目标出现。

4. `screenshot` / `annotate_som`
关键步骤后核验结果；页面复杂或 ARIA 不直观时用 `annotate_som`。

5. `ai_browse`
目标是明确的多步浏览任务时，直接给自然语言目标，让内部循环执行观察、规划和动作。

6. `start_gif_recording` / `stop_gif_recording`
只有在确实需要展示操作过程时才录制。

规则：

- 页面跳转、刷新、内容明显变化后，旧 ref 可能失效，应重新 `observe`
- `click_ref`、`fill_ref` 优先级高于 `click`、`type`
- `click`、`type` 只作为回退方案，不要默认先用 CSS selector

## Action 清单

### 页面导航与读取

| Action | 参数 | 返回形态 | 说明 |
|--------|------|----------|------|
| `navigate` | `url` | 文本 | 打开网页；需要视觉核验时再显式调用 `observe` 或 `screenshot` |
| `get_content` | - | 文本 | 优先返回 ARIA 树和 refs，失败时降级为页面文本 |
| `observe` | - | 多模态：ARIA 文本 + 截图 | 推荐起手动作，返回页面结构、refs 和截图 |
| `screenshot` | - | 多模态：截图 | 单独截当前页 |
| `wait_for` | `selector` | 文本 | 等待 CSS selector 出现 |
| `go_back` | - | 文本 | 浏览器后退 |
| `go_forward` | - | 文本 | 浏览器前进 |
| `get_url` | - | 文本 | 获取当前页面 URL |

### 页面交互与高级能力

| Action | 参数 | 返回形态 | 说明 |
|--------|------|----------|------|
| `click` | `selector` | 文本 | 用 CSS selector 点击；仅作回退 |
| `type` | `selector`、`text` | 文本 | 用 CSS selector 输入；仅作回退 |
| `click_ref` | `ref` | 文本 | 用 ARIA ref 点击元素，推荐 |
| `fill_ref` | `ref`、`text` | 文本 | 用 ARIA ref 填写输入框，推荐 |
| `evaluate` | `script` | 文本/JSON | 执行 JS，要求 `browser.evaluateEnabled=true` |
| `annotate_som` | - | 多模态：标注说明 + 截图 | 输出带编号框的视觉标注截图和交互元素列表 |
| `ai_browse` | `goal` | 文本 | 目标级多步浏览，内部执行 observe -> plan -> act，最多 20 步 |

### 录制与标签页

| Action | 参数 | 返回形态 | 说明 |
|--------|------|----------|------|
| `start_gif_recording` | - | 文本 | 开始录制后续浏览器操作 |
| `stop_gif_recording` | - | 多模态：GIF | 停止录制并返回 GIF |
| `list_tabs` | - | 文本/JSON | 列出当前 page 类型标签页 |
| `create_tab` | `url` 可选 | 文本 | 创建新标签页；未传 URL 时默认 `about:blank` |
| `close_tab` | `target_id` | 文本 | 关闭指定标签页 |
| `switch_tab` | `target_id` | 文本 | 切换到指定标签页；需要视觉核验时再显式调用 `observe` 或 `screenshot` |

## 返回特征

- `observe`、`annotate_som`、`screenshot`、`stop_gif_recording` 一般返回多模态内容
- `navigate`、`click`、`type`、`click_ref`、`fill_ref`、`ai_browse`、`switch_tab` 默认返回文本；需要视觉核验时应显式调用 `observe` 或 `screenshot`
- 截图默认是 JPEG，质量约 `75`
- 内联截图过大时会被自动省略，此时只返回文本结果
- `get_content` 会优先返回 ARIA 树和 refs，不要把它当成纯 HTML 抓取

## 故障分型与恢复

### `not available`

说明浏览器控制器未连接。应引导用户到 Gateway 的浏览器扩展引导页：

`http://127.0.0.1:26222/browser-extension/`

如果扩展持续未连接或仍然无法使用，应继续提示用户前往用户区“自动化”标签中的“浏览器管理”页面，检查扩展状态、连接状态和相关配置信息。

### `transient`

常见于元素还在加载、动画未结束、页面尚未稳定。

处理方式：

- 先 `wait_for`
- 再 `observe`
- 必要时重新执行交互

### `structural`

常见于 selector 错误、ref 失效、页面结构已变。

处理方式：

- 重新 `observe`
- 用新的 ref 重试
- 不要继续沿用旧 ref

### `fatal`

常见于 CDP 连接断开、浏览器崩溃、控制器失联。

处理方式：

- 让用户检查扩展或浏览器连接
- 恢复连接后重新开始

### `ai_browse is not available`

说明当前没有配置 AI planner。此时改用手动工作流：

`observe` -> `click_ref` / `fill_ref` -> `screenshot`

## 配置与运行前提

- 浏览器工具只有在 `BrowserController != nil` 时才可用
- `evaluate` 需要 `browser.evaluateEnabled=true`
- `ai_browse` 需要 Gateway 为浏览器控制器注入 AI planner
- `chrome` 扩展接力模式依赖本地 relay，默认地址 `127.0.0.1:18792`
- 当前工具接口不负责切换 profile；profile 由浏览器/Gateway 配置决定

浏览器 profile 说明：

| Profile | 说明 | 场景 |
|---------|------|------|
| `openacosmi` | 独立隔离浏览器配置目录 | 默认、安全、适合通用自动化 |
| `chrome` | 通过扩展接管现有 Chrome 活动标签页 | 需要访问已登录会话时使用 |

扩展接力模式补充：

- 扩展通过 `chrome.debugger` 连接当前活动标签页
- 扩展通过本地 CDP relay 与 Gateway 通信
- 徽标状态通常表示：`ON` 已连接，`...` 连接中，`!` relay 不可达

## 安全边界

- `evaluate` 属于高风险动作，除非任务确实需要，不要默认执行自定义脚本
- 遇到登录页时，不要代替用户处理短信码、邮箱码、验证码或 2FA
- 如果页面提供二维码登录，应返回二维码或页面状态给用户，由用户自行扫码
- `close_tab` 只在用户明确要求关闭某个标签页时使用，不要任务结束就顺手关闭
- 浏览器工具适合网页内交互，不应用于系统级桌面控制
- 扩展接力模式可访问已登录上下文，只应在可信本地或 tailnet 环境使用
