---
name: sessions-send
description: "Send messages to other sessions or sub-agents, with CLI unattended execution support"
tools: sessions_send
metadata:
  tree_id: "sessions/sessions_send"
  tree_group: "sessions"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Sessions Send — Inter-Session Messaging

## Usage Guide

- Send a message to another session or sub-agent by session key
- Use `sessions_list` first to identify the target session

## CLI Unattended Mode

```bash
crabclaw agent --message "text" [--to DEST | --session-id ID | --agent ID] [--deliver]
```

| Flag | Description |
|------|-------------|
| `--to` | Derives session key (groups isolated, DMs collapse to `main`) |
| `--session-id` | Reuse existing session |
| `--agent` | Target configured agent's `main` session |
| `--deliver` | Send reply back to channel (requires `--channel`) |
| `--reply-to` / `--reply-channel` | Override delivery targets |
| `--thinking` / `--verbose` | Persist to session store |

- Gateway down → falls back to embedded local run
- Default output: reply text + `MEDIA:<url>` lines
- `--json`: structured payload
