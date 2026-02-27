---
document_type: Audit
status: Completed
created: 2026-02-27
last_updated: 2026-02-27
scope: "GAP-2 修复：send RPC 扩展 mediaBase64 支持"
verdict: PASS
skill5_verified: true
---

# 审计报告：GAP-2 修复 — send RPC mediaBase64 扩展

## 审计范围

对 GAP-2（Agent 无法直接发送 binary 媒体到飞书）修复代码的安全性、正确性和资源安全审计。

修改文件：
1. `backend/internal/channels/outbound.go` — `OutboundSendParams` 新增字段
2. `backend/internal/gateway/server_methods_send.go` — `handleSend` 解析 `mediaBase64`
3. `backend/internal/channels/feishu/plugin.go` — `SendMessage` + `sendMediaData`

---

## 逐项审计

### F-01: `OutboundSendParams` 新增字段

**位置**: `channels/outbound.go:30-34`

**变更**: 新增 `MediaData []byte` + `MediaMimeType string`

**分析**:
- 纯数据结构扩展，zero value 为 nil/""
- 不影响现有代码（所有现有调用者不设置这些字段，zero value 不触发新逻辑）
- 向后兼容，无破坏性变更

**判定**: ✅ 安全

### F-02: `handleSend` 解析 mediaBase64

**位置**: `gateway/server_methods_send.go:123-162`

**变更**: 解析 `mediaBase64` + `mediaMimeType` 参数，base64 解码后通过 `ChannelMgr.SendMessage` 发送

**安全检查**:
- ✅ base64 解码错误处理：`DecodeString` 失败返回 BadRequest
- ✅ 大小限制：10 MB 上限（`maxMediaBase64Size = 10 * 1024 * 1024`），匹配飞书 image API 限制
- ✅ OOM 防护：先解码再检查长度，解码后最大 10 MB，安全范围
- ✅ 路径隔离：仅在 `channelID != "chat"` 且 `ChannelMgr != nil` 时触发
- ✅ 不影响 SSRF 防护：binary 路径完全跳过 URL 下载，不经过 `validateMediaURL`
- ⚠️ base64 编码膨胀：10 MB 解码数据 ≈ 13.3 MB base64 字符串，RPC JSON 解析时内存峰值约 27 MB

**注意事项**:
- `base64.StdEncoding.DecodeString` 会将整个字符串读入内存再解码，对于大文件有 2x 内存开销
- 但 10 MB 限制（解码后）控制了最大内存为 ~27 MB，可接受

**判定**: ✅ 安全，内存使用可控

### F-03: `sendMediaData` 方法

**位置**: `channels/feishu/plugin.go:302-336`

**变更**: 新增 `sendMediaData` 方法，接收二进制数据直接调用 UploadImage/UploadFile

**安全检查**:
- ✅ 无 HTTP 下载：不经过网络获取，无 SSRF 风险
- ✅ 复用现有 `detectMediaCategory`：MIME + magic bytes 双重检测
- ✅ 复用现有 `UploadImage`/`UploadFile`/`SendImage`/`SendAudio`/`SendFile`：已审计的方法
- ✅ 错误处理：所有分支返回 `fmt.Errorf("upload ...: %w", err)`，保留上下文
- ✅ 无 panic 路径：零 panic 策略合规

**判定**: ✅ 安全

### F-04: `SendMessage` 新增 MediaData 分支

**位置**: `channels/feishu/plugin.go:274-286`

**变更**: 在 MediaURL 检查之前，优先检查 `MediaData`

**分析**:
- ✅ 优先级正确：`MediaData` > `MediaURL` > 纯文本
- ✅ fallback 逻辑：binary 发送失败 → 降级到文字发送（与 MediaURL 行为一致）
- ✅ 同时有文字时追加发送（与现有行为一致）

**判定**: ✅ 正确

---

## 安全总结

| 检查项 | 结果 |
|--------|------|
| SSRF 影响 | ✅ 无影响（binary 路径完全绕过 URL 下载） |
| OOM 风险 | ✅ 10 MB 限制控制内存 |
| 输入验证 | ✅ base64 格式验证 + 大小限制 |
| XSS/注入 | ✅ 无渲染到前端，直接上传到飞书 API |
| 向后兼容 | ✅ 新字段 zero value 不触发新逻辑 |
| 零 panic | ✅ 所有错误路径用 error 返回 |
| 资源泄漏 | ✅ 无需 cleanup（[]byte 由 GC 回收） |

---

## 判定

**PASS** — 变更安全、正确、最小化。

### 遗留注意事项
- L-NEW-1: base64 编码膨胀导致 RPC JSON 包较大（最大 ~13.3 MB），可考虑未来增加 multipart upload 支持
- GAP-1（视频降级为文件）仍为 Low 优先级，未修复
