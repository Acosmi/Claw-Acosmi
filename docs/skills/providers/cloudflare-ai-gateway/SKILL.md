---
name: cloudflare-ai-gateway
description: "Cloudflare AI Gateway 设置（认证 + 模型选择）"
---

# Cloudflare AI Gateway

Cloudflare AI Gateway 置于供应商 API 前端，支持添加分析、缓存和控制。对于 Anthropic，创宇太虚通过你的 Gateway 端点使用 Anthropic Messages API。

- 供应商：`cloudflare-ai-gateway`
- Base URL：`https://gateway.ai.cloudflare.com/v1/<account_id>/<gateway_id>/anthropic`
- 默认模型：`cloudflare-ai-gateway/claude-sonnet-4-5`
- API 密钥：`CLOUDFLARE_AI_GATEWAY_API_KEY`（通过 Gateway 发送请求的供应商 API 密钥）

对于 Anthropic 模型，使用你的 Anthropic API 密钥。

## 快速开始

1. 设置供应商 API 密钥和 Gateway 信息：

```bash
openacosmi onboard --auth-choice cloudflare-ai-gateway-api-key
```

1. 设置默认模型：

```json5
{
  agents: {
    defaults: {
      model: { primary: "cloudflare-ai-gateway/claude-sonnet-4-5" },
    },
  },
}
```

## 非交互式示例

```bash
openacosmi onboard --non-interactive \
  --mode local \
  --auth-choice cloudflare-ai-gateway-api-key \
  --cloudflare-ai-gateway-account-id "your-account-id" \
  --cloudflare-ai-gateway-gateway-id "your-gateway-id" \
  --cloudflare-ai-gateway-api-key "$CLOUDFLARE_AI_GATEWAY_API_KEY"
```

## 需认证的 Gateway

若在 Cloudflare 中启用了 Gateway 认证，需添加 `cf-aig-authorization` 头（这是在供应商 API 密钥之外的额外认证）。

```json5
{
  models: {
    providers: {
      "cloudflare-ai-gateway": {
        headers: {
          "cf-aig-authorization": "Bearer <cloudflare-ai-gateway-token>",
        },
      },
    },
  },
}
```

## 环境说明

若网关作为守护进程（launchd/systemd）运行，确保 `CLOUDFLARE_AI_GATEWAY_API_KEY` 对该进程可用（例如在 `~/.openacosmi/.env` 中或通过 `env.shellEnv`）。
