---
name: remote-approval-config
description: "Read, update, and verify remote approval configuration through the dedicated remote_approval_config tool"
tools: remote_approval_config
metadata:
  tree_id: "system/remote_approval_config"
  tree_group: "system"
  min_tier: "task_write"
  approval_type: "exec_escalation"
---

# Remote Approval Config

## Supported Actions

- `get`
- `set`
- `test`

## Safe Workflow

1. `remote_approval_config(action="get")`
2. Use the returned dedicated `hash`
3. `remote_approval_config(action="set", baseHash="<latest>", ...)`
4. `remote_approval_config(action="test", provider="...")` when verification is needed

## Rules

- The `baseHash` here is the remote-approval config hash, not the generic `config.get` hash
- Omit secret subfields to preserve existing credentials
- Prefer this tool over `gateway(action="security.remoteApproval.*")` when it is available
