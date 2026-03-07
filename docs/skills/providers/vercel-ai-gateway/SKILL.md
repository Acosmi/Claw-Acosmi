---
name: vercel-ai-gateway
description: "Vercel AI Gateway 设置（认证 + 模型选择）"
---

# Vercel AI Gateway

[Vercel AI Gateway](https://vercel.com/ai-gateway) 提供统一 API，通过单一端点访问数百种模型。

- 供应商：`vercel-ai-gateway`
- 认证：`AI_GATEWAY_API_KEY`
- API：兼容 Anthropic Messages

## 快速开始

1. 设置 API 密钥（推荐：为网关存储）：

```bash
openacosmi onboard --auth-choice ai-gateway-api-key
```

1. 设置默认模型：

```json5
{
  agents: {
    defaults: {
      model: { primary: "vercel-ai-gateway/anthropic/claude-opus-4.6" },
    },
  },
}
```

## 非交互式示例

```bash
openacosmi onboard --non-interactive \
  --mode local \
  --auth-choice ai-gateway-api-key \
  --ai-gateway-api-key "$AI_GATEWAY_API_KEY"
```

## 环境说明

若网关作为守护进程（launchd/systemd）运行，确保 `AI_GATEWAY_API_KEY` 对该进程可用（例如在 `~/.openacosmi/.env` 中或通过 `env.shellEnv`）。
