---
document_type: Deferred
status: Active
created: 2026-02-26
last_updated: 2026-02-27
source: docs/claude/audit/audit-2026-02-26-multimodal-input.md
---

# 多模态输入 — 延迟项

## ✅ 已修复

### GAP-2: send RPC 无 binary/base64 输入路径（已修复 2026-02-27）
- **审计报告**: `audit/audit-2026-02-27-multimedia-feishu-outbound.md`
- **修复内容**: `OutboundSendParams` 新增 `MediaData []byte` + `MediaMimeType string`；
  `handleSend` 解析 `mediaBase64` 参数并路由到 `ChannelMgr`；
  `FeishuPlugin` 新增 `sendMediaData` 方法直接上传二进制到飞书。
- **文件**: `channels/outbound.go`, `gateway/server_methods_send.go`, `channels/feishu/plugin.go`

## MEDIUM (8 项)

### M-07: sendMediaMessage 缺少 HTTP 状态码检查
- **文件**: `feishu/plugin.go`
- **影响**: 404/500 响应被当作有效媒体上传到飞书
- **修复**: 检查 `resp.StatusCode != 200`

### M-08: http.DefaultClient 无超时
- **文件**: `feishu/resource.go`, `feishu/plugin.go`
- **影响**: slowloris 攻击可无限挂起 HTTP 连接
- **修复**: 创建包级 `httpClient = &http.Client{Timeout: 30s}`

### M-09: FeishuPlugin.Stop 未取消 WebSocket goroutine
- **文件**: `feishu/plugin.go:200-215`
- **影响**: Stop 后 WebSocket goroutine 泄漏
- **修复**: 调用 `wsCancel[accountID]()` 并清理 wsClients/wsCancel
- **注**: 预存在问题

### M-10: DownloadResource URL 路径注入
- **文件**: `feishu/resource.go:46-47`
- **影响**: messageID/fileKey 含特殊字符可改变 API 路径
- **修复**: 使用 `url.PathEscape()`

### M-11: getTenantAccessToken 无缓存
- **文件**: `feishu/resource.go`
- **影响**: 每次上传/下载都重新获取 token，可能触发飞书限流
- **修复**: 添加 `sync.Mutex` 保护的缓存 + 过期检查
- **注**: 预存在问题

### M-12: stt.config.set 无输入验证
- **文件**: `gateway/server_methods_stt.go:75-113`
- **影响**: 可写入任意 provider 字符串、SSRF via BaseURL
- **修复**: Provider 白名单验证 + BaseURL HTTPS 检查

### M-14: MediaRecorder 缺少 onerror handler
- **文件**: `ui/controllers/voice-recorder.ts`
- **影响**: 录音期间 OS 错误可导致 stop() Promise 永远不 resolve
- **修复**: 添加 `recorder.onerror` → cleanup + reject

### M-03 (补充): Silent failure on voice start error
- **文件**: `ui/app.ts:507-519`
- **影响**: 用户点击麦克风无反应 (权限拒绝时无提示)
- **修复**: 在 catch 中设置 lastError 显示用户可见错误

## LOW (10 项)

- L-01: truncateStr 按字节截断可能破坏 UTF-8 (server_methods_chat.go)
- L-02: detectImageMediaType 未知格式默认 image/png (server_multimodal.go)
- L-03: STT handler 超时未继承调用者 context (server_methods_stt.go)
- L-04: processAttachmentsForChat 每次调用重建 provider (server_methods_chat.go)
- L-05: 录音无最大时长限制 (voice-recorder.ts)
- L-06: 录音 blob 无大小检查 (app.ts)
- L-07: 计时器 500ms 间隔 vs 秒级显示不匹配 (voice-recorder.ts)
- L-08: 消息去重清理用 goroutine 而非 time.AfterFunc (plugin.go, 预存在)
- L-09: 上传文件名硬编码 audio.opus/file (plugin.go)
- L-10: detectMediaCategory magic bytes 检查有限 (plugin.go)
