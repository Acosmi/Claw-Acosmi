---
name: gateway
description: "Use the real gateway contract for generic config, narrow-domain config, restart, and updates"
tools: gateway
metadata:
  tree_id: "system/gateway"
  tree_group: "system"
  min_tier: "task_multimodal"
  approval_type: "exec_escalation"
---

# Gateway — Executable Configuration Contract

`gateway` 现在保留为通用配置与系统动作入口，同时也是专项配置 RPC 的兼容回退层。
如果当前 runtime 还暴露了顶层专项工具，优先使用：
- `browser_config`
- `remote_approval_config`
- `image_config`
- `stt_config`
- `docconv_config`
- `media_config`

## Supported Actions

- Generic config:
  - `config.get`
  - `config.schema`
  - `config.set`
  - `config.patch`
  - `config.apply`
- Browser:
  - `tools.browser.get`
  - `tools.browser.set`
- Remote approval:
  - `security.remoteApproval.config.get`
  - `security.remoteApproval.config.set`
  - `security.remoteApproval.test`
- Image:
  - `image.config.get`
  - `image.config.set`
  - `image.test`
  - `image.models`
  - `image.ollama.models`
- STT:
  - `stt.config.get`
  - `stt.config.set`
  - `stt.test`
  - `stt.models`
- DocConv:
  - `docconv.config.get`
  - `docconv.config.set`
  - `docconv.test`
  - `docconv.formats`
- Media:
  - `media.config.get`
  - `media.config.update`
- System / update:
  - `restart`
  - `update.run`

## Real Contract

- `config.get` returns a redacted snapshot with `hash`, `raw`, `parsed`, `config`, `issues`
- `config.schema` returns JSON Schema + `uiHints`
- `config.set` / `config.apply` / `config.patch` all use:
  - `raw`: JSON5 string
  - `baseHash`: latest hash from `config.get`
- `config.patch` requires `raw` to be an object merge patch
- `config.apply` and `config.patch` may also take:
  - `sessionKey`
  - `note`
  - `restartDelayMs`
- `restart` is the gateway tool action name; it maps to backend RPC `system.restart`
  - optional params: `reason`, `delayMs`
- Main-config narrow getters return a current config `hash`:
  - `tools.browser.get`
  - `image.config.get`
  - `stt.config.get`
  - `docconv.config.get`
  - `media.config.get`
- `security.remoteApproval.config.get` returns a dedicated remote-approval `hash` for its own config file
- Narrow write actions are field-level merges, not full-document replacements:
  - `tools.browser.set`
  - `security.remoteApproval.config.set`
  - `image.config.set`
  - `stt.config.set`
  - `docconv.config.set`
  - `media.config.update`
- Agent-side safe contract: always pass the latest `baseHash` before any write, including narrow writes

## Safe Procedure

1. Choose the narrowest action family that matches the target:
   - generic document edit: `config.*`
   - specialized top-level tool when available: `browser_config` / `remote_approval_config` / `image_config` / `stt_config` / `docconv_config` / `media_config`
   - fallback inside `gateway`: `tools.browser.*` / `security.remoteApproval.*` / `image.*` / `stt.*` / `docconv.*` / `media.config.*`
2. Read first:
   - generic: `config.get` and usually `config.schema`
   - narrow: the relevant `*.config.get`
3. For generic writes, pick one path:
   - `config.patch` for minimal diff + immediate apply
   - `config.apply` for full replacement + immediate apply
   - `config.set` for full replacement without restart
4. For narrow writes, call the dedicated setter with the latest `baseHash`
5. If the domain supports it, run verification helpers after write:
   - `security.remoteApproval.test`
   - `image.test`
   - `stt.test`
   - `docconv.test`
6. Check returned `validation`, `verification`, `snapshot`, `hash`
7. Carry forward the new `hash` before any further write

## Rules

- Do not `config.patch` and then `config.apply` for the same change. `config.patch` already writes and schedules restart.
- Do not mix generic `config.patch` with a dedicated domain setter for the same narrow change unless the user explicitly wants a whole-document edit.
- Prefer `tools.browser.set` / `image.config.set` / `stt.config.set` / `docconv.config.set` / `media.config.update` / `security.remoteApproval.config.set` over generic `config.patch` when only that domain is being changed.
- If `browser_config` / `remote_approval_config` / `image_config` / `stt_config` / `docconv_config` / `media_config` are visible in the current runtime, prefer them over the equivalent `gateway` subactions.
- Avoid touching multiple high-risk blocks in one call.
- Prefer minimal diff for sensitive values.
- Requires **exec_escalation** approval.

## Boundaries

- This skill is about the executable `gateway` tool only.
- Do not assume `health`, `status`, `logs.tail`, `system.reset`, `system.backup.restore` are available via this tool.
- Browser / remote approval / media / image / stt / docconv are already exposed here as dedicated narrow gateway actions.
