---
name: openrouter
description: "通过 OpenRouter 统一 API 在创宇太虚中访问多种模型"
---

# OpenRouter

OpenRouter 提供**统一 API**，通过单一端点和 API 密钥将请求路由到多种模型。它兼容 OpenAI，大多数 OpenAI SDK 只需切换 base URL 即可使用。

## CLI 设置

```bash
openacosmi onboard --auth-choice apiKey --token-provider openrouter --token "$OPENROUTER_API_KEY"
```

## 配置示例

```json5
{
  env: { OPENROUTER_API_KEY: "sk-or-..." },
  agents: {
    defaults: {
      model: { primary: "openrouter/anthropic/claude-sonnet-4-5" },
    },
  },
}
```

## 备注

- 模型引用格式为 `openrouter/<provider>/<model>`。
- 更多模型/供应商选项参见 [/concepts/model-providers](/concepts/model-providers)。
- OpenRouter 底层使用 Bearer token 配合你的 API 密钥。
