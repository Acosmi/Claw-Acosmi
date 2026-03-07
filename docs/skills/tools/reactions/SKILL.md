---
name: reactions
description: "跨渠道共享的 Reaction 语义"
---

# Reaction 工具

跨渠道共享的 Reaction 语义：

- 添加 Reaction 时 `emoji` 为必填。
- `emoji=""` 在支持的渠道中移除机器人的 Reaction。
- `remove: true` 在支持的渠道中移除指定的 emoji（需要提供 `emoji`）。

渠道说明：

- **Discord/Slack**：空 `emoji` 移除机器人在该消息上的所有 Reaction；`remove: true` 仅移除指定的 emoji。
- **Google Chat**：空 `emoji` 移除应用在该消息上的 Reaction；`remove: true` 仅移除指定的 emoji。
- **Telegram**：空 `emoji` 移除机器人的 Reaction；`remove: true` 也会移除 Reaction，但工具验证仍需要非空的 `emoji`。
- **WhatsApp**：空 `emoji` 移除机器人的 Reaction；`remove: true` 映射为空 emoji（仍需提供 `emoji`）。
- **Signal**：当 `channels.signal.reactionNotifications` 启用时，入站 Reaction 通知会触发系统事件。
