---
name: firecrawl
description: "Firecrawl 作为 web_fetch 的回退服务（反机器人 + 缓存提取）"
---

# Firecrawl

Claw Acosmi 可以使用 **Firecrawl** 作为 `web_fetch` 的回退内容提取器。它是一项托管的内容提取服务，支持机器人对抗和缓存，有助于处理 JS 密集型网站或阻止普通 HTTP 抓取的页面。

## 获取 API 密钥

1. 创建 Firecrawl 账户并生成 API 密钥。
2. 将其存储在配置中或在 Gateway 环境中设置 `FIRECRAWL_API_KEY`。

## 配置 Firecrawl

```json5
{
  tools: {
    web: {
      fetch: {
        firecrawl: {
          apiKey: "FIRECRAWL_API_KEY_HERE",
          baseUrl: "https://api.firecrawl.dev",
          onlyMainContent: true,
          maxAgeMs: 172800000,
          timeoutSeconds: 60,
        },
      },
    },
  },
}
```

说明：

- `firecrawl.enabled` 在存在 API 密钥时默认为 true。
- `maxAgeMs` 控制缓存结果的最大有效期（毫秒）。默认 2 天。

## 隐匿 / 反机器人对抗

Firecrawl 提供**代理模式**参数用于机器人对抗（`basic`、`stealth` 或 `auto`）。Claw Acosmi 在 Firecrawl 请求中始终使用 `proxy: "auto"` 加 `storeInCache: true`。如果省略 proxy，Firecrawl 默认使用 `auto`。`auto` 模式在基本尝试失败后会使用隐匿代理重试，可能比仅使用 basic 的抓取消耗更多积分。

## `web_fetch` 如何使用 Firecrawl

`web_fetch` 提取顺序：

1. Readability（本地）
2. Firecrawl（如已配置）
3. 基本 HTML 清理（最后回退）

参见 [Web 工具](/tools/web) 了解完整的 Web 工具配置。
