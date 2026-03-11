---
name: stt-config
description: "Read, update, verify, and enumerate speech-to-text configuration through the dedicated stt_config tool"
tools: stt_config
metadata:
  tree_id: "system/stt_config"
  tree_group: "system"
  min_tier: "task_write"
  approval_type: "exec_escalation"
---

# STT Config

## Supported Actions

- `get`
- `set`
- `test`
- `models`

## Safe Workflow

1. `stt_config(action="get")`
2. Optionally inspect `models`
3. `stt_config(action="set", baseHash="<latest>", ...)`
4. `stt_config(action="test")` when connectivity must be verified

## Rules

- `set` requires the latest main config `baseHash`
- Omit `apiKey` to preserve the existing secret
- Prefer this tool over `gateway(action="stt.*")` when it is available
