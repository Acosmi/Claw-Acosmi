---
name: list-dir
description: "List directory contents with global read access for file discovery"
tools: list_dir
metadata:
  tree_id: "fs/list_dir"
  tree_group: "fs"
  min_tier: "task_light"
  approval_type: "none"
---

# List Directory

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `path` | yes | Directory path to list |

## Usage Guide

- Lists files and subdirectories at the given path
- Has **global read** access — can list directories outside workspace
- No approval required
- Use before `read_file` when unsure about exact file names or structure
- Returns file names, sizes, and modification times
