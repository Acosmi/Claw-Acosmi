---
name: llm-task
description: "面向工作流的 JSON-only LLM 任务（可选插件工具）"
---

# LLM Task

`llm-task` 是一个**可选的插件工具**，运行 JSON-only LLM 任务并返回结构化输出（可选使用 JSON Schema 验证）。

适用于 Lobster 等工作流引擎：您可以添加单个 LLM 步骤，无需为每个工作流编写自定义 Claw Acosmi 代码。

## 启用插件

1. 启用插件：

```json
{
  "plugins": {
    "entries": {
      "llm-task": { "enabled": true }
    }
  }
}
```

1. 将工具加入白名单（注册时带 `optional: true`）：

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "tools": { "allow": ["llm-task"] }
      }
    ]
  }
}
```

## 配置（可选）

```json
{
  "plugins": {
    "entries": {
      "llm-task": {
        "enabled": true,
        "config": {
          "defaultProvider": "openai-codex",
          "defaultModel": "gpt-5.2",
          "defaultAuthProfileId": "main",
          "allowedModels": ["openai-codex/gpt-5.3-codex"],
          "maxTokens": 800,
          "timeoutMs": 30000
        }
      }
    }
  }
}
```

`allowedModels` 是 `provider/model` 字符串的白名单。如果设置了，白名单之外的请求将被拒绝。

## 工具参数

- `prompt`（string，必填）
- `input`（any，可选）
- `schema`（object，可选 JSON Schema）
- `provider`（string，可选）
- `model`（string，可选）
- `authProfileId`（string，可选）
- `temperature`（number，可选）
- `maxTokens`（number，可选）
- `timeoutMs`（number，可选）

## 输出

返回 `details.json`，包含解析后的 JSON（提供了 `schema` 时会进行验证）。

## 示例：Lobster 工作流步骤

```lobster
openacosmi.invoke --tool llm-task --action json --args-json '{
  "prompt": "Given the input email, return intent and draft.",
  "input": {
    "subject": "Hello",
    "body": "Can you help?"
  },
  "schema": {
    "type": "object",
    "properties": {
      "intent": { "type": "string" },
      "draft": { "type": "string" }
    },
    "required": ["intent", "draft"],
    "additionalProperties": false
  }
}'
```

## 安全说明

- 该工具为 **JSON-only**，指示模型仅输出 JSON（无代码围栏、无注释）。
- 此运行不向模型暴露任何工具。
- 除非使用 `schema` 验证，否则将输出视为不可信。
- 在任何有副作用的步骤（发送、发布、执行）之前添加审批。
