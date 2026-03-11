---
name: image-config
description: "Read, update, verify, and enumerate image-understanding configuration through the dedicated image_config tool"
tools: image_config
metadata:
  tree_id: "system/image_config"
  tree_group: "system"
  min_tier: "task_write"
  approval_type: "exec_escalation"
---

# Image Config

## Supported Actions

- `get`
- `set`
- `test`
- `models`
- `ollama_models`

## Safe Workflow

1. `image_config(action="get")`
2. Optionally inspect `models` or `ollama_models`
3. `image_config(action="set", baseHash="<latest>", ...)`
4. `image_config(action="test")` when connectivity must be verified

## Rules

- `set` requires the latest main config `baseHash`
- Omit `apiKey` to preserve the existing secret
- Prefer this tool over `gateway(action="image.*")` when it is available
