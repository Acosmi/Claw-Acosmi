---
name: openai
description: "通过 API 密钥或 Codex 订阅在创宇太虚中使用 OpenAI"
---

# OpenAI

OpenAI 提供 GPT 模型的开发者 API。Codex 支持 **ChatGPT 登录**（订阅访问）或 **API 密钥登录**（按量计费）。Codex 云端需要 ChatGPT 登录。

## 方案 A：OpenAI API 密钥（OpenAI Platform）

**适用于：** 直接 API 访问和按量计费。
从 OpenAI 控制面板获取 API 密钥。

### CLI 设置

```bash
openacosmi onboard --auth-choice openai-api-key
# 或非交互式
openacosmi onboard --openai-api-key "$OPENAI_API_KEY"
```

### 配置示例

```json5
{
  env: { OPENAI_API_KEY: "sk-..." },
  agents: { defaults: { model: { primary: "openai/gpt-5.1-codex" } } },
}
```

## 方案 B：OpenAI Code（Codex）订阅

**适用于：** 使用 ChatGPT/Codex 订阅访问而非 API 密钥。
Codex 云端需要 ChatGPT 登录，Codex CLI 支持 ChatGPT 或 API 密钥登录。

### CLI 设置（Codex OAuth）

```bash
# 在向导中运行 Codex OAuth
openacosmi onboard --auth-choice openai-codex

# 或直接运行 OAuth
openacosmi models auth login --provider openai-codex
```

### 配置示例（Codex 订阅）

```json5
{
  agents: { defaults: { model: { primary: "openai-codex/gpt-5.3-codex" } } },
}
```

## 备注

- 模型引用始终使用 `provider/model` 格式（参见 [/concepts/models](/concepts/models)）。
- 认证详情和复用规则参见 [/concepts/oauth](/concepts/oauth)。
