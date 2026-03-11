---
name: message
description: "Send text messages and manage channel operations across remote platforms"
tools: message
metadata:
  tree_id: "messaging/message"
  tree_group: "messaging"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Message — Channel Messaging & Operations

## Channel Verification Flow

1. `channels.status` — identify configured/unconfigured channels
2. `channels.save` — minimal config → re-check status
3. `send` — short text message (verify routing, error codes, receipt)
4. File/media → switch to `send_media` (separate validation required)
5. `poll` — verify inbound polling (channel must support it)
6. `channels.logout` — clear stale credentials

## Key Boundaries

- `send` = **text messages only**
- Files/images/PDFs → use `send_media` tool
- Text working ≠ all delivery works — must validate file delivery separately
- `save` success ≠ usable — first config may need gateway restart

## Reactions

- `emoji` required for all reaction operations
- Empty `emoji=""` removes bot's ALL reactions (Discord/Slack/Google Chat)
- `remove: true` removes specific emoji (requires non-empty `emoji`)
- Reaction notifications require `channels.signal.reactionNotifications` enabled

## Troubleshooting

- `send` success ≠ right destination — check `to`, alias, session routing
- Channel configured ≠ functional — always send test message after setup
