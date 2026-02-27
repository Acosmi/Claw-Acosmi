---
document_type: Audit
status: Completed
created: 2026-02-27
last_updated: 2026-02-27
scope: "多模态输入链路 + 飞书出站媒体发送能力 全面审计"
verdict: PASS with gaps (2 functional gaps identified, 1 design gap)
skill5_verified: true
---

# 审计报告：多模态链路 & 飞书出站媒体发送

## 审计背景

本次审计由用户提出以下两个问题触发：
1. 系统是否已完成多媒体格式转换链路集成（图片/文档/音频 → Markdown）？配置插件工具是否到位？
2. 系统是否具备向飞书发送图片、截图、文件、视频的能力？真实状态如何？

审计范围覆盖后端 Go 代码、前端 TypeScript 代码、已归档文档，并通过联网查阅飞书官方开放平台文档进行验证。

---

## 第一部分：多模态输入链路审计

### 1.1 总体状态

多模态输入链路已于历史 Phase 1-7 全部实现并通过审计（`audit-2026-02-26-multimodal-input.md` PASS）。

| 子系统 | 后端 | 前端配置 UI | 状态 |
|--------|------|------------|------|
| STT 语音转文字 | ✅ | ✅ `wizard-stt.ts` | 完整 |
| DocConv 文档转 Markdown | ✅ | ✅ `wizard-docconv.ts` | 完整 |
| 飞书入站多媒体解析 | ✅ | — | 完整 |
| 前端语音录制 | — | ✅ `voice-recorder.ts` | 完整 |
| 附件 chat.send | ✅ | ✅ | 完整 |

### 1.2 格式转换矩阵

| 输入格式 | 转换目标 | 实现方式 | 状态 |
|----------|---------|---------|------|
| `.txt` `.md` `.csv` `.json` `.yaml` `.xml` | Markdown | 内置直读 | ✅ |
| `.py` `.go` `.rs` `.ts` `.java` 等代码文件 | Markdown 代码块 | 内置语法高亮 | ✅ |
| `.html` `.docx` `.xlsx` `.pptx` `.pdf` | Markdown | MCP 插件（mcp-pandoc / mcp-document-converter / doc-ops-mcp） | ✅ |
| 音频 `.webm` `.opus` `.mp4` 等 | 文字（STT） | OpenAI Whisper / Groq / Azure / 本地 whisper.cpp | ✅ |
| 图片 | 原始传递给 Agent（多模态） | pass-through | ✅ |
| 视频 | ❌ 无内容提取 | pass-through（无 OCR/转录） | ⚠️ |
| `.epub` `.mobi` `.azw3` | 不支持 | 用户需求设计排除 | ❌ by design |

### 1.3 配置 Wizard 现状

两个配置 Wizard 均已实现，集成在 Settings 页面：

**`wizard-stt.ts`** — STT 配置向导
- 步骤式配置：provider → apiKey → model → language → 连通测试
- 支持 provider：openai / groq / azure / local-whisper
- RPC：`stt.config.get` / `stt.config.set` / `stt.test` / `stt.models`
- 连通测试按钮：`stt.test` RPC

**`wizard-docconv.ts`** — DocConv 配置向导
- 步骤式配置：provider → MCP 预设选择 → 参数 → 格式测试
- 支持 provider：mcp（3 个内置预设）/ builtin（Pandoc）
- MCP 预设：`mcp-pandoc` / `mcp-document-converter` / `doc-ops-mcp`
- RPC：`docconv.config.get` / `docconv.config.set` / `docconv.test` / `docconv.formats`

**结论**：配置插件工具完备，用户在 Settings 中完成配置即可启用。**无缺口。**

### 1.4 已知遗留问题（来自 deferred/multimodal-input.md）

**MEDIUM（8项）：**
- M-07：`sendMediaMessage` 下载时不检查 HTTP 状态码（404 被当有效数据上传）
- M-08：`http.DefaultClient` 无超时（**已在 plugin.go:391-401 修复**，使用 30s 自定义 client）
- M-09：`FeishuPlugin.Stop` 未取消 WebSocket goroutine（预存在）
- M-10：`DownloadResource` 未对 messageID/fileKey 做 `url.PathEscape()`
- M-11：`getTenantAccessToken` 无缓存，每次上传重新获取（限流风险）
- M-12：`stt.config.set` 无 provider 白名单验证
- M-14：前端 `MediaRecorder` 缺 `onerror` handler
- M-03：语音录制权限拒绝时无用户可见错误提示

**LOW（10项）：** 见 `deferred/multimodal-input.md`

---

## 第二部分：飞书出站媒体发送能力审计

### 2.1 已实现能力

**底层 SDK 发送方法（`client.go`）：**

| 方法 | 消息类型 | 状态 |
|------|---------|------|
| `SendTextMessage` | `text` | ✅ |
| `SendRichTextMessage` | `post`（富文本） | ✅ |
| `SendCardMessage` | `interactive`（卡片） | ✅ |
| `SendImageMessage` | `image` | ✅ |
| `SendAudioMessage` | `audio`（内联播放） | ✅ |
| `SendFileMessage` | `file` | ✅ |

**媒体上传（`resource.go`）：**

| 方法 | API | 大小限制 | 格式 |
|------|-----|---------|------|
| `UploadImage` | `POST /im/v1/images` | 10 MB | JPEG/PNG/WEBP/GIF/TIFF/BMP/ICO |
| `UploadFile` | `POST /im/v1/files` | 30 MB | opus/mp4/pdf/doc/xls/ppt/stream |

**自动媒体发送流程（`plugin.go:302-349`）：**
```
params.MediaURL → SSRF校验 → HTTP下载 → MIME检测 → UploadImage/UploadFile → Send*Message
```

安全防护：
- SSRF 防护 `validateMediaURL()`：拦截 loopback / 私有 IP / link-local
- 30 MB 上限读取（`readLimited`，超限返回错误，非静默截断）
- HTTP 超时 30s + 最多 5 次重定向（每次重定向均重新 SSRF 校验）

### 2.2 功能缺口

#### GAP-1：视频消息降级（⚠️ Low）

**位置**：`plugin.go:325-349`，`detectMediaCategory` + switch 语句

**现状**：
```go
// detectMediaCategory 对 video/* 返回 "video"
// 但 sendMediaMessage switch 无 "video" case
switch mediaCategory {
case "image": ...
case "audio": ...
default:  // ← video 走这里
    return sender.SendFile(...)  // 发成普通文件，非视频消息
}
```

**影响**：视频被发送为普通文件附件，飞书端无法内联播放。飞书确有 `video` msg_type，但需要额外的 `duration`、`cover_image_key` 等字段，实现复杂度较高。

**建议**：低优先级，当前行为（降级为 file）对用户尚可接受。

#### GAP-2：Agent 无法直接发送 binary 媒体（🔴 核心设计缺口）

**位置**：`channels/outbound.go:22-30`，`OutboundSendParams` 结构体

**现状**：
```go
type OutboundSendParams struct {
    MediaURL  string  // 只接受公网 URL
    // 无 MediaData []byte
    // 无 MediaBase64 string
}
```

Agent 通过 `send` RPC 发送媒体时，`handleSend` 只解析 `mediaUrl`/`mediaUrls` 参数，没有 binary 输入路径。

**影响**：Agent 生成的截图、动态生成的图表、oa-coder 工具输出的图片，**无法通过任何现有路径发送到飞书**。

---

## 第三部分：截图发送方案分析

### 3.1 方案对比

| 方案 | 描述 | 可行性 | 原因 |
|------|------|-------|------|
| `file://` URL | 用文件协议传路径 | ❌ | scheme 不是 http/https，第一关拦截 |
| `http://127.0.0.1` | 本地起 HTTP server | ❌ | SSRF 防护拦截 loopback IP |
| `http://192.168.x.x` | 局域网 IP | ❌ | SSRF 防护拦截私有 IP |
| 公网 CDN 中转 | 先上传公网再引用 | ✅ 可行 | 不优雅，需外部依赖 |
| `media.store` RPC | 存临时文件再引用 | ✅ 可行 | 三步操作，有临时文件管理 |
| **`send` 直接传 base64** | `MediaData []byte` | ✅ **推荐** | 最简单，单 RPC，无副作用 |

### 3.2 推荐方案：`send` RPC 扩展 base64 输入

**飞书 `post` 富文本 `img` tag（官方文档确认）：**
```json
{
  "tag": "img",
  "image_key": "d640eeea-4d2f-4cb3-88d8-c964fab53987",
  "width": 300,
  "height": 300
}
```
`image_key` 必须由 `UploadImage` API 上传后获得，不支持 base64 直嵌或 URL。

**推荐完整流程：**
```
Agent 截图(base64)
    ↓
send RPC: { mediaBase64: "...", mediaMimeType: "image/png" }
    ↓
handleSend: base64 decode → OutboundSendParams.MediaData
    ↓
sendMediaMessage: MediaData 非空 → 直接 UploadImage（跳过 HTTP 下载和 SSRF）
    ↓
image_key → build post JSON: [{tag:"img", image_key:"xxx"}]
    ↓
SendRichTextMessage → 飞书用户收到图片（内联在富文本中）
```

**变更范围（最小化）：**

| 文件 | 变更内容 | 行数估计 |
|------|---------|---------|
| `channels/outbound.go` | `OutboundSendParams` 加 `MediaData []byte` + `MediaMimeType string` | +5 行 |
| `gateway/server_methods_send.go` | `handleSend` 解析 `mediaBase64` → `base64.StdDecoding.DecodeString` → 填 `MediaData` | +15 行 |
| `channels/feishu/plugin.go` | `sendMediaMessage` 加 `if len(params.MediaData) > 0` 分支，跳过 HTTP 下载 | +12 行 |
| `channels/feishu/plugin.go` | `SendMessage` 向 `sendMediaMessage` 传 `params` 完整结构（或新参数） | +3 行 |

**总计约 35 行新增，不修改任何现有逻辑，不影响 SSRF 防护（URL 路径不变）。**

---

## 第四部分：安全审计摘要

| 检查项 | 结果 | 说明 |
|--------|------|------|
| SSRF 防护 | ✅ 有效 | `validateMediaURL` 拦截私有/回环 IP，重定向均重新校验 |
| 媒体大小限制 | ✅ 有效 | `readLimited` 超限返回错误（非截断） |
| HTTP 超时 | ✅ 有效 | 自定义 30s client，非 `http.DefaultClient` |
| base64 输入 XSS | ✅ 安全 | `UploadImage` 直接上传二进制，不渲染到前端 |
| base64 输入 OOM | ⚠️ 需注意 | 实现时需加大小限制（建议 ≤ 10 MB，匹配飞书 image 限制） |
| Token 缓存（M-11） | ⚠️ 遗留 | 每次上传重新拉 token，高频场景有限流风险 |
| URL 路径注入（M-10） | ⚠️ 遗留 | `DownloadResource` 未对 messageID/fileKey 做 PathEscape |

---

## 判定

**整体 PASS，含 2 项功能缺口：**

| 编号 | 类型 | 描述 | 优先级 |
|------|------|------|--------|
| GAP-1 | 功能降级 | 视频消息发送为文件，无内联播放 | 🟢 Low |
| GAP-2 | 设计缺口 | `send` RPC 无 binary/base64 输入路径，Agent 无法发送动态生成媒体 | 🔴 High |

**GAP-2 推荐修复方案**：`send` RPC 扩展 `mediaBase64` + `mediaMimeType` 参数，约 35 行改动，不破坏现有逻辑。

---

## 参考来源

- 飞书开放平台 - 发送消息内容结构：`https://open.feishu.cn/document/server-docs/im-v1/message-content-description/create_json`
- 飞书开放平台 - 发送富文本消息：`https://open.feishu.cn/document/ukTMukTMukTM/uMDMxEjLzATMx4yMwETM`
- 内部文档：`docs/claude/deferred/multimodal-input.md`
- 内部文档：`docs/claude/audit/audit-2026-02-26-multimodal-input.md`
- 代码审查：`backend/internal/channels/feishu/` 全目录
- 代码审查：`backend/internal/gateway/server_methods_send.go`
- 代码审查：`ui/src/ui/views/wizard-stt.ts` + `wizard-docconv.ts`
