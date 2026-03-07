---
name: brave-search
description: "Brave Search API 配置（用于 web_search 工具）"
---

# Brave Search API

创宇太虚默认使用 Brave Search 作为 `web_search` 的搜索服务提供商。

## 获取 API 密钥

1. 在 [https://brave.com/search/api/](https://brave.com/search/api/) 创建 Brave Search API 账户
2. 在控制台中选择 **Data for Search** 套餐并生成 API 密钥
3. 将密钥存入配置（推荐）或在网关环境变量中设置 `BRAVE_API_KEY`

## 配置示例

```json5
{
  tools: {
    web: {
      search: {
        provider: "brave",
        apiKey: "BRAVE_API_KEY_HERE",
        maxResults: 5,
        timeoutSeconds: 30,
      },
    },
  },
}
```

## 注意事项

- Data for AI 套餐**不**兼容 `web_search`。
- Brave 提供免费层及付费套餐，请在 Brave API 门户查看当前配额限制。

完整 web_search 配置详见 [Web 工具](/tools/web)。
