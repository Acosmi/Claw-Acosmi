---
name: zai
description: "通过 Z.AI（GLM 模型）接入创宇太虚"
---

# Z.AI

Z.AI 是 **GLM** 模型的 API 平台。提供 GLM 的 REST API 并使用 API 密钥认证。在 Z.AI 控制台创建 API 密钥。创宇太虚使用 `zai` 供应商配合 Z.AI API 密钥。

## CLI 设置

```bash
openacosmi onboard --auth-choice zai-api-key
# 或非交互式
openacosmi onboard --zai-api-key "$ZAI_API_KEY"
```

## 配置示例

```json5
{
  env: { ZAI_API_KEY: "sk-..." },
  agents: { defaults: { model: { primary: "zai/glm-4.7" } } },
}
```

## 备注

- GLM 模型以 `zai/<model>` 格式访问（例如：`zai/glm-4.7`）。
- 模型家族概览参见 [/providers/glm](/providers/glm)。
- Z.AI 使用 Bearer auth 配合你的 API 密钥。
