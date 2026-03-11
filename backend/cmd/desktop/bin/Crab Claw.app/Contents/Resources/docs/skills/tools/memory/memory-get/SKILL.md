---
name: memory-get
description: "Get specific memory entry by ID or path from UHMS storage"
tools: memory_get
metadata:
  tree_id: "memory/memory_get"
  tree_group: "memory"
  min_tier: "question"
  approval_type: "none"
---

# Memory Get — Direct Entry Retrieval

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `path` | yes | Memory entry path or ID |
| `from` | no | Start line for partial read |
| `lines` | no | Number of lines to return |

## Usage Guide

- Retrieves a specific memory entry by its path/ID
- Available from `question` tier — no approval required
- Use after `memory_search` to get full content of a found entry
- Supports partial reads via `from` + `lines` for large entries
