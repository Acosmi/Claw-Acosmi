---
name: ollama
description: "使用 Ollama（本地 LLM 运行时）运行 Claw Acosmi"
---

# Ollama

Ollama 是一个本地 LLM 运行时，可以轻松地在您的机器上运行开源模型。Claw Acosmi 通过 Ollama 的 OpenAI 兼容 API 进行集成，并且当您通过设置 `OLLAMA_API_KEY`（或认证配置）且未定义显式 `models.providers.ollama` 条目时，可以**自动发现支持工具调用的模型**。

## 快速开始

1. 安装 Ollama：[https://ollama.ai](https://ollama.ai)

2. 拉取模型：

```bash
ollama pull gpt-oss:20b
# 或
ollama pull llama3.3
# 或
ollama pull qwen2.5-coder:32b
# 或
ollama pull deepseek-r1:32b
```

1. 为 Claw Acosmi 启用 Ollama（任意值均可；Ollama 不需要真正的密钥）：

```bash
# 设置环境变量
export OLLAMA_API_KEY="ollama-local"

# 或在配置文件中配置
openacosmi config set models.providers.ollama.apiKey "ollama-local"
```

1. 使用 Ollama 模型：

```json5
{
  agents: {
    defaults: {
      model: { primary: "ollama/gpt-oss:20b" },
    },
  },
}
```

## 模型发现（隐式 Provider）

当您设置了 `OLLAMA_API_KEY`（或认证配置）且**未**定义 `models.providers.ollama` 时，Claw Acosmi 会从本地 Ollama 实例（`http://127.0.0.1:11434`）自动发现模型：

- 查询 `/api/tags` 和 `/api/show`
- 仅保留报告了 `tools` 能力的模型
- 当模型报告 `thinking` 时标记 `reasoning`
- 从 `model_info["<arch>.context_length"]` 读取 `contextWindow`（如可用）
- 将 `maxTokens` 设为上下文窗口的 10 倍
- 所有费用设为 `0`

这样可以避免手动配置模型条目，同时保持模型目录与 Ollama 的能力对齐。

查看可用模型：

```bash
ollama list
openacosmi models list
```

要添加新模型，只需使用 Ollama 拉取：

```bash
ollama pull mistral
```

新模型将被自动发现并可供使用。

如果您显式设置了 `models.providers.ollama`，自动发现功能将被跳过，您需要手动定义模型（见下文）。

## 配置

### 基本设置（隐式发现）

启用 Ollama 最简单的方式是通过环境变量：

```bash
export OLLAMA_API_KEY="ollama-local"
```

### 显式设置（手动定义模型）

在以下情况使用显式配置：

- Ollama 运行在其他主机/端口上。
- 您想强制指定上下文窗口或模型列表。
- 您想包含未报告工具支持的模型。

```json5
{
  models: {
    providers: {
      ollama: {
        // 使用包含 /v1 的地址以兼容 OpenAI API
        baseUrl: "http://ollama-host:11434/v1",
        apiKey: "ollama-local",
        api: "openai-completions",
        models: [
          {
            id: "gpt-oss:20b",
            name: "GPT-OSS 20B",
            reasoning: false,
            input: ["text"],
            cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
            contextWindow: 8192,
            maxTokens: 8192 * 10
          }
        ]
      }
    }
  }
}
```

如果设置了 `OLLAMA_API_KEY`，您可以在 Provider 条目中省略 `apiKey`，Claw Acosmi 会在可用性检查时自动填充。

### 自定义 Base URL（显式配置）

如果 Ollama 运行在不同的主机或端口上（显式配置会禁用自动发现，需手动定义模型）：

```json5
{
  models: {
    providers: {
      ollama: {
        apiKey: "ollama-local",
        baseUrl: "http://ollama-host:11434/v1",
      },
    },
  },
}
```

### 模型选择

配置完成后，所有 Ollama 模型均可使用：

```json5
{
  agents: {
    defaults: {
      model: {
        primary: "ollama/gpt-oss:20b",
        fallbacks: ["ollama/llama3.3", "ollama/qwen2.5-coder:32b"],
      },
    },
  },
}
```

## 高级选项

### 推理模型

当 Ollama 在 `/api/show` 中报告 `thinking` 时，Claw Acosmi 会将模型标记为具有推理能力：

```bash
ollama pull deepseek-r1:32b
```

### 模型费用

Ollama 是免费的且在本地运行，因此所有模型费用设为 $0。

### 流式传输配置

由于底层 SDK 与 Ollama 响应格式存在[已知问题](https://github.com/badlogic/pi-mono/issues/1205)，**Ollama 模型默认禁用流式传输**。这可以防止使用支持工具调用的模型时出现损坏的响应。

禁用流式传输后，响应将一次性返回（非流式模式），避免了交错的 content/reasoning delta 导致的输出混乱。

#### 重新启用流式传输（高级）

如果您想为 Ollama 重新启用流式传输（可能会导致工具调用模型出现问题）：

```json5
{
  agents: {
    defaults: {
      models: {
        "ollama/gpt-oss:20b": {
          streaming: true,
        },
      },
    },
  },
}
```

#### 为其他 Provider 禁用流式传输

如有需要，您也可以为任何 Provider 禁用流式传输：

```json5
{
  agents: {
    defaults: {
      models: {
        "openai/gpt-4": {
          streaming: false,
        },
      },
    },
  },
}
```

### 上下文窗口

对于自动发现的模型，Claw Acosmi 使用 Ollama 报告的上下文窗口（如可用），否则默认为 `8192`。您可以在显式 Provider 配置中覆盖 `contextWindow` 和 `maxTokens`。

## 故障排除

### Ollama 未被检测到

确保 Ollama 正在运行，并且您已设置 `OLLAMA_API_KEY`（或认证配置），且**未**定义显式 `models.providers.ollama` 条目：

```bash
ollama serve
```

并确保 API 可访问：

```bash
curl http://localhost:11434/api/tags
```

### 没有可用模型

Claw Acosmi 仅自动发现报告了工具支持的模型。如果您的模型未列出，可以：

- 拉取一个支持工具调用的模型，或
- 在 `models.providers.ollama` 中显式定义该模型。

添加模型：

```bash
ollama list  # 查看已安装的模型
ollama pull gpt-oss:20b  # 拉取支持工具调用的模型
ollama pull llama3.3     # 或其他模型
```

### 连接被拒绝

检查 Ollama 是否在正确的端口上运行：

```bash
# 检查 Ollama 是否正在运行
ps aux | grep ollama

# 或重启 Ollama
ollama serve
```

### 响应损坏或输出中包含工具名

如果您在使用 Ollama 模型时看到包含工具名（如 `sessions_send`、`memory_get`）的混乱响应或碎片化文本，这是由上游 SDK 的流式响应问题导致的。在最新版 Claw Acosmi 中，**此问题已通过默认禁用 Ollama 模型的流式传输来修复**。

如果您手动启用了流式传输并遇到此问题：

1. 从 Ollama 模型条目中移除 `streaming: true` 配置，或
2. 为 Ollama 模型显式设置 `streaming: false`（参见[流式传输配置](#流式传输配置)）

## 另请参阅

- [模型 Provider](/concepts/model-providers) - 所有 Provider 概览
- [模型选择](/concepts/models) - 如何选择模型
- [配置](/gateway/configuration) - 完整配置参考
