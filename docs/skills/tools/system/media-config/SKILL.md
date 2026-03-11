---
name: media-config
description: "Read and update media-agent configuration through the dedicated media_config tool"
tools: media_config
metadata:
  tree_id: "system/media_config"
  tree_group: "system"
  min_tier: "task_write"
  approval_type: "exec_escalation"
---

# Media Config

## Supported Actions

- `get`
- `update`

## Safe Workflow

1. `media_config(action="get")`
2. Read the latest `hash`
3. `media_config(action="update", baseHash="<latest>", ...)`
4. Check `result.validation`, `result.verification`, and the new `hash`

## Rules

- `update` requires the latest main config `baseHash`
- Omit secret subfields to preserve existing credentials
- Prefer this tool over `gateway(action="media.config.*")` when it is available
