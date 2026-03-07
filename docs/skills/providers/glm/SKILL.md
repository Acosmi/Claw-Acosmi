---
name: glm
description: "GLM 模型家族概览及在创宇太虚中的使用方法"
---

# GLM 模型

GLM 是一个**模型家族**（非公司），通过 Z.AI 平台提供。在创宇太虚中，GLM 模型通过 `zai` 供应商访问，模型 ID 如 `zai/glm-4.7`。

## CLI 设置

```bash
openacosmi onboard --auth-choice zai-api-key
```

## 配置示例

```json5
{
  env: { ZAI_API_KEY: "sk-..." },
  agents: { defaults: { model: { primary: "zai/glm-4.7" } } },
}
```

## 备注

- GLM 版本和可用性可能变化；请查看 Z.AI 文档获取最新信息。
- 示例模型 ID 包括 `glm-4.7` 和 `glm-4.6`。
- 供应商详情参见 [/providers/zai](/providers/zai)。
