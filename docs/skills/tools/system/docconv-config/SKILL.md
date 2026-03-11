---
name: docconv-config
description: "Read, update, verify, and enumerate document-conversion configuration through the dedicated docconv_config tool"
tools: docconv_config
metadata:
  tree_id: "system/docconv_config"
  tree_group: "system"
  min_tier: "task_write"
  approval_type: "exec_escalation"
---

# DocConv Config

## Supported Actions

- `get`
- `set`
- `test`
- `formats`

## Safe Workflow

1. `docconv_config(action="get")`
2. Optionally inspect `formats`
3. `docconv_config(action="set", baseHash="<latest>", ...)`
4. `docconv_config(action="test")` when verification is needed

## Rules

- `set` requires the latest main config `baseHash`
- Use narrow field updates instead of generic `config.patch` when only DocConv changes
- Prefer this tool over `gateway(action="docconv.*")` when it is available
