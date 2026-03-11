---
name: hooks-automation
description: "Webhook 自动化：/hooks/agent 外部事件触发、hooks.mappings 映射与投递配置"
---

# Webhook 自动化技能

## 适用场景

外部系统（GitHub、GitLab、Gmail、CI 等）通过 HTTP 触发一次 Agent 运行。

### 选择原则

| 场景 | 工具 |
|------|------|
| 人工立刻跑一次 | `sessions_send` |
| 固定周期执行 | `cron` |
| 外部事件触发 | `/hooks/agent` |

## POST /hooks/agent

### 必填参数

- `message`: 任务描述

### 常用可选参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `name` | `Hook` | 会话名称 |
| `sessionKey` | `hook:<uuid>` | 会话标识 |
| `wakeMode` | `now` | `now` 或 `next-heartbeat` |
| `deliver` | `true` | 是否投递最终回复 |
| `channel` | `last` | 投递目标频道 |
| `to` | — | 显式投递目标 |
| `model` | — | 指定模型 |
| `timeoutSeconds` | — | 超时时间 |

### 认证

- `Authorization: Bearer <token>`
- `x-openacosmi-token: <token>`

成功返回 `202 Accepted` + `runId`。

## hooks.mappings

通过 `POST /hooks/<name>` 配合映射配置：

- `action: "agent"` — 启动完整 Agent 运行
- `action: "wake"` — 仅唤醒/心跳（不等于完整任务）
- `match.path` / `match.source` — 匹配 webhook 来源
- `messageTemplate` — 按 payload 模板渲染

**注意**: "外部事件来了就跑一次任务" → `action: "agent"`，不要误配成 `wake`。

## 投递与频道

- `deliver=true` 表示最终回复投递到频道
- 常用 `channel` 值: `last`, `new`, `background`, `whatsapp`, `telegram`, `discord`, `slack` 等
- 无历史路由时必须显式提供 `channel` + `to`
- 文件发送仍需在运行过程中调用 `send_media`

## 边界

- hook 入口负责触发运行，不保证中途进度外发
- `deliver` 解决最终回复投递，不等于阶段性进度广播
- 中途进度需求参考 `progress-reporting` 技能
