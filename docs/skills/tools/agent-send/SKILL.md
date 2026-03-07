---
name: agent-send
description: "直接运行 `openacosmi agent` CLI（支持可选投递）"
---

# `openacosmi agent`

用于在**没有入站聊天消息**时主动运行一次 Agent。  
默认通过 Gateway 执行；Gateway 不可达时会回退到本地内嵌运行。

## 何时使用

- 需要立刻主动跑一次任务
- 需要“跑完后把最终结果发回渠道”
- 需要从脚本、命令行、人工补偿流程触发一次 Agent
- 不需要等定时器，也不需要等 webhook 事件

选择原则：

- 一次性立即执行：`agent-send`
- 定时执行：`cron-ops`
- 外部系统触发：`hooks-agent`

## 会话选择

- `--to <dest>`：按目标推导会话键
- `--session-id <id>`：复用已有会话
- `--agent <id>`：直接指定已配置 Agent
- 必填消息：`--message <text>`

## 投递原则

- `--deliver` 用于把**最终回复**投递回渠道
- 若已有稳定会话路由，可直接沿现有路由投递
- 若没有既有路由，显式提供 `--reply-channel` 与 `--reply-to`
- 中途状态若要对实时界面可见，可另行使用 `report_progress`；不要假设远程频道会收到中间进度
- 需要把 PDF、截图、代码文件发到渠道时，不要只依赖最终文本回复；改用 `send_media`

## 示例

```bash
openacosmi agent --to +15555550123 --message "status update"
openacosmi agent --agent ops --message "Summarize logs"
openacosmi agent --session-id 1234 --message "Summarize inbox" --thinking medium
openacosmi agent --to +15555550123 --message "Summon reply" --deliver
openacosmi agent --agent ops --message "Generate report" --deliver --reply-channel slack --reply-to "#reports"
```

## 常用标志

- `--local`：本地运行（需要在 Shell 中配置模型 Provider 的 API 密钥）
- `--deliver`：将回复发送到所选渠道
- `--channel`：投递渠道
- `--reply-to`：投递目标覆盖
- `--reply-channel`：投递渠道覆盖
- `--reply-account`：投递账户 ID 覆盖
- `--thinking <off|minimal|low|medium|high|xhigh>`：持久化 thinking 级别（仅限 GPT-5.2 + Codex 模型）
- `--verbose <on|full|off>`：持久化 verbose 级别
- `--timeout <seconds>`：覆盖 Agent 超时时间
- `--json`：输出结构化 JSON

## 故障树

- Gateway 路径失败：CLI 会尝试回退到本地内嵌运行，这时问题要区分成“网关不可达”还是“本地模型/环境未就绪”
- 只开了 `--deliver` 但没送到渠道：通常是当前会话没有可继承路由，或缺少 `--reply-channel` / `--reply-to`
- 最终回复到了，但文件没出去：说明你把文件交付误当成纯文本 deliver，应该拆给 `send_media`
- 一次性命令越跑越像定时任务：说明技能选型错了，应回到 `cron-ops` 或 `hooks-agent`
- 会话跑偏：通常是 `--to`、`--session-id`、`--agent` 选错，导致在错误会话里主动执行

## 回滚步骤

- 若不确定投递路由，先去掉 `--deliver` 跑一次纯本地/纯会话执行，确认结果内容正确
- 需要外发但没有既有路由时，补齐 `--reply-channel` 和 `--reply-to` 后再重跑
- 发现任务本质上是定时或外部触发时，停止继续堆 `agent-send`，改回 `cron-ops` 或 `hooks-agent`
- 发现最终产物是文件时，保留本次执行结果，交付阶段切到 `send_media`
- 若 Gateway 不可达且本地也缺少模型环境，先修运行环境，不要反复重试同一命令

## 验收清单

- Agent 确实在预期会话或预期 Agent 上完成了一次独立运行
- 若要求外发，最终回复已送达正确渠道和正确目标
- 若需要文件交付，文件已通过 `send_media` 单独完成
- 本次执行场景确实属于“一次性主动运行”，而不是定时或 webhook 场景
- 结果链路清晰：运行、投递、文件交付分别由正确能力承担
