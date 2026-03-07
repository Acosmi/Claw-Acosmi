---
name: thinking
description: "/think + /verbose 指令语法及其对模型推理的影响"
---

# Thinking 级别（/think 指令）

## 功能说明

- 在任何入站消息正文中使用内联指令：`/t <level>`、`/think:<level>` 或 `/thinking <level>`。
- 级别（别名）：`off | minimal | low | medium | high | xhigh`（仅限 GPT-5.2 + Codex 模型）
  - minimal → "think"
  - low → "think hard"
  - medium → "think harder"
  - high → "ultrathink"（最大预算）
  - xhigh → "ultrathink+"（仅限 GPT-5.2 + Codex 模型）
  - `x-high`、`x_high`、`extra-high`、`extra high` 和 `extra_high` 映射到 `xhigh`。
  - `highest`、`max` 映射到 `high`。
- Provider 说明：
  - Z.AI（`zai/*`）仅支持二进制 thinking（`on`/`off`）。任何非 `off` 级别都视为 `on`（映射到 `low`）。

## 解析顺序

1. 消息上的内联指令（仅对该消息生效）。
2. 会话覆盖（通过发送仅包含指令的消息设置）。
3. 全局默认值（配置中的 `agents.defaults.thinkingDefault`）。
4. 回退：具有推理能力的模型为 low；否则为 off。

## 设置会话默认值

- 发送一条**仅包含**指令的消息（允许空白），例如 `/think:medium` 或 `/t high`。
- 该设置持续到当前会话（默认按发送者隔离）；通过 `/think:off` 或会话空闲重置清除。
- 发送确认回复（`Thinking level set to high.` / `Thinking disabled.`）。如果级别无效（如 `/thinking big`），命令会被拒绝并附带提示，会话状态保持不变。
- 发送 `/think`（或 `/think:`）且不带参数可查看当前 thinking 级别。

## Agent 应用

- **内嵌 Pi**：解析后的级别传递给进程内 Pi Agent 运行时。

## Verbose 指令（/verbose 或 /v）

- 级别：`on`（最小）| `full` | `off`（默认）。
- 仅包含指令的消息切换会话 verbose 并回复 `Verbose logging enabled.` / `Verbose logging disabled.`；无效级别返回提示，不改变状态。
- `/verbose off` 存储显式会话覆盖；通过 Sessions UI 选择 `inherit` 清除。
- 内联指令仅影响该消息；否则应用会话/全局默认值。
- 发送 `/verbose`（或 `/verbose:`）且不带参数可查看当前 verbose 级别。
- 当 verbose 开启时，发出结构化工具结果的 Agent（Pi 及其他 JSON Agent）会将每个工具调用作为独立的元数据消息发送，前缀为 `<emoji> <tool-name>: <arg>`（如可用的路径/命令）。这些工具摘要在每个工具开始时发送（独立气泡），而非流式增量。
- 当 verbose 为 `full` 时，工具输出也会在完成后转发（独立气泡，截断到安全长度）。如果在运行中切换 `/verbose on|full|off`，后续工具气泡遵循新设置。

## 推理可见性（/reasoning）

- 级别：`on|off|stream`。
- 仅包含指令的消息切换是否在回复中显示 thinking 块。
- 启用时，推理作为**独立消息**发送，前缀为 `Reasoning:`。
- `stream`（仅 Telegram）：在回复生成时将推理流式传输到 Telegram 草稿气泡，然后发送不含推理的最终回答。
- 别名：`/reason`。
- 发送 `/reasoning`（或 `/reasoning:`）且不带参数可查看当前推理级别。

## 相关

- 提权模式文档见 [提权模式](/tools/elevated)。

## 心跳

- 心跳探测正文为配置的心跳提示（默认：`Read HEARTBEAT.md if it exists (workspace context). Follow it strictly. Do not infer or repeat old tasks from prior chats. If nothing needs attention, reply HEARTBEAT_OK.`）。心跳消息中的内联指令照常应用（但避免在心跳中更改会话默认值）。
- 心跳投递默认仅包含最终负载。要同时发送独立的 `Reasoning:` 消息（如可用），设置 `agents.defaults.heartbeat.includeReasoning: true` 或每 Agent 设置 `agents.list[].heartbeat.includeReasoning: true`。

## Web 聊天 UI

- Web 聊天的 Thinking 选择器在页面加载时镜像入站会话存储/配置中存储的会话级别。
- 选择其他级别仅对下一条消息生效（`thinkingOnce`）；发送后，选择器回弹到存储的会话级别。
- 要更改会话默认值，发送 `/think:<level>` 指令（同上）；选择器将在下次刷新后反映更改。
