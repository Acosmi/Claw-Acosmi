---
name: apply-patch
description: "Apply structured multi-file patches (add/update/delete/move) with plan confirmation"
tools: apply_patch
metadata:
  tree_id: "fs/apply_patch"
  tree_group: "fs"
  min_tier: "task_multimodal"
  approval_type: "plan_confirm"
---

# Apply Patch — Structured Multi-File Editing

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `input` | yes | Complete patch content with delimiters |

## Patch Format

```
*** Begin Patch
*** Update File: path/to/file.go
 context line
-old line
+new line
 context line

*** Add File: path/to/new_file.go
+line 1
+line 2

*** Delete File: path/to/obsolete.go

*** Move to: new/path/renamed.go
*** Update File: old/path/original.go
 context
-old
+new

*** End Patch
```

## Operations

| Operation | Syntax | Description |
|-----------|--------|-------------|
| Update | `*** Update File: path` | Modify existing file with context + diff |
| Add | `*** Add File: path` | Create new file (all lines prefixed `+`) |
| Delete | `*** Delete File: path` | Remove file entirely |
| Move | `*** Move to: new_path` before Update | Rename + modify |

## Rules

- `*** End of File` marks append-only sections
- Requires **plan confirmation**
- Experimental; disabled by default — enable via `tools.exec.applyPatch.enabled`
- Workspace-scoped writes only
