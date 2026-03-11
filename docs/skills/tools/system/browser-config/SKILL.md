---
name: browser-config
description: "Read and update browser configuration through the dedicated browser_config tool"
tools: browser_config
metadata:
  tree_id: "system/browser_config"
  tree_group: "system"
  min_tier: "task_write"
  approval_type: "exec_escalation"
---

# Browser Config

## Supported Actions

- `get`
- `set`

## Safe Workflow

1. `browser_config(action="get")`
2. Read `hash`, current snapshot, and any warnings
3. `browser_config(action="set", baseHash="<latest>", ...)`
4. Check `result.validation`, `result.verification`, and new `hash`

## Rules

- `set` requires the latest `baseHash`
- Only update browser-scoped fields such as `enabled`, `cdpUrl`, `evaluateEnabled`, `headless`
- Prefer this tool over `gateway(action="tools.browser.*")` when it is available
