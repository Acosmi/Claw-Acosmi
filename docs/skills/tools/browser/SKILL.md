---
name: browser
description: "Web automation via CDP: navigate, observe, click_ref, fill_ref, screenshot, annotate_som, ai_browse, GIF recording. ARIA refs for robust element targeting. Prefer over Argus for web"
tools: browser
---

## When to Use browser (vs Argus vs web_search)

| 场景 | 工具 | 原因 |
|------|------|------|
| 网页导航、点击、填表 | browser | ARIA ref 精准定位，比 CSS 选择器更健壮 |
| 读取静态网页内容 | web_fetch | 更轻量，无需浏览器 |
| 桌面原生应用操作 | spawn_argus_agent | 原生 UI 无 ARIA 树 |
| 网页截图验证 | browser (screenshot) | 页内截图 |
| 全桌面截图 | argus_capture_screen | 全屏截取 |
| 意图级多步浏览任务 | browser (ai_browse) | 自动 observe→plan→act 循环 |
| 可视化元素定位调试 | browser (annotate_som) | SOM 编号截图，视觉锚定 |
| 多步操作录制回放 | browser (start/stop_gif_recording) | 生成 GIF 动画 |

**关键区别**: browser 通过 ARIA 无障碍树 + ref 标识符定位元素（语义化、跨 DOM 变更健壮），Argus 用屏幕坐标（适合原生应用）。

**规则**: 有 URL 的用 browser，原生桌面窗口用 Argus。

**推荐工作流**: 先 `observe` 获取页面 ARIA 结构和 ref 标注 → 用 `click_ref` / `fill_ref` 交互（比 CSS selector 更可靠）。

---

## Agent tool: `browser`

Agent 获得**一个工具** (`browser`)，包含 **23 种 action**:

### 四层操控模型

| 层级 | Action | 场景 |
|------|--------|------|
| 1. CSS 选择器 (legacy) | `click`, `type`, `wait_for` | 向后兼容，已知选择器稳定的场景 |
| 2. ARIA ref (推荐) | `observe`, `click_ref`, `fill_ref` | 精准、抗页面变更，首选 |
| 3. SOM 视觉标注 | `annotate_som` | 视觉定位调试，编号截图 |
| 4. 意图级 | `ai_browse` | 自然语言多步任务，自动循环 |

### 完整 Action 列表

| Action | 参数 | 说明 |
|--------|------|------|
| `navigate` | `url` | 导航到 URL。返回截图用于验证。 |
| `get_content` | — | 获取页面内容。返回 ARIA 无障碍树或 innerText。 |
| `observe` | — | **推荐首步。** 返回 ARIA 树 + ref 标注 (e1, e2...) + 截图。 |
| `annotate_som` | — | SOM 视觉标注截图：在交互元素上叠加编号方框，返回编号截图 + 元素列表。用于视觉定位和调试。 |
| `click` | `selector` | 通过 CSS 选择器点击元素。 |
| `click_ref` | `ref` | **推荐。** 通过 ARIA ref (如 "e1") 点击元素。比 CSS 更健壮。 |
| `type` | `selector`, `text` | 通过 CSS 选择器输入文本。 |
| `fill_ref` | `ref`, `text` | **推荐。** 通过 ARIA ref 填入文本。 |
| `screenshot` | — | 截取页面截图 (JPEG, 优化)。 |
| `evaluate` | `script` | 执行 JavaScript（需 `browser.evaluateEnabled=true`）。 |
| `wait_for` | `selector` | 等待 CSS 选择器出现 (10s 超时, 100ms 轮询)。 |
| `go_back` | — | 浏览器后退。 |
| `go_forward` | — | 浏览器前进。 |
| `get_url` | — | 获取当前页面 URL。 |
| `ai_browse` | `goal` | **意图级浏览。** 自动 observe→plan→act 循环 (最多 20 步)。 |
| `start_gif_recording` | — | 开始 GIF 录制。后续 navigate/click_ref/fill_ref 操作自动捕获帧。最多 200 帧。 |
| `stop_gif_recording` | — | 停止 GIF 录制并返回动画 GIF 数据（含帧数和字节数）。 |
| `list_tabs` | — | 列出所有浏览器标签页（返回 id/url/title/type）。 |
| `create_tab` | `url` | 创建新标签页（url 可选，默认 about:blank）。 |
| `close_tab` | `target_id` | 关闭指定标签页。 |
| `switch_tab` | `target_id` | 切换到指定标签页。 |

### 推荐工作流

1. **`observe`** — 获取 ARIA 无障碍树 + 截图。响应中包含 ref 标识符 (e1, e2...) 用于交互元素。
2. **`click_ref` / `fill_ref`** — 用 observe 返回的 ref 交互。比 CSS 选择器更可靠（基于 ARIA 角色）。
3. **`screenshot`** — 验证结果。所有改变页面状态的操作都自动包含验证截图。

### SOM 视觉标注工作流

当 ARIA ref 不够直观或需要视觉确认元素位置时：

1. **`annotate_som`** — 在页面截图上叠加编号方框，每个交互元素标注序号。
2. 返回内容包括：编号截图 + 元素列表 `[index] tag (role=xxx) "text"`。
3. 可结合 `click_ref` 使用 SOM 编号对应的 ref 进行交互。

### GIF 录制工作流

用于记录多步操作过程：

1. **`start_gif_recording`** — 开始录制。
2. 执行一系列 `navigate` / `click_ref` / `fill_ref` 操作 — 每步自动捕获前后帧。
3. **`stop_gif_recording`** — 结束录制，返回动画 GIF 数据。

限制：最多 200 帧（超出后静默丢弃），MaxWidth 800px 自动缩放。

### 错误分类

- **Transient** — 元素可能还在加载。先用 `wait_for` 或 `observe`。
- **Structural** — 该 ref/selector 处没有元素。用 `observe` 检查页面状态。
- **Fatal** — CDP 连接断开。浏览器可能崩溃或断连。

### AI Browse (意图级)

`ai_browse` action 接受自然语言目标（如 "在京东搜索 MacBook Pro 并截图第一个结果"），自动执行多步 observe→plan→act 循环。大幅减少主对话回合数和 token 消耗。

需要 Gateway 中配置 AI planner。不可用时，使用手动 `observe` + `click_ref`/`fill_ref` 工作流替代。

### 截图优化

- 格式: JPEG (quality 75) + `optimizeForSpeed` — 比 PNG 小 3-5 倍。
- 每个改变状态的 action (navigate, click, type, click_ref, fill_ref) 自动返回验证截图。
- 截图通过 `__MULTIMODAL__` 协议传入 LLM 视觉通道。

### 选择器缓存 (Stagehand 模式)

系统内置 CSS 选择器缓存，以 `(pageURL, ref)` 为 key：
- **缓存命中**: 跳过全量 DOM 扫描，直接用缓存的 CSS 选择器。
- **自动失效**: `navigate` 和 `observe` 操作自动清空缓存。
- 对使用者透明，无需手动管理。

### 选项

- `profile` — 选择浏览器 profile (openacosmi, chrome, 或 remote CDP)。
- `target` — (`sandbox` | `host` | `node`) 选择浏览器运行位置。
- 沙箱 session 中，`target: "host"` 需要 `agents.defaults.sandbox.browser.allowHostControl=true`。
- 省略 `target` 时: 沙箱 session 默认 `sandbox`，非沙箱默认 `host`。

---

## Profiles

系统支持两种主要 profile:

- **`openacosmi`**: 专属隔离浏览器（独立 user data dir，与个人浏览器完全分开）。
- **`chrome`**: Chrome 扩展中继模式（控制已有 Chrome tab，需安装扩展并手动 attach）。

设置 `browser.defaultProfile: "openacosmi"` 使用托管浏览器模式。

## 配置

浏览器设置在 `~/.openacosmi/openacosmi.json`:

```json5
{
  browser: {
    enabled: true,
    defaultProfile: "chrome",
    headless: false,
    noSandbox: false,
    attachOnly: false,
    executablePath: "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
    profiles: {
      openacosmi: { cdpPort: 18800 },
      work: { cdpPort: 18801 },
      remote: { cdpUrl: "http://10.0.0.42:9222" },
    },
  },
}
```

- 浏览器控制服务绑定在 loopback 上，端口 = `gateway.port` + 2（默认 19003）。
- Relay 端口 = browser control port + 1（默认 19004）。
- 本地 CDP 端口: 18800-18899。
- `attachOnly: true` = 不启动浏览器，仅附加已运行的。
- 浏览器自动检测顺序: 系统默认 → Chrome → Brave → Edge → Chromium。
- **零配置启动**: 无 CDP URL 配置时，`EnsureChrome()` 自动发现或启动 Chrome。

## 安全

- `browser.evaluateEnabled=false` 可禁用 JS 执行（防 prompt injection）。
- 远程 CDP URL/token 视为 secret，通过环境变量管理。
- 扩展中继模式下附加到个人 tab = 授予该账户完整访问权限。

## MCP Server（外部工具集成）

浏览器自动化可以作为独立 MCP Server 运行，供 Claude Code、Cursor、VS Code 等外部工具使用。

```bash
# 启动（自动发现/启动 Chrome）
openacosmi-browser-mcp

# 指定 CDP URL
openacosmi-browser-mcp --cdp-url ws://127.0.0.1:9222
```

Claude Desktop 配置 (`claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "openacosmi-browser": {
      "command": "openacosmi-browser-mcp",
      "args": []
    }
  }
}
```

MCP 提供 **18 个工具**，命名对齐 Playwright MCP 惯例：`browser_navigate`, `browser_snapshot`, `browser_click`, `browser_fill`, `browser_screenshot`, `browser_evaluate`, `browser_ai_browse`, `browser_annotate_som`, `browser_list_tabs`, `browser_create_tab`, `browser_close_tab`, `browser_switch_tab` 等。

## Chrome 扩展

Chrome 扩展让 Agent 控制用户现有的 Chrome 标签页。安装后点击工具栏按钮 Attach/Detach 标签页。

安装: Chrome → `chrome://extensions` → 开发者模式 → 加载已解压的扩展 → 选择 `browser-extension/` 目录。

详见 `docs/skills/tools/chrome-extension/SKILL.md` 和 `docs/使用帮助/浏览器自动化扩展安装使用说明.md`。

## 故障排除

Linux 上 snap Chromium 问题，参见 [Browser troubleshooting](/tools/browser-linux-troubleshooting)。

## CLI 参考

完整 CLI 命令列表参见 `docs/cli/browser.md`（Rust CLI，arch: rust-cli）。

## 故障树

- 工具选错：网页任务应优先 `browser`，原生桌面窗口或全桌面状态应改走 Argus；不要把两者混成一条路径。
- `click_ref` / `fill_ref` 失败：最常见原因是 ref 已过期或页面状态已变，先重新 `observe`，不要继续盲点旧 ref。
- `ai_browse` 不可用：当前实现要求已配置 AI planner；未配置时应回退到 `observe` + `click_ref` / `fill_ref`。
- `evaluate` 失败：可能不是脚本错，而是 `browser.evaluateEnabled=false` 导致的配置级禁用。
- 操作后没有看到验证图：自动附带截图是 best-effort；截图失败或过大时可能只剩文本结果，先判断页面状态再决定是否重试。
- `annotate_som` 元素不全：SOM 仅标注可交互元素（button/link/input 等），纯文本不会被编号。
- `stop_gif_recording` 返回空：可能未先调用 `start_gif_recording`，或录制期间无页面操作。

## 回滚步骤

1. 先回到最短链路：`navigate` → `observe` → `click_ref` / `fill_ref`，不要一上来堆 CSS selector 或 `ai_browse`。
2. `ref` 失效时先重新 `observe` 获取新页面结构；不要连续重试旧 `e1/e2`。
3. `ai_browse` 不可用时立即改为手动 ARIA/ref 工作流；`evaluate` 被禁用时不要继续依赖脚本注入排障。
4. 若页面其实不是浏览器 DOM 任务，而是原生窗口、系统弹窗或整桌面状态，停止继续用 browser，切到 Argus。

## 验收清单

- 页面任务的工具选择正确：网页走 `browser`，桌面原生界面走 Argus。
- `observe` 已拿到当前页面结构，后续交互使用的是最新 ref，而不是过期 ref 或凭空猜测的 selector。
- 状态变化后的结果可被截图或文本结果验证；若无验证图，也能解释为截图 best-effort 降级而非误判成点击失败。
- `ai_browse`、`evaluate` 仅在对应运行时条件满足时使用，失败时能区分配置缺失、页面变化和 CDP 连接问题。
- SOM 和 GIF 功能用于辅助调试和记录，不作为主交互路径。
