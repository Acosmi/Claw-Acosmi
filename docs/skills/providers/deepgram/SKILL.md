---
name: deepgram
description: "Deepgram 语音转录用于入站语音消息"
---

# Deepgram（音频转录）

Deepgram 是语音转文字 API。在创宇太虚中用于通过 `tools.media.audio` 进行**入站音频/语音消息转录**。

启用后，创宇太虚将音频文件上传至 Deepgram 并将转录文本注入回复管线（`{{Transcript}}` + `[Audio]` 块）。这**不是**流式的；使用的是预录制转录端点。

官网：[https://deepgram.com](https://deepgram.com)
文档：[https://developers.deepgram.com](https://developers.deepgram.com)

## 快速开始

1. 设置 API 密钥：

```bash
DEEPGRAM_API_KEY=dg_...
```

1. 启用供应商：

```json5
{
  tools: {
    media: {
      audio: {
        enabled: true,
        models: [{ provider: "deepgram", model: "nova-3" }],
      },
    },
  },
}
```

## 选项

- `model`：Deepgram 模型 ID（默认：`nova-3`）
- `language`：语言提示（可选）
- `tools.media.audio.providerOptions.deepgram.detect_language`：启用语言检测（可选）
- `tools.media.audio.providerOptions.deepgram.punctuate`：启用标点（可选）
- `tools.media.audio.providerOptions.deepgram.smart_format`：启用智能格式化（可选）

带语言的示例：

```json5
{
  tools: {
    media: {
      audio: {
        enabled: true,
        models: [{ provider: "deepgram", model: "nova-3", language: "en" }],
      },
    },
  },
}
```

带 Deepgram 选项的示例：

```json5
{
  tools: {
    media: {
      audio: {
        enabled: true,
        providerOptions: {
          deepgram: {
            detect_language: true,
            punctuate: true,
            smart_format: true,
          },
        },
        models: [{ provider: "deepgram", model: "nova-3" }],
      },
    },
  },
}
```

## 备注

- 认证遵循标准供应商认证顺序；`DEEPGRAM_API_KEY` 是最简单的方式。
- 使用代理时可通过 `tools.media.audio.baseUrl` 和 `tools.media.audio.headers` 覆盖端点或头。
- 输出遵循与其他供应商相同的音频规则（大小限制、超时、转录注入）。
