---
name: read-file
description: "Read file contents with path validation and global read access"
tools: read_file
metadata:
  tree_id: "fs/read_file"
  tree_group: "fs"
  min_tier: "task_light"
  approval_type: "none"
---

# Read File

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `path` | yes | Absolute or workspace-relative file path |
| `from` | no | Start line number (1-based) |
| `lines` | no | Number of lines to read |

## Usage Guide

- Reads file content and returns it as text
- Supports partial reads via `from` + `lines` for large files
- Has **global read** access — can read files outside workspace
- No approval required; safe for all intent tiers
- Binary files return base64 or error depending on type
- Use `list_dir` first if unsure about file location
