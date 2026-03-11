---
name: send-email
description: "Send email messages with new/reply threading, multi-account support, and frequency limits"
tools: send_email
metadata:
  tree_id: "media/send_email"
  tree_group: "media"
  min_tier: "task_write"
  approval_type: "plan_confirm"
---

# Send Email — Email Composition & Delivery

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `to` | yes | Recipient email address |
| `subject` | yes | Subject line |
| `body` | yes | Plain text body |
| `account` | no | Sender account ID (default: defaultAccount) |
| `reply_to_session` | no | Session key to auto-restore thread headers |
| `cc` | no | Comma-separated CC addresses |

## Limitations

- **Plain text only** — no HTML rich text
- **No outbound attachments** (V1 supports inbound only)
- No draft/calendar/contact operations
- Per-account frequency limits (hourly/daily)
- Unavailable under NoNetwork delegation

## Workflow

1. User requests email → confirm subject + body
2. Call `send_email` with parameters
3. Return result (success or failure reason)

## Safety

- Auto-adds `Auto-Submitted` header to prevent mail loops
- Requires **plan confirmation** before sending
