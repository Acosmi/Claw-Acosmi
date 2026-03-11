---
name: sessions-spawn
description: "Spawn sub-agent sessions (coder/media/custom) with delegation contracts and scope control"
tools: sessions_spawn
metadata:
  tree_id: "sessions/sessions_spawn"
  tree_group: "sessions"
  min_tier: "task_multimodal"
  approval_type: "plan_confirm"
---

# Sessions Spawn — Sub-Agent Delegation

## When to Delegate

| Target | Use Case | Entry Point |
|--------|----------|-------------|
| **Open Coder** | Multi-file edits, refactoring, test writing | `spawn_coder_agent` |
| **Media Agent** | Hot topics, content creation, multi-platform publishing | `spawn_media_agent` |
| **Custom** | Any configured agent from `agents_list` | `sessions_spawn` |

## Coder Contract

```
spawn_coder_agent(
  task_brief = "clear, specific description",
  scope = "path/to/allowed/files",
  constraints = ["no_network", "read_only"]
)
```

Return states: `completed` | `partial` (check artifacts) | `needs_auth` (evaluate risk)
Max 3 negotiation rounds.

**When NOT to delegate**: single file <50 lines, simple fix → direct `write_file`. Tests/build/lint → direct `bash`.

## Media Agent Contract

```
spawn_media_agent(
  task_brief = "≤500 chars, clear topic/platform",
  scope = [{path: "dir/", permissions: ["read", "write"]}],
  constraints = ["no_network"]
)
```

All publish operations need approval (no auto-publish). Handles content creation, NOT coding or desktop UI.

## Rules

- Requires **plan confirmation**
- Sub-agents run in isolated LLM sessions
- Monotonic permission decay: sub-agent cannot exceed parent's permissions
- Simple file forwarding → use `send_media` directly, not media agent
