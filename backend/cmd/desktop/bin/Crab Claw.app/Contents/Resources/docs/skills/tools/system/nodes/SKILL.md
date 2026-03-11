---
name: nodes
description: "List, describe, notify and control paired node devices with token lifecycle management"
tools: nodes
metadata:
  tree_id: "system/nodes"
  tree_group: "system"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Nodes — Device Pairing & Remote Control

## Standard Flow

1. `node.pair.list` — verify pending pairing requests
2. Approve trusted, reject unknown sources
3. `node.describe` — validate connectivity (`connected=true` + capability fields)
4. `node.invoke` — execute whitelisted commands on remote node
5. `node.invoke.result` — collect execution results

## Critical Boundaries

- **Paired ≠ callable**: always check `connected=true` before invoke
- **Node connectivity ≠ approval governance**: Gateway does NOT proxy `exec.approvals.node.*` remotely
- **Token rotation ≠ revocation**: suspected leaks need both rotate AND revoke

## Token Lifecycle

- Rotate tokens regularly on schedule
- Revoke immediately on suspected compromise
- Node ops (pairing, invocation) are separate from node approval file governance
