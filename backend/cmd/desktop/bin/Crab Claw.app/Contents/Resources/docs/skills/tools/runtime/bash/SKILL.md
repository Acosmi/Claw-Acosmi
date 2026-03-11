---
name: bash
description: "Execute shell commands with approval governance, allowlist enforcement, and elevated mode support"
tools: bash
metadata:
  tree_id: "runtime/bash"
  tree_group: "runtime"
  min_tier: "task_light"
  approval_type: "exec_escalation"
---

# Bash — Shell Command Execution

## Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `command` | yes | — | Shell command to execute |
| `host` | no | `sandbox` | Execution target: `sandbox` \| `gateway` \| `node` |
| `timeout` | no | 1800 | Seconds before kill |
| `yieldMs` | no | 10000 | Auto-background threshold (ms) |
| `pty` | no | false | Enable TTY mode for interactive CLIs |

## Approval Model

- **Sandbox** (`host=sandbox`): runs directly, no exec approvals required
- **Gateway** (`host=gateway`): subject to `exec-approvals.json` policy
- **Node** (`host=node`): subject to node-local approval governance

Policy knobs (from `~/.openacosmi/exec-approvals.json`):
- `security`: `deny` | `allowlist` | `full`
- `ask`: `off` | `on-miss` | `always`
- `askFallback`: `deny` | `allowlist` | `full` (when UI unavailable)

Effective policy = **stricter of** `tools.exec.*` config and approval defaults.

## Allowlist Rules

- Matches **resolved binary paths** only (not basename)
- Chaining (`;`, `&&`, `||`) and redirections rejected in allowlist mode
- **Safe bins** (stdin-only, no allowlist entry needed): `jq`, `grep`, `cut`, `sort`, `uniq`, `head`, `tail`, `tr`, `wc`

## Elevated Mode

- `/elevated on` or `/elevated ask` — run on gateway host, keep exec approvals
- `/elevated full` — run on gateway host, auto-approve (skip approvals)
- Per-session state; inline `/elevated` affects only that message
- Controlled by: `tools.elevated.enabled` (global) + per-agent + sender whitelist

## Security

- Host exec rejects `env.PATH` and loader overrides (`LD_*`/`DYLD_*`) to prevent binary hijacking
- Non-Windows: if `SHELL=fish`, prefers `bash`/`sh` from PATH
- Merges login-shell PATH for `host=gateway`; sandbox runs `sh -lc`
