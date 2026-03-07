---
name: xiaomi
description: "通过小米 MiMo（mimo-v2-flash）接入创宇太虚"
---

# 小米 MiMo

小米 MiMo 是 **MiMo** 模型的 API 平台。提供兼容 OpenAI 和 Anthropic 格式的 REST API，使用 API 密钥认证。在[小米 MiMo 控制台](https://platform.xiaomimimo.com/#/console/api-keys)创建 API 密钥。创宇太虚使用 `xiaomi` 供应商配合小米 MiMo API 密钥。

## 模型概览

- **mimo-v2-flash**：262144 token 上下文窗口，兼容 Anthropic Messages API。
- Base URL：`https://api.xiaomimimo.com/anthropic`
- 认证：`Bearer $XIAOMI_API_KEY`

## CLI 设置

```bash
openacosmi onboard --auth-choice xiaomi-api-key
# 或非交互式
openacosmi onboard --auth-choice xiaomi-api-key --xiaomi-api-key "$XIAOMI_API_KEY"
```

## 配置示例

```json5
{
  env: { XIAOMI_API_KEY: "your-key" },
  agents: { defaults: { model: { primary: "xiaomi/mimo-v2-flash" } } },
  models: {
    mode: "merge",
    providers: {
      xiaomi: {
        baseUrl: "https://api.xiaomimimo.com/anthropic",
        api: "anthropic-messages",
        apiKey: "XIAOMI_API_KEY",
        models: [
          {
            id: "mimo-v2-flash",
            name: "Xiaomi MiMo V2 Flash",
            reasoning: false,
            input: ["text"],
            cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
            contextWindow: 262144,
            maxTokens: 8192,
          },
        ],
      },
    },
  },
}
```

## 备注

- 模型引用：`xiaomi/mimo-v2-flash`。
- 当设置 `XIAOMI_API_KEY`（或存在认证配置）时，供应商会自动注入。
- 供应商规则参见 [/concepts/model-providers](/concepts/model-providers)。
