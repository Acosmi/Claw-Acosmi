---
name: session-status
description: "Show session status card with usage metrics, time, mode, and cost analysis"
tools: session_status
metadata:
  tree_id: "sessions/session_status"
  tree_group: "sessions"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Session Status — Usage & Diagnostics

## Usage Guide

- Displays session status card: usage stats, elapsed time, current mode
- No approval required

## Usage Analytics Context

| Operation | Purpose |
|-----------|---------|
| `usage.status` | Subsystem health check |
| `sessions.usage` | Session-level totals (tokens/cost/messages) |
| `sessions.usage.timeseries` | Peak/valley analysis |
| `sessions.usage.logs` | Anomaly window forensics |
| `usage.cost` | Provider/model aggregation |

## Rules

- Cost analysis must cite time range + methodology
- Do not cross-compare with different price tables
