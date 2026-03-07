---
name: chat-sessions
description: "会话与聊天运维：chat.* 与 sessions.* 的排障和管理流程"
---

# Chat 与 Sessions 运维

用于定位消息链路问题、会话污染问题、历史压缩问题。

## 覆盖方法
- `chat.send`
- `chat.history`
- `chat.abort`
- `chat.inject`
- `sessions.list`
- `sessions.preview`
- `sessions.resolve`
- `sessions.patch`
- `sessions.reset`
- `sessions.delete`
- `sessions.compact`

## 标准操作流程
1. 用 `sessions.list` 与 `sessions.preview` 确认目标会话键。
2. 用 `chat.history` 定位异常消息片段与时间窗。
3. 需要中断长任务时调用 `chat.abort`。
4. 需要修正会话元信息时调用 `sessions.patch`。
5. 污染严重时用 `sessions.reset`，历史过长时用 `sessions.compact`。

## 风险控制
- 不要对生产会话直接 `sessions.delete`，先做 `preview`。
- `chat.inject` 仅用于回放/修复，不用于正常业务消息发送。

## 成功判定
- `chat.send` 可正常回包。
- `chat.history` 顺序和角色字段正常。

## 故障树

- 会话改错对象：未先用 `sessions.list`、`sessions.preview`、`sessions.resolve` 确认 key，就直接 `patch`、`reset`、`compact` 或 `delete`。
- 把 `chat.abort` 当成“撤销一切”：它的真实语义是标记中断并广播 abort 事件，不会回滚已经发生的外部副作用。
- 把 `sessions.reset` 当成彻底清空：当前实现会归档旧 transcript、生成新 `sessionId`，但会保留一批白名单字段。
- 把 `sessions.compact` 当成智能摘要：当前实现只是截断 transcript 行并重置 token 计数，不做语义总结。
- 把 `sessions.delete` 当成无痕删除：它受 admin 权限和主 session 保护约束，默认还会归档 transcript。

## 回滚步骤

1. 先 `sessions.list` + `sessions.preview`，必要时再 `sessions.resolve`，确认目标 key 后再做修改。
2. 先轻后重：优先 `sessions.patch`，再考虑 `sessions.compact` 或 `sessions.reset`，最后才是 `sessions.delete`。
3. 历史链路异常时先看 `chat.history`；`chat.inject` 只用于修复/回放，不替代正常 `chat.send`。
4. 中断长任务时把 `chat.abort` 当成“停止后续处理”，不要把它宣传成“已撤销所有已执行动作”。

## 验收清单

- 目标会话 key 已通过 list/preview/resolve 确认，没有误操作主会话或错误分支会话。
- `chat.history`、`chat.send`、`chat.abort` 的结果与当前 transcript、运行状态相互一致。
- `sessions.reset`、`sessions.compact`、`sessions.delete` 的副作用被理解清楚：归档、截断、保留字段和主 session 保护都符合预期。
- 修复链路中没有把 `chat.inject` 当成常规发消息接口，也没有在未预览的情况下直接删除生产会话。
