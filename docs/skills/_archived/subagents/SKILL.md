---
name: subagents
description: "子 Agent：生成隔离的 Agent 运行并将结果通告回请求者聊天渠道"
---

# 子 Agent

子 Agent 是从现有 Agent 运行中生成的后台 Agent 运行。它们在独立会话中运行（`agent:<agentId>:subagent:<uuid>`），完成后会将结果**通告**回请求者的聊天渠道。

## 斜杠命令

使用 `/subagents` 检查或控制**当前会话**的子 Agent 运行：

- `/subagents list`
- `/subagents stop <id|#|all>`
- `/subagents log <id|#> [limit] [tools]`
- `/subagents info <id|#>`
- `/subagents send <id|#> <message>`

`/subagents info` 显示运行元数据（状态、时间戳、会话 ID、transcript 路径、清理策略）。

核心目标：

- 并行化"研究/长任务/慢工具"工作，不阻塞主运行。
- 默认保持子 Agent 隔离（会话分离 + 可选沙箱）。
- 保持工具暴露面安全：子 Agent 默认**不获取**会话工具。
- 避免嵌套扇出：子 Agent 不能生成子 Agent。

成本说明：每个子 Agent 有其**独立的**上下文和 Token 用量。对于繁重或重复的任务，为子 Agent 设置更便宜的模型，主 Agent 保持使用更高质量的模型。可通过 `agents.defaults.subagents.model` 或每 Agent 覆盖配置。

## 专用委托工具

对于特定类型的委托任务，优先使用专用工具而非 `sessions_spawn`：

- `spawn_coder_agent`（Open Coder）：编码任务（多文件编辑、重构、测试）
- `spawn_argus_agent`（灵瞳 Argus）：桌面/GUI 操作（原生应用、OCR、视觉工作流）
- `spawn_media_agent`：媒体运营（内容创作、发布、趋势分析）

这些专用工具使用委托合约系统，提供能力控制、协商循环和质量审核。

## 通用子 Agent 工具

使用 `sessions_spawn`：

- 启动一个子 Agent 运行（`deliver: false`，全局通道：`subagent`）
- 然后运行通告步骤并将通告回复发布到请求者聊天渠道
- 默认模型：继承调用者，除非设置 `agents.defaults.subagents.model`（或每 Agent `agents.list[].subagents.model`）；显式的 `sessions_spawn.model` 仍然优先。
- 默认 thinking：继承调用者，除非设置 `agents.defaults.subagents.thinking`（或每 Agent `agents.list[].subagents.thinking`）；显式的 `sessions_spawn.thinking` 仍然优先。

工具参数：

- `task`（必填）
- `label?`（可选）
- `agentId?`（可选；如果允许，在另一个 Agent ID 下生成）
- `model?`（可选；覆盖子 Agent 模型；无效值会被跳过，子 Agent 使用默认模型运行，工具结果中带警告）
- `thinking?`（可选；覆盖子 Agent 运行的 thinking 级别）
- `runTimeoutSeconds?`（默认 `0`；设置后，子 Agent 运行在 N 秒后中止）
- `cleanup?`（`delete|keep`，默认 `keep`）

白名单：

- `agents.list[].subagents.allowAgents`：允许通过 `agentId` 指定的 Agent ID 列表（`["*"]` 允许任意）。默认：仅请求者 Agent。

发现：

- 使用 `agents_list` 查看当前允许用于 `sessions_spawn` 的 Agent ID。

自动归档：

- 子 Agent 会话在 `agents.defaults.subagents.archiveAfterMinutes`（默认 60）后自动归档。
- 归档使用 `sessions.delete` 并将 transcript 重命名为 `*.deleted.<timestamp>`（同一文件夹）。
- `cleanup: "delete"` 在通告后立即归档（仍通过重命名保留 transcript）。
- 自动归档为尽力而为；Gateway 重启时挂起的计时器会丢失。
- `runTimeoutSeconds` **不会**自动归档；它仅停止运行。会话保留到自动归档时。

## 身份验证

子 Agent 认证按**Agent ID** 解析，而非按会话类型：

- 子 Agent 会话键为 `agent:<agentId>:subagent:<uuid>`。
- 认证存储从该 Agent 的 `agentDir` 加载。
- 主 Agent 的认证 profile 作为**回退**合并；Agent profile 在冲突时覆盖主 profile。

注意：合并是累加的，主 profile 始终作为回退可用。目前不支持每 Agent 完全隔离的认证。

## 通告

子 Agent 通过通告步骤报告：

- 通告步骤在子 Agent 会话内运行（不是请求者会话）。
- 如果子 Agent 回复正好是 `ANNOUNCE_SKIP`，则不发布任何内容。
- 否则通告回复通过后续 `agent` 调用（`deliver=true`）发布到请求者聊天渠道。
- 通告回复在可用时保留线程/话题路由（Slack threads、Telegram topics、Matrix threads）。
- 通告消息规范化为稳定模板：
  - `Status:` 从运行结果推导（`success`、`error`、`timeout` 或 `unknown`）。
  - `Result:` 来自通告步骤的摘要内容（缺失时为 `(not available)`）。
  - `Notes:` 错误详情和其他有用上下文。
- `Status` 不从模型输出推断；它来自运行时结果信号。

通告负载末尾包含统计行（即使被包装）：

- 运行时间（如 `runtime 5m12s`）
- Token 用量（输入/输出/总计）
- 配置了模型定价时的估计成本（`models.providers.*.models[].cost`）
- `sessionKey`、`sessionId` 和 transcript 路径（主 Agent 可通过 `sessions_history` 获取历史或检查磁盘上的文件）

## 工具策略（子 Agent 工具）

默认情况下，子 Agent 获得**除会话工具外的所有工具**：

- `sessions_list`
- `sessions_history`
- `sessions_send`
- `sessions_spawn`

通过配置覆盖：

```json5
{
  agents: {
    defaults: {
      subagents: {
        maxConcurrent: 1,
      },
    },
  },
  tools: {
    subagents: {
      tools: {
        // deny 优先
        deny: ["gateway", "cron"],
        // 如果设置了 allow，则变为仅允许模式（deny 仍然优先）
        // allow: ["read_file", "bash", "process"]
      },
    },
  },
}
```

## 并发

子 Agent 使用专用的进程内队列通道：

- 通道名：`subagent`
- 并发数：`agents.defaults.subagents.maxConcurrent`（默认 `8`）

## 停止

- 在请求者聊天中发送 `/stop` 会中止请求者会话并停止从中生成的所有活跃子 Agent 运行。

## 限制

- 子 Agent 通告为**尽力而为**。如果 Gateway 重启，挂起的"通告回复"工作会丢失。
- 子 Agent 仍然共享相同的 Gateway 进程资源；将 `maxConcurrent` 视为安全阀。
- `sessions_spawn` 始终非阻塞：它立即返回 `{ status: "accepted", runId, childSessionKey }`。
- 子 Agent 上下文仅注入 `AGENTS.md` + `TOOLS.md`（不包含 `SOUL.md`、`IDENTITY.md`、`USER.md`、`HEARTBEAT.md` 或 `BOOTSTRAP.md`）。

## 故障树

- 把 `sessions_spawn` 当成同步调用：它只会立即返回 `accepted`，真实结果要看子会话收敛与后续通告。
- 期望子 Agent 完整继承主会话能力：子 Agent 默认不拿会话工具，认证是“子 Agent profile + 主 profile 回退”，不是完全隔离副本。
- 结果没有回到主对话：通告本身是 best-effort，Gateway 重启、队列回退失败、`ANNOUNCE_SKIP` / `NO_REPLY` 都会导致不发公告。
- 想继续扇出更多子 Agent：当前不支持嵌套生成，子 Agent 不能再生成子 Agent。

## 回滚步骤

1. 先把任务缩成单一明确交付物，保留 `cleanup=keep`，用最小 `sessions_spawn` 验证子会话能起、能跑、能回收结果。
2. 若没有公告，先用 `/subagents info`、`/subagents log` 或 `sessions_history` 查子会话，再决定是否重跑，不要连续重复 spawn。
3. 若任务必须同步给出结果，停止继续堆子 Agent，改回主 Agent 直接执行或拆成显式步骤。
4. 若任务依赖强隔离认证或额外会话工具，先调整工具策略/Agent 配置；不要假设子 Agent 自带主会话全部上下文。

## 验收清单

- `sessions_spawn` 已返回有效 `runId` 与 `childSessionKey`，且子会话可被 `/subagents` 或会话工具查询到。
- 子 Agent 任务完成后，主对话收到预期公告，或能明确解释为 `ANNOUNCE_SKIP` / `NO_REPLY` / Gateway 重启导致的 best-effort 未送达。
- `cleanup=keep|delete` 的行为与预期一致，没有误以为删除就等于彻底抹除 transcript。
- 任务设计没有依赖嵌套 subagent，也没有错误假设子 Agent 拥有主会话全部工具和认证隔离。
