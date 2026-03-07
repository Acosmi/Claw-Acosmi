---
name: slash-commands
description: "斜杠命令：文本 vs 原生、配置和支持的命令"
---

# 斜杠命令

命令由 Gateway 处理。大多数命令必须作为以 `/` 开头的**独立**消息发送。仅宿主机的 bash 聊天命令使用 `! <cmd>`（`/bash <cmd>` 为别名）。

有两个相关系统：

- **命令**：独立的 `/...` 消息。
- **指令**：`/think`、`/verbose`、`/reasoning`、`/elevated`、`/exec`、`/model`、`/queue`。
  - 指令在模型看到之前从消息中剥离。
  - 在普通聊天消息中（非仅指令），它们会作为内联提示继续影响本条消息；对**已授权发送者**，当前实现也会写入会话状态。
  - 在仅指令消息中（消息仅包含指令），它们会立即持久化到会话并回复确认。
  - 指令仅对**已授权发送者**生效（渠道白名单/配对加 `commands.useAccessGroups`）。未授权发送者的指令被视为纯文本。

还有一些**内联快捷键**（仅白名单/已授权发送者）：`/help`、`/commands`、`/status`、`/whoami`（`/id`）。它们立即运行，在模型看到前从消息中剥离，剩余文本继续正常流程。

## 配置

```json5
{
  commands: {
    native: "auto",
    nativeSkills: "auto",
    text: true,
    bash: false,
    bashForegroundMs: 2000,
    config: false,
    debug: false,
    restart: false,
    useAccessGroups: true,
  },
}
```

- `commands.text`（默认 `true`）启用在聊天消息中解析 `/...`。
  - 在没有原生命令的场景（WhatsApp/WebChat/Signal/iMessage/Google Chat/MS Teams）中，即使设为 `false` 文本命令仍然有效。
- `commands.native`（默认 `"auto"`）注册原生命令。
  - Auto：Discord/Telegram 开启；Slack 关闭（直到添加斜杠命令）；不支持原生命令的 Provider 忽略。
  - 设置 `channels.discord.commands.native`、`channels.telegram.commands.native` 或 `channels.slack.commands.native` 按 Provider 覆盖（bool 或 `"auto"`）。
  - `false` 在启动时清除 Discord/Telegram 上先前注册的命令。Slack 命令在 Slack 应用中管理，不会自动移除。
- `commands.nativeSkills`（默认 `"auto"`）在支持时原生注册**技能**命令。
  - Auto：Discord/Telegram 开启；Slack 关闭（Slack 需要为每个技能创建一个斜杠命令）。
  - 设置 `channels.discord.commands.nativeSkills`、`channels.telegram.commands.nativeSkills` 或 `channels.slack.commands.nativeSkills` 按 Provider 覆盖。
- `commands.bash`（默认 `false`）启用 `! <cmd>` 运行宿主机 Shell 命令（`/bash <cmd>` 为别名；需要 `tools.elevated` 白名单）。
- `commands.bashForegroundMs`（默认 `2000`）控制 bash 转后台前的等待时间（`0` 立即转后台）。
- `commands.config`（默认 `false`）启用 `/config`（读写 `openacosmi.json`）。
- `commands.debug`（默认 `false`）启用 `/debug`（仅运行时覆盖）。
- `commands.useAccessGroups`（默认 `true`）对命令执行白名单/策略。

## 命令列表

文本 + 原生（启用时）：

- `/help`
- `/commands`
- `/skill <name> [input]`（按名称运行技能）
- `/status`（显示当前状态；如可用则包含当前模型 Provider 的用量/配额）
- `/allowlist`（列出/添加/移除白名单条目）
- `/approve <id> allow-once|allow-always|deny`（处理 exec 审批提示）
- `/context [list|detail|json]`（解释"上下文"；`detail` 显示每文件 + 每工具 + 每技能 + 系统提示大小）
- `/whoami`（显示您的发送者 ID；别名：`/id`）
- `/subagents list|stop|log|info|send`（检查、停止、查看日志或向当前会话的子 Agent 运行发送消息）
- `/config show|get|set|unset`（持久化配置到磁盘，仅所有者；需要 `commands.config: true`）
- `/debug show|set|unset|reset`（运行时覆盖，仅所有者；需要 `commands.debug: true`）
- `/usage off|tokens|full|cost`（每响应用量页脚或本地成本摘要）
- `/tts off|always|inbound|tagged|status|provider|limit|summary|audio`（控制 TTS；参见 [/tts](/tts)）
  - Discord：原生命令为 `/voice`（Discord 保留了 `/tts`）；文本 `/tts` 仍然有效。
- `/stop`
- `/restart`
- `/dock-telegram`（别名：`/dock_telegram`）（将回复切换到 Telegram）
- `/dock-discord`（别名：`/dock_discord`）（将回复切换到 Discord）
- `/dock-slack`（别名：`/dock_slack`）（将回复切换到 Slack）
- `/activation mention|always`（仅群组）
- `/send on|off|inherit`（仅所有者）
- `/reset` 或 `/new [model]`（可选模型提示；剩余内容透传）
- `/think <off|minimal|low|medium|high|xhigh>`（根据模型/Provider 动态选择；别名：`/thinking`、`/t`）
- `/verbose on|full|off`（别名：`/v`）
- `/reasoning on|off|stream`（别名：`/reason`；开启时发送前缀为 `Reasoning:` 的独立消息；`stream` = 仅 Telegram 草稿）
- `/elevated on|off|ask|full`（别名：`/elev`；`full` 跳过 exec 审批）
- `/exec host=<sandbox|gateway|node> security=<deny|allowlist|full> ask=<off|on-miss|always> node=<id>`（发送 `/exec` 查看当前值）
- `/model <name>`（别名：`/models`；或 `agents.defaults.models.*.alias` 中的 `/<alias>`）
- `/queue <mode>`（加选项如 `debounce:2s cap:25 drop:summarize`；发送 `/queue` 查看当前设置）
- `/bash <command>`（仅宿主机；`! <command>` 的别名；需要 `commands.bash: true` + `tools.elevated` 白名单）

仅文本：

- `/compact [instructions]`（参见 [/concepts/compaction](/concepts/compaction)）
- `! <command>`（仅宿主机；一次一个；使用 `!poll` + `!stop` 处理长时间运行的任务）
- `!poll`（检查输出/状态；接受可选 `sessionId`；`/bash poll` 也可以）
- `!stop`（停止运行中的 bash 任务；接受可选 `sessionId`；`/bash stop` 也可以）

说明：

- 命令接受命令和参数之间可选的 `:`（如 `/think: high`、`/send: on`、`/help:`）。
- `/new <model>` 接受模型别名、`provider/model` 或 Provider 名称（模糊匹配）；无匹配时文本被视为消息正文。
- 完整 Provider 用量分析，使用 `openacosmi status --usage`。
- `/allowlist add|remove` 需要 `commands.config=true` 并遵守渠道 `configWrites`。
- `/usage` 控制每响应用量页脚；`/usage cost` 打印本地成本摘要。
- `/restart` 默认禁用；设置 `commands.restart: true` 启用。
- `/verbose` 用于调试和额外可见性；正常使用时保持**关闭**。
- `/reasoning`（和 `/verbose`）在群组环境中有风险：可能暴露您不想公开的内部推理或工具输出。建议保持关闭，尤其在群聊中。
- **快速路径：** 白名单发送者的仅命令消息立即处理（绕过队列 + 模型）。
- **群组提及控制：** 白名单发送者的仅命令消息绕过提及要求。
- **内联快捷键（仅白名单发送者）：** 某些命令也在嵌入普通消息时有效，在模型看到剩余文本之前被剥离。
  - 示例：`hey /status` 触发状态回复，剩余文本继续正常流程。
- 当前支持：`/help`、`/commands`、`/status`、`/whoami`（`/id`）。
- 未授权的仅命令消息被静默忽略，内联 `/...` Token 被视为纯文本。
- **技能命令：** `user-invocable` 技能作为斜杠命令暴露。名称清理为 `a-z0-9_`（最多 32 字符）；冲突时加数字后缀（如 `_2`）。
  - `/skill <name> [input]` 按名称运行技能（当原生命令限制阻止每技能命令时有用）。
  - 默认情况下，技能命令作为普通请求转发给模型。
  - 技能可选声明 `command-dispatch: tool` 将命令直接路由到工具（确定性，无模型）。
  - 示例：`/prose`（OpenProse 插件）— 参见 [OpenProse](/prose)。
- **原生命令参数：** Discord 使用自动补全处理动态选项（省略必填参数时使用按钮菜单）。Telegram 和 Slack 在命令支持选项且您省略参数时显示按钮菜单。

## 用量展示（何处显示什么）

- **Provider 用量/配额**（示例："Claude 80% left"）在启用用量跟踪时显示在 `/status` 中。
- **每响应 Token/成本** 由 `/usage off|tokens|full` 控制（附加到普通回复）。
- `/model status` 关于**模型/认证/端点**，不是用量。

## 模型选择（`/model`）

`/model` 实现为指令。

示例：

```
/model
/model list
/model 3
/model openai/gpt-5.2
/model opus@anthropic:default
/model status
```

说明：

- `/model` 和 `/model list` 显示紧凑的编号选择器（模型系列 + 可用 Provider）。
- `/model <#>` 从选择器中选择（尽量使用当前 Provider）。
- `/model status` 显示详细视图，包括已配置的 Provider 端点（`baseUrl`）和 API 模式（`api`）。

## 调试覆盖

`/debug` 可设置**仅运行时**配置覆盖（内存，非磁盘）。仅所有者。默认禁用；设置 `commands.debug: true` 启用。

示例：

```
/debug show
/debug set messages.responsePrefix="[openacosmi]"
/debug set channels.whatsapp.allowFrom=["+1555","+4477"]
/debug unset messages.responsePrefix
/debug reset
```

说明：

- 覆盖立即应用于新的配置读取，但**不会**写入 `openacosmi.json`。
- 使用 `/debug reset` 清除所有覆盖并恢复到磁盘上的配置。

## 配置更新

`/config` 写入磁盘上的配置（`openacosmi.json`）。仅所有者。默认禁用；设置 `commands.config: true` 启用。

示例：

```
/config show
/config show messages.responsePrefix
/config get messages.responsePrefix
/config set messages.responsePrefix="[openacosmi]"
/config unset messages.responsePrefix
```

说明：

- 写入前验证配置；无效更改被拒绝。
- `/config` 更新在重启后持续生效。

## 场景说明

- **文本命令** 在普通聊天会话中运行（私聊共享 `main`，群组有自己的会话）。
- **原生命令** 使用隔离会话：
  - Discord：`agent:<agentId>:discord:slash:<userId>`
  - Slack：`agent:<agentId>:slack:slash:<userId>`（前缀可通过 `channels.slack.slashCommand.sessionPrefix` 配置）
  - Telegram：`telegram:slash:<userId>`（通过 `CommandTargetSessionKey` 指向聊天会话）
- **`/stop`** 指向活跃聊天会话以中止当前运行。
- **Slack：** `channels.slack.slashCommand` 仍支持单个 `/openacosmi` 风格的命令。如果启用 `commands.native`，您必须为每个内置命令创建一个 Slack 斜杠命令（与 `/help` 同名）。Slack 的命令参数菜单以临时 Block Kit 按钮形式投递。

## 故障树

- 命令“没反应”：最常见不是命令不存在，而是发送者未授权、群组提及条件不满足，或该渠道对未授权控制命令选择静默忽略。
- 误把内联指令当成一次性提示：当前实现里，已授权发送者的 `/think`、`/verbose`、`/reasoning`、`/elevated`、`/exec`、`/model`、`/queue` 会写入会话状态，不只影响当前一句。
- 把 `/exec` 当全局配置：它只改当前会话默认值；查看当前值可直接发 `/exec`，全局持久化应走 `/config`。
- 期望各渠道原生命令行为完全一致：文本命令、原生命令和隔离 slash session 在不同渠道并不等价，尤其 Slack/Telegram/Discord 各自有差异。

## 回滚步骤

1. 先用 `/status`、`/whoami` 或 `/commands` 验证当前发送者是否走到了正确命令路径，再发有副作用的指令。
2. 需要改会话默认值时，优先发“仅指令消息”拿到明确 ack；不要在长业务消息里顺手夹带 `/exec` 或 `/model` 再假设它不会持久化。
3. `/exec` 先查询当前默认值，再一次只改一个维度（`host`、`security`、`ask`、`node`），避免把 host 和审批问题混成同一故障。
4. 发现会话设置被污染时，不要继续叠更多指令；改用 `/new` 或显式会话修复，再恢复需要的指令默认值。

## 验收清单

- 当前渠道的文本命令/原生命令路径清楚，授权发送者与群组提及要求已确认。
- 会话级指令变更拿到了可解释的反馈，且后续消息表现与当前会话默认值一致。
- `/exec`、`/model`、`/queue` 的修改被理解为会话状态，不再误写成全局配置变更。
- 未授权发送者的命令被正确视为纯文本或静默忽略，不再把这类现象误判成“命令系统坏了”。
