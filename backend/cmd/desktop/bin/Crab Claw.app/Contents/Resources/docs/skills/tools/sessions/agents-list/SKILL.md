---
name: agents-list
description: "List available agent IDs and their configurations for session spawning"
tools: agents_list
metadata:
  tree_id: "sessions/agents_list"
  tree_group: "sessions"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Agents List

## Usage Guide

- Lists all configured agent IDs available for `sessions_spawn`
- Returns agent names, types, and configuration summaries
- Use before spawning to verify target agent exists and is properly configured
- No approval required
