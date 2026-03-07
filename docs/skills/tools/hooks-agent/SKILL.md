---
name: hooks-agent
description: "Webhook 主动触发：/hooks/agent 与 hooks.mappings 的外部事件入口"
---

# Hook 主动触发

用于外部系统通过 HTTP 触发一次 Agent 运行。  
它适合“事件来了就跑一次”，不适合固定时钟调度，也不适合人工即时补跑。

## 何时使用

- GitHub、GitLab、Gmail、CI、表单系统等外部事件到来时
- 需要把 webhook 载荷转成一次 Agent 任务
- 需要由服务端主动执行并可选投递最终结果

## 选择原则

- 人工立刻跑一次：`agent-send`
- 固定周期执行：`cron-ops`
- 外部事件触发：`hooks-agent`

## 直接入口：`POST /hooks/agent`

最小输入：

- `message`：必填

常用可选字段：

- `name`：默认 `Hook`
- `sessionKey`：默认 `hook:<uuid>`
- `wakeMode`：仅支持 `now` 或 `next-heartbeat`，默认 `now`
- `deliver`：默认 `true`
- `channel`：默认 `last`
- `to`：显式投递目标
- `model`
- `thinking`
- `timeoutSeconds`

认证方式：

- `Authorization: Bearer <token>`
- `x-openacosmi-token: <token>`

成功时返回 `202 Accepted` 与 `runId`。

## 渠道与投递

- `deliver=true` 表示**最终回复**可以投递到渠道。
- `channel` 常用值是 `last` 或具体渠道名。
- 当前允许的 `channel` 值包括：
  - `last`
  - `new`
  - `background`
  - `whatsapp`
  - `telegram`
  - `discord`
  - `googlechat`
  - `slack`
  - `signal`
  - `imessage`
  - `msteams`
- 如果没有现成的历史路由，显式提供 `channel` + `to`，不要依赖模糊的默认路由。
- 如果结果里包含 PDF、截图、代码文件等，实际文件发送仍应在运行过程中调用 `send_media`。

## `hooks.mappings`

除了直接调用 `/hooks/agent`，也可以通过 `POST /hooks/<name>` 配合 `hooks.mappings` 做映射：

- `action: "agent"`：真正启动一次 Agent 运行
- `action: "wake"`：只做唤醒/心跳，不等于完整任务执行
- `match.path` / `match.source`：匹配不同 webhook 来源
- `messageTemplate` / `sessionKey` / `to` / `model`：可按 payload 模板渲染

如果需求是“外部事件来了就跑一次任务”，优先落在 `action: "agent"`，不要误配成 `wake`。

## 当前边界

- hook 入口负责触发运行，不保证中途状态自动外发到远程聊天渠道。
- `deliver` 解决的是最终回复投递，不等于阶段性进度广播。
- 如果用户明确要求中途可见进度，另行参考 `progress-reporting`，但不要假设远程频道一定能看到。

## 故障树

- 请求未触发运行：先看 token、`message`、JSON 结构是否有效，再看 hook 映射是否命中了正确动作
- 返回 `202` 但结果没送达：通常是 `deliver` 路由问题，不是触发本身失败
- 使用 `channel: "last"` 却没历史路由：这是默认路由缺失，应显式补 `channel` + `to`
- 把 `action: "wake"` 当成完整任务执行：会只触发唤醒，不会按预期跑完整 Agent 任务
- 期望 hook 直接完成文件发送或中途进度广播：这超出 hook 主职责，应分别交给 `send_media` 和显式进度方案

## 回滚步骤

- 先回退到最小 `POST /hooks/agent` 输入，只保留 `message` 验证触发链路
- 若映射复杂，先把自定义 `hooks.mappings` 暂时简化成固定 `action: "agent"` 排查
- 投递异常时先关掉 `deliver` 验证任务本身能否正常跑完，再补 `channel` / `to`
- 发现需求本质上是定时调度或人工补跑时，停止继续堆 hook，改回 `cron-ops` 或 `agent-send`
- 文件交付需求单独拆给运行过程中的 `send_media`，不要继续试图让 hook 配置代替它

## 验收清单

- 请求返回 `202` 且得到有效 `runId`
- 触发出的会话键、模型、wake 模式与预期一致
- 若启用 `deliver`，最终回复已送达正确渠道与正确目标
- 若使用映射，确认命中的是 `agent` 而不是误配成 `wake`
- 没有把 webhook 入口误当成文件发送或中途进度通道
