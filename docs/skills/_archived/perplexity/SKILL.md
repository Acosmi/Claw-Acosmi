---
name: perplexity
description: "Perplexity Sonar 配置（用于 web_search 工具）"
---

# Perplexity Sonar

创宇太虚可以使用 Perplexity Sonar 作为 `web_search` 工具的搜索引擎。支持通过 Perplexity 直连 API 或 OpenRouter 中转连接。

## API 接入方式

### Perplexity（直连）

- 基础地址：[https://api.perplexity.ai](https://api.perplexity.ai)
- 环境变量：`PERPLEXITY_API_KEY`

### OpenRouter（替代方案）

- 基础地址：[https://openrouter.ai/api/v1](https://openrouter.ai/api/v1)
- 环境变量：`OPENROUTER_API_KEY`
- 支持预付费/加密货币充值。

## 配置示例

```json5
{
  tools: {
    web: {
      search: {
        provider: "perplexity",
        perplexity: {
          apiKey: "pplx-...",
          baseUrl: "https://api.perplexity.ai",
          model: "perplexity/sonar-pro",
        },
      },
    },
  },
}
```

## 从 Brave 切换到 Perplexity

```json5
{
  tools: {
    web: {
      search: {
        provider: "perplexity",
        perplexity: {
          apiKey: "pplx-...",
          baseUrl: "https://api.perplexity.ai",
        },
      },
    },
  },
}
```

若同时设置了 `PERPLEXITY_API_KEY` 和 `OPENROUTER_API_KEY`，通过 `tools.web.search.perplexity.baseUrl`（或 `tools.web.search.perplexity.apiKey`）来消除歧义。

若未设置 baseUrl，创宇太虚根据 API 密钥来源自动选择默认值：

- `PERPLEXITY_API_KEY` 或 `pplx-...` → 直连 Perplexity（`https://api.perplexity.ai`）
- `OPENROUTER_API_KEY` 或 `sk-or-...` → OpenRouter（`https://openrouter.ai/api/v1`）
- 未知密钥格式 → OpenRouter（安全回退）

## 模型

- `perplexity/sonar` — 快速问答 + 网络搜索
- `perplexity/sonar-pro`（默认） — 多步推理 + 网络搜索
- `perplexity/sonar-reasoning-pro` — 深度研究

完整 web_search 配置详见 [Web 工具](/tools/web)。
