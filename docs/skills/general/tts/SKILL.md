---
name: tts
description: "文本转语音（TTS）用于出站回复"
---

# 文本转语音（TTS）

创宇太虚可以使用 ElevenLabs、OpenAI 或 Edge TTS 将出站回复转换为音频。
适用于所有创宇太虚可发送音频的场景；Telegram 上会显示为圆形语音消息气泡。

## Supported services

- **ElevenLabs** (primary or fallback provider)
- **OpenAI** (primary or fallback provider; also used for summaries)
- **Edge TTS** (primary or fallback provider; uses `node-edge-tts`, default when no API keys)

### Edge TTS notes

Edge TTS uses Microsoft Edge's online neural TTS service via the `node-edge-tts`
library. It's a hosted service (not local), uses Microsoft’s endpoints, and does
not require an API key. `node-edge-tts` exposes speech configuration options and
output formats, but not all options are supported by the Edge service. citeturn2search0

Because Edge TTS is a public web service without a published SLA or quota, treat it
as best-effort. If you need guaranteed limits and support, use OpenAI or ElevenLabs.
Microsoft's Speech REST API documents a 10‑minute audio limit per request; Edge TTS
does not publish limits, so assume similar or lower limits. citeturn0search3

## 可选密钥

若要使用 OpenAI 或 ElevenLabs：

- `ELEVENLABS_API_KEY`（或 `XI_API_KEY`）
- `OPENAI_API_KEY`

Edge TTS **不**需要 API 密钥。若未找到任何 API 密钥，创宇太虚默认使用 Edge TTS
（除非通过 `messages.tts.edge.enabled=false` 禁用）。

若配置了多个供应商，选定的供应商优先使用，其余作为回退选项。
自动摘要使用配置的 `summaryModel`（或 `agents.defaults.model.primary`），
因此启用摘要时该供应商也必须已认证。

## 服务链接

- [OpenAI 文本转语音指南](https://platform.openai.com/docs/guides/text-to-speech)
- [OpenAI Audio API 参考](https://platform.openai.com/docs/api-reference/audio)
- [ElevenLabs 文本转语音](https://elevenlabs.io/docs/api-reference/text-to-speech)
- [ElevenLabs 认证](https://elevenlabs.io/docs/api-reference/authentication)
- [node-edge-tts](https://github.com/SchneeHertz/node-edge-tts)
- [Microsoft 语音输出格式](https://learn.microsoft.com/azure/ai-services/speech-service/rest-text-to-speech#audio-outputs)

## 默认是否启用？

否。自动 TTS 默认**关闭**。通过配置 `messages.tts.auto` 或按会话使用 `/tts always`（别名：`/tts on`）启用。

TTS 启用后，Edge TTS 默认**开启**，当无 OpenAI 或 ElevenLabs API 密钥时自动使用。

## 配置

TTS 配置位于 `openacosmi.json` 中的 `messages.tts` 下。
完整 schema 详见 [网关配置](/gateway/configuration)。

### 最小配置（启用 + 供应商）

```json5
{
  messages: {
    tts: {
      auto: "always",
      provider: "elevenlabs",
    },
  },
}
```

### OpenAI 主供应商 + ElevenLabs 回退

```json5
{
  messages: {
    tts: {
      auto: "always",
      provider: "openai",
      summaryModel: "openai/gpt-4.1-mini",
      modelOverrides: {
        enabled: true,
      },
      openai: {
        apiKey: "openai_api_key",
        model: "gpt-4o-mini-tts",
        voice: "alloy",
      },
      elevenlabs: {
        apiKey: "elevenlabs_api_key",
        baseUrl: "https://api.elevenlabs.io",
        voiceId: "voice_id",
        modelId: "eleven_multilingual_v2",
        seed: 42,
        applyTextNormalization: "auto",
        languageCode: "en",
        voiceSettings: {
          stability: 0.5,
          similarityBoost: 0.75,
          style: 0.0,
          useSpeakerBoost: true,
          speed: 1.0,
        },
      },
    },
  },
}
```

### Edge TTS 主供应商（无需 API 密钥）

```json5
{
  messages: {
    tts: {
      auto: "always",
      provider: "edge",
      edge: {
        enabled: true,
        voice: "en-US-MichelleNeural",
        lang: "en-US",
        outputFormat: "audio-24khz-48kbitrate-mono-mp3",
        rate: "+10%",
        pitch: "-5%",
      },
    },
  },
}
```

### 禁用 Edge TTS

```json5
{
  messages: {
    tts: {
      edge: {
        enabled: false,
      },
    },
  },
}
```

### 自定义限制 + 偏好路径

```json5
{
  messages: {
    tts: {
      auto: "always",
      maxTextLength: 4000,
      timeoutMs: 30000,
      prefsPath: "~/.openacosmi/settings/tts.json",
    },
  },
}
```

### 仅在收到语音消息后回复音频

```json5
{
  messages: {
    tts: {
      auto: "inbound",
    },
  },
}
```

### 禁用长回复的自动摘要

```json5
{
  messages: {
    tts: {
      auto: "always",
    },
  },
}
```

然后运行：

```bash
/tts summary off
```

### 字段说明

- `auto`：自动 TTS 模式（`off`、`always`、`inbound`、`tagged`）。
  - `inbound` 仅在收到入站语音消息后发送音频。
  - `tagged` 仅在回复包含 `[[tts]]` 标签时发送音频。
- `enabled`：旧版开关（doctor 会迁移为 `auto`）。
- `mode`：`"final"`（默认）或 `"all"`（包含工具/块回复）。
- `provider`：`"elevenlabs"`、`"openai"` 或 `"edge"`（回退自动处理）。
- 若 `provider` **未设置**，创宇太虚优先使用 `openai`（若有密钥），然后 `elevenlabs`（若有密钥），否则 `edge`。
- `summaryModel`：可选的低成本模型用于自动摘要；默认为 `agents.defaults.model.primary`。
  - 接受 `provider/model` 或已配置的模型别名。
- `modelOverrides`：允许模型发出 TTS 指令（默认开启）。
- `maxTextLength`：TTS 输入的硬上限（字符数）。超出时 `/tts audio` 失败。
- `timeoutMs`：请求超时（ms）。
- `prefsPath`：覆盖本地偏好 JSON 路径（供应商/限制/摘要）。
- `apiKey` 值回退到环境变量（`ELEVENLABS_API_KEY`/`XI_API_KEY`、`OPENAI_API_KEY`）。
- `elevenlabs.baseUrl`：覆盖 ElevenLabs API 基础 URL。
- `elevenlabs.voiceSettings`：
  - `stability`、`similarityBoost`、`style`：`0..1`
  - `useSpeakerBoost`：`true|false`
  - `speed`：`0.5..2.0`（1.0 = 正常）
- `elevenlabs.applyTextNormalization`：`auto|on|off`
- `elevenlabs.languageCode`：2 字母 ISO 639-1（如 `en`、`de`）
- `elevenlabs.seed`：整数 `0..4294967295`（尽力确定性）
- `edge.enabled`：允许使用 Edge TTS（默认 `true`；无需 API 密钥）。
- `edge.voice`：Edge 神经语音名称（如 `en-US-MichelleNeural`）。
- `edge.lang`：语言代码（如 `en-US`）。
- `edge.outputFormat`：Edge 输出格式（如 `audio-24khz-48kbitrate-mono-mp3`）。
  - 有效值详见 Microsoft Speech 输出格式；并非所有格式都被 Edge 支持。
- `edge.rate` / `edge.pitch` / `edge.volume`：百分比字符串（如 `+10%`、`-5%`）。
- `edge.saveSubtitles`：在音频文件旁写入 JSON 字幕。
- `edge.proxy`：Edge TTS 请求的代理 URL。
- `edge.timeoutMs`：请求超时覆盖（ms）。

## 模型驱动覆盖（默认开启）

默认情况下，模型**可以**为单次回复发出 TTS 指令。
当 `messages.tts.auto` 为 `tagged` 时，这些指令是触发音频的必要条件。

启用时，模型可发出 `[[tts:...]]` 指令来覆盖单次回复的语音，
加上可选的 `[[tts:text]]...[[/tts:text]]` 块来提供表现性标签（笑声、歌唱提示等），这些标签仅出现在音频中。

回复载荷示例：

```text
Here you go.

[[tts:provider=elevenlabs voiceId=pMsXgVXv3BLzUgSXRplE model=eleven_v3 speed=1.1]]
[[tts:text]](笑) 再读一遍这首歌。[[/tts:text]]
```

可用指令键（启用时）：

- `provider`（`openai` | `elevenlabs` | `edge`）
- `voice`（OpenAI 语音）或 `voiceId`（ElevenLabs）
- `model`（OpenAI TTS 模型或 ElevenLabs 模型 ID）
- `stability`、`similarityBoost`、`style`、`speed`、`useSpeakerBoost`
- `applyTextNormalization`（`auto|on|off`）
- `languageCode`（ISO 639-1）
- `seed`

禁用所有模型覆盖：

```json5
{
  messages: {
    tts: {
      modelOverrides: {
        enabled: false,
      },
    },
  },
}
```

可选白名单（禁用特定覆盖但保留标签功能）：

```json5
{
  messages: {
    tts: {
      modelOverrides: {
        enabled: true,
        allowProvider: false,
        allowSeed: false,
      },
    },
  },
}
```

## 用户偏好设置

斜杠命令将本地覆盖写入 `prefsPath`（默认：
`~/.openacosmi/settings/tts.json`，可通过 `OPENACOSMI_TTS_PREFS` 或
`messages.tts.prefsPath` 覆盖）。

存储字段：

- `enabled`
- `provider`
- `maxLength`（摘要阈值；默认 1500 字符）
- `summarize`（默认 `true`）

这些设置会覆盖该主机上的 `messages.tts.*` 配置。

## Output formats (fixed)

- **Telegram**: Opus voice note (`opus_48000_64` from ElevenLabs, `opus` from OpenAI).
  - 48kHz / 64kbps is a good voice-note tradeoff and required for the round bubble.
- **Other channels**: MP3 (`mp3_44100_128` from ElevenLabs, `mp3` from OpenAI).
  - 44.1kHz / 128kbps is the default balance for speech clarity.
- **Edge TTS**: uses `edge.outputFormat` (default `audio-24khz-48kbitrate-mono-mp3`).
  - `node-edge-tts` accepts an `outputFormat`, but not all formats are available
    from the Edge service. citeturn2search0
  - Output format values follow Microsoft Speech output formats (including Ogg/WebM Opus). citeturn1search0
  - Telegram `sendVoice` accepts OGG/MP3/M4A; use OpenAI/ElevenLabs if you need
    guaranteed Opus voice notes. citeturn1search1
  - If the configured Edge output format fails, Claw Acosmi retries with MP3.

OpenAI/ElevenLabs formats are fixed; Telegram expects Opus for voice-note UX.

## Auto-TTS behavior

When enabled, Claw Acosmi:

- skips TTS if the reply already contains media or a `MEDIA:` directive.
- skips very short replies (< 10 chars).
- summarizes long replies when enabled using `agents.defaults.model.primary` (or `summaryModel`).
- attaches the generated audio to the reply.

If the reply exceeds `maxLength` and summary is off (or no API key for the
summary model), audio
is skipped and the normal text reply is sent.

## Flow diagram

```
Reply -> TTS enabled?
  no  -> send text
  yes -> has media / MEDIA: / short?
          yes -> send text
          no  -> length > limit?
                   no  -> TTS -> attach audio
                   yes -> summary enabled?
                            no  -> send text
                            yes -> summarize (summaryModel or agents.defaults.model.primary)
                                      -> TTS -> attach audio
```

## 斜杠命令用法

只有一个命令：`/tts`。
启用详情详见 [斜杠命令](/tools/slash-commands)。

Discord 注意：`/tts` 是 Discord 内置命令，因此创宇太虚在 Discord 上注册为 `/voice`。文本 `/tts ...` 仍然有效。

```bash
/tts off
/tts always
/tts inbound
/tts tagged
/tts status
/tts provider openai
/tts limit 2000
/tts summary off
/tts audio 你好，来自创宇太虚
```

注意事项：

- 命令需要已授权的发送者（白名单/所有者规则仍然适用）。
- `commands.text` 或原生命令注册必须已启用。
- `off|always|inbound|tagged` 是按会话的开关（`/tts on` 是 `/tts always` 的别名）。
- `limit` 和 `summary` 存储在本地偏好中，而非主配置。
- `/tts audio` 生成一次性音频回复（不会开启 TTS）。

## 智能体工具

`tts` 工具将文本转换为语音并返回 `MEDIA:` 路径。当结果兼容 Telegram 时，工具会包含 `[[audio_as_voice]]` 以便 Telegram 发送语音气泡。

## 网关 RPC

网关方法：

- `tts.status`
- `tts.enable`
- `tts.disable`
- `tts.convert`
- `tts.setProvider`
- `tts.providers`
