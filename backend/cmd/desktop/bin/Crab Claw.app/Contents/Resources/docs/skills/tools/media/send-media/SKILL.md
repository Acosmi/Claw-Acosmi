---
name: send-media
description: "向远程频道发送文件和媒体（飞书/Discord/Telegram/WhatsApp），单文件上限 30MB"
tools: send_media
metadata:
  tree_id: "media/send_media"
  tree_group: "media"
  min_tier: "task_light"
  approval_type: "data_export"
---

# send_media

`send_media` 用于把文件或媒体发送到当前会话频道，或显式指定的其他远程频道。

适用场景：

- 把工作区中的图片、PDF、报告、音视频发到飞书/Discord/Telegram/WhatsApp
- 把刚上传到当前聊天里的图片、音频、视频继续转发到远程频道
- 给媒体附带一段说明文字一起发送

边界：

- 这是“文件/媒体投递”工具，不负责生成文件
- 如果文件还不存在，先用其他工具生成或定位，再调用 `send_media`
- 如果只是发送纯文本，优先使用文本消息工具，不要绕 `send_media`

## 参数

| 参数 | 是否必填 | 说明 |
|-----------|----------|-------------|
| `file_path` | 推荐 | 工具可访问范围内的绝对路径 |
| `media_base64` | 备选 | Base64 媒体数据，仅在文件本身已在内存中时使用 |
| `file_name` | 否 | 指定远程文件名；未提供时自动推断 |
| `target` | 否 | `channel:id` 格式；留空时默认当前会话频道 |
| `mime_type` | 否 | MIME 类型；`file_path` 模式下通常自动识别 |
| `message` | 否 | 随文件一起发送的说明文字 |

## 使用规则

1. 优先使用 `file_path`
   已知本地绝对路径时，直接传 `file_path`，最稳定。
2. `media_base64` 只作为备选
   只有媒体数据本来就在内存中、没有可复用文件路径时才使用。
3. 当前聊天附件可直接复用
   如果用户指的是“刚刚发的那张图/那段音频/那个视频”，可以同时省略 `file_path` 和 `media_base64`，工具会优先复用当前轮附件，再回退到 transcript 中最近的可复用媒体。
4. `target` 默认当前频道
   只有用户明确要求发到“另一个频道/群/会话”时，才设置 `target`。

## 约束

- 单文件硬上限 **30MB**
- 发送本地文件需要 **data_export** 审批；若路径超出当前可访问范围，还会触发挂载/路径权限检查
- 默认使用路径 basename 作为远程文件名
- 常见图片、PDF、Office 文档可自动识别 MIME 类型

## 故障排查

| 错误 | 原因 | 处理 |
|-------|-------|-----|
| `"Media sender not available"` | 当前上下文没有可用的频道发送器 | 检查频道是否已配置并启用 |
| `"No target and no session channel"` | 既没有当前会话路由，也没显式传 `target` | 明确指定 `target` |
| 路径访问失败 | 文件路径超出权限范围或需要挂载审批 | 检查绝对路径、挂载范围和审批状态 |
| 超过 30MB | 文件过大 | 压缩、分片，或改为其他投递方式 |
