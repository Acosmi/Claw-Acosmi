---
name: web
description: "Web 搜索 + 抓取工具（博查搜索 API 默认、Google Custom Search 可选）"
tools: web_search
---

## 何时使用 web_search/web_fetch（vs browser）

| 场景 | 工具 | 原因 |
|------|------|------|
| 搜索信息 | web_search | API 调用，快速，结构化结果 |
| 读取文章/文档 | web_fetch | 提取可读内容，无需 JS |
| 操作 Web 应用（点击、填表） | browser | 需要 DOM 交互 |
| JS 重度 SPA 内容 | browser | web_fetch 无法执行 JS |
| 需要登录的页面 | browser | web_fetch 无 session/cookies |

---

# Web 工具

Claw Acosmi 内置两个轻量级 Web 工具：

- `web_search` — 通过博查搜索 API（默认）或 Google Custom Search（可选）搜索网络。
- `web_fetch` — HTTP 抓取 + 可读内容提取（HTML → markdown/text）。

这些**不是**浏览器自动化。对于 JS 密集型网站或需要登录的场景，请使用 [Browser 工具](/tools/browser)。

## 工作原理

- `web_search` 调用您配置的 Provider 并返回结果。
  - **博查**（默认）：返回结构化结果（标题、URL、摘要）。
  - **Google**（可选）：使用 Google Custom Search API 返回结构化结果。
- 结果按查询缓存 15 分钟（可配置）。
- `web_fetch` 执行普通 HTTP GET 并提取可读内容（HTML → markdown/text）。它**不会**执行 JavaScript。
- `web_fetch` 默认启用（除非显式禁用）。

## 选择搜索 Provider

| Provider | 优点 | 缺点 | API 密钥 |
|----------|------|------|----------|
| **博查**（默认） | 中文搜索优化、快速、结构化结果 | — | `BOCHA_API_KEY` |
| **Google** | 全球覆盖、多语言 | 需要 Google Cloud 项目 + CSE ID | `GOOGLE_SEARCH_API_KEY` + `GOOGLE_SEARCH_ENGINE_ID` |

在配置中设置 Provider：

```json5
{
  tools: {
    web: {
      search: {
        provider: "bocha", // 或 "google"
      },
    },
  },
}
```

## 获取博查 API 密钥

1. 在博查搜索 API 平台注册账户
2. 创建应用并生成 API 密钥
3. 运行 `openacosmi configure --section web` 将密钥存储到配置中（推荐），或在环境中设置 `BOCHA_API_KEY`。

### 密钥存放位置（推荐）

**推荐：** 运行 `openacosmi configure --section web`。密钥存储在 `~/.openacosmi/openacosmi.json` 的 `tools.web.search.bocha.apiKey` 下。

**环境变量替代：** 在 Gateway 进程环境中设置 `BOCHA_API_KEY`。Gateway 安装方式下，放在 `~/.openacosmi/.env`（或您的服务环境中）。

## 使用 Google Custom Search（可选）

Google Custom Search API 提供全球搜索结果，需要 Google Cloud 项目。

### 获取 Google API 密钥

1. 在 [Google Cloud Console](https://console.cloud.google.com/) 创建项目
2. 启用 Custom Search JSON API
3. 创建 API 密钥
4. 在 [Programmable Search Engine](https://programmablesearchengine.google.com/) 创建搜索引擎并获取 Search Engine ID

### 设置 Google 搜索

```json5
{
  tools: {
    web: {
      search: {
        provider: "google",
        google: {
          apiKey: "YOUR_GOOGLE_API_KEY",
          searchEngineId: "YOUR_SEARCH_ENGINE_ID",
        },
      },
    },
  },
}
```

## web_search

使用配置的 Provider 搜索网络。

### 要求

- `tools.web.search.enabled` 不能为 `false`（默认启用）
- 所选 Provider 的 API 密钥：
  - **博查**：`BOCHA_API_KEY` 或 `tools.web.search.bocha.apiKey`
  - **Google**：`GOOGLE_SEARCH_API_KEY` + `GOOGLE_SEARCH_ENGINE_ID`

### 配置

```json5
{
  tools: {
    web: {
      search: {
        enabled: true,
        provider: "bocha", // 默认
        maxResults: 5,
        timeoutSeconds: 30,
        cacheTtlMinutes: 15,
      },
    },
  },
}
```

### 工具参数

- `query`（必填）
- `count`（1–10；默认来自配置）
- `country`（可选）：2 字母国家代码，用于区域特定结果（如 "DE"、"US"、"ALL"）。
- `search_lang`（可选）：搜索结果的 ISO 语言代码（如 "de"、"en"、"fr"）
- `freshness`（可选）：按时间过滤结果

**示例：**

```javascript
// 基础搜索
await web_search({ query: "最新 AI 新闻", count: 5 });

// 指定区域
await web_search({ query: "local restaurants", country: "US", search_lang: "en" });
```

## web_fetch

抓取 URL 并提取可读内容。

### web_fetch 要求

- `tools.web.fetch.enabled` 不能为 `false`（默认启用）
- 可选 Firecrawl 回退：设置 `tools.web.fetch.firecrawl.apiKey` 或 `FIRECRAWL_API_KEY`。

### web_fetch 配置

```json5
{
  tools: {
    web: {
      fetch: {
        enabled: true,
        maxChars: 50000,
        maxCharsCap: 50000,
        timeoutSeconds: 30,
        cacheTtlMinutes: 15,
        maxRedirects: 3,
        readability: true,
        firecrawl: {
          enabled: true,
          apiKey: "FIRECRAWL_API_KEY_HERE",
          baseUrl: "https://api.firecrawl.dev",
          onlyMainContent: true,
          maxAgeMs: 86400000,
          timeoutSeconds: 60,
        },
      },
    },
  },
}
```

### web_fetch 工具参数

- `url`（必填，仅 http/https）
- `extractMode`（`markdown` | `text`）
- `maxChars`（截断长页面）

说明：

- `web_fetch` 先使用 Readability（主内容提取），然后 Firecrawl（如已配置）。如果都失败，工具返回错误。
- Firecrawl 请求默认使用反机器人模式并缓存结果。
- `web_fetch` 阻止私有/内部主机名，并重新检查重定向（通过 `maxRedirects` 限制）。
- `maxChars` 被限制在 `tools.web.fetch.maxCharsCap`。
- `web_fetch` 是尽力而为的提取；某些网站需要使用 browser 工具。
- 参见 [Firecrawl](/tools/firecrawl) 了解密钥设置和服务详情。
- 响应被缓存（默认 15 分钟）以减少重复抓取。
- 如果使用工具 profile/白名单，添加 `web_search`/`web_fetch` 或 `group:web`。
- 如果缺少搜索 API 密钥，`web_search` 返回简短的设置提示和文档链接。
