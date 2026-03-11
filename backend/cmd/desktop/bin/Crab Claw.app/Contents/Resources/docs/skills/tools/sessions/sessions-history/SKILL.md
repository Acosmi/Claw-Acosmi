---
name: sessions-history
description: "Fetch conversation history for another session or sub-agent for review"
tools: sessions_history
metadata:
  tree_id: "sessions/sessions_history"
  tree_group: "sessions"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Sessions History

## Usage Guide

- Retrieves transcript/history from another session or sub-agent
- Use for reviewing past conversations, debugging sub-agent behavior, or auditing
- Requires knowing the target session key (use `sessions_list` first)
- No approval required
