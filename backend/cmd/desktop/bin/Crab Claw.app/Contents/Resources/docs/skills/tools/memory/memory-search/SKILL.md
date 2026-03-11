---
name: memory-search
description: "Search UHMS memory by keyword for semantic retrieval across long-context sessions"
tools: memory_search
metadata:
  tree_id: "memory/memory_search"
  tree_group: "memory"
  min_tier: "question"
  approval_type: "none"
---

# Memory Search — UHMS Semantic Retrieval

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `query` | yes | Search keyword or phrase |

## Usage Guide

- Searches the 太虚记忆系统 (UHMS) for relevant memory entries
- Available from `question` tier — can answer recall queries without task-level intent
- Returns ranked results by semantic relevance
- Use for: recalling past conversations, finding stored knowledge, checking what was discussed

## UHMS Operations Context

| Operation | Purpose |
|-----------|---------|
| `memory.list/get/delete` | Direct memory targeting |
| `memory.compress/commit` | Housekeeping and consolidation |
| `memory.uhms.search` | Quality validation after changes |
| `memory.vector.optimize` | Run after bulk writes |

## Best Practices

- After UHMS config changes, run search regression test
- After compression, verify key memories still retrievable
- Deletion requires pre-recording ID + content summary
