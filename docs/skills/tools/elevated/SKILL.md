---
name: elevated
description: "提权执行模式和 /elevated 指令"
---

# 提权模式（/elevated 指令）

## 功能说明

- `/elevated on` 在 Gateway 宿主机上运行，保留 exec 审批（与 `/elevated ask` 相同）。
- `/elevated full` 在 Gateway 宿主机上运行，**并且**自动审批 exec（跳过 exec 审批）。
- `/elevated ask` 在 Gateway 宿主机上运行，保留 exec 审批（与 `/elevated on` 相同）。
- `on`/`ask` **不会**强制设置 `exec.security=full`；已配置的安全/审批策略仍然适用。
- 仅在 Agent **处于沙箱中**时改变行为（否则 exec 已经在宿主机上运行）。
- 指令格式：`/elevated on|off|ask|full`、`/elev on|off|ask|full`。
- 仅接受 `on|off|ask|full`；其他任何输入会返回提示且不改变状态。

## 控制范围（以及不控制的范围）

- **可用性控制**：`tools.elevated` 是全局基线。`agents.list[].tools.elevated` 可以进一步限制每个 Agent 的提权（两者都必须允许）。
- **每会话状态**：`/elevated on|off|ask|full` 为当前会话键设置提权级别。
- **内联指令**：消息中的 `/elevated on|ask|full` 仅对该消息生效。
- **群组**：在群聊中，提权指令仅在 Agent 被提及时生效。仅包含命令且绕过提及要求的消息被视为已提及。
- **宿主执行**：提权强制 `exec` 在 Gateway 宿主机上运行；`full` 还会设置 `security=full`。
- **审批**：`full` 跳过 exec 审批；`on`/`ask` 在白名单/审批规则要求时遵守审批。
- **非沙箱 Agent**：对执行位置无影响；仅影响控制、日志和状态。
- **工具策略仍然适用**：如果 `exec` 被工具策略拒绝，则无法使用提权。
- **与 `/exec` 分离**：`/exec` 为授权发送者调整每会话默认值，不需要提权。

## 解析顺序

1. 消息上的内联指令（仅对该消息生效）。
2. 会话覆盖（通过发送仅包含指令的消息设置）。
3. 全局默认值（配置中的 `agents.defaults.elevatedDefault`）。

## 设置会话默认值

- 发送一条**仅包含**指令的消息（允许空白），例如 `/elevated full`。
- 发送确认回复（`Elevated mode set to full...` / `Elevated mode disabled.`）。
- 如果提权访问被禁用或发送者不在已批准的白名单中，指令会回复可操作的错误信息且不改变会话状态。
- 发送 `/elevated`（或 `/elevated:`）且不带参数可查看当前提权级别。

## 可用性 + 白名单

- 功能开关：`tools.elevated.enabled`（即使代码支持，默认也可以通过配置关闭）。
- 发送者白名单：`tools.elevated.allowFrom`，按 Provider 设置白名单（如 `discord`、`whatsapp`）。
- 每 Agent 开关：`agents.list[].tools.elevated.enabled`（可选；只能进一步限制）。
- 每 Agent 白名单：`agents.list[].tools.elevated.allowFrom`（可选；设置后，发送者必须同时匹配全局 + 每 Agent 白名单）。
- Discord 回退：如果省略了 `tools.elevated.allowFrom.discord`，则使用 `channels.discord.dm.allowFrom` 列表作为回退。设置 `tools.elevated.allowFrom.discord`（即使为 `[]`）可覆盖。每 Agent 白名单**不**使用回退。
- 所有控制条件都必须通过；否则提权被视为不可用。

## 日志 + 状态

- 提权 exec 调用以 info 级别记录日志。
- 会话状态包含提权模式（如 `elevated=ask`、`elevated=full`）。
