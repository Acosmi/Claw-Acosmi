---
name: sessions-list
description: "List active sessions with filters and pagination for session management"
tools: sessions_list
metadata:
  tree_id: "sessions/sessions_list"
  tree_group: "sessions"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Sessions List

## Usage Guide

- Lists other sessions with optional filters (agent, channel, status)
- Supports pagination for large session sets
- Use to locate target sessions before `sessions_send` or `sessions_history`
- No approval required
