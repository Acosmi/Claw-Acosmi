---
name: write-file
description: "Create or overwrite files within workspace scope, requires plan confirmation"
tools: write_file
metadata:
  tree_id: "fs/write_file"
  tree_group: "fs"
  min_tier: "task_write"
  approval_type: "plan_confirm"
---

# Write File

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `path` | yes | Target file path (workspace-scoped) |
| `content` | yes | Full file content to write |

## Usage Guide

- Creates new files or fully overwrites existing ones
- Scoped to workspace — cannot write outside workspace boundary
- Requires **plan confirmation** before execution
- For partial edits, prefer `apply_patch` (structured diff format)
- For single-line changes, direct `bash` with sed/echo may be simpler
- Always confirm the write path with user before creating files in new locations
