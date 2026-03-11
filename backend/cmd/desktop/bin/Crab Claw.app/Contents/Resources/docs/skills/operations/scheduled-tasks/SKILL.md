---
name: scheduled-tasks
description: "定时任务自动化工作流：cron + agent-send + delivery 的端到端编排"
---

# 定时任务自动化技能

## 端到端流程

```
设计 → cron.add → 验证 → 投递配置 → 监控 → 维护
```

### 1. 设计阶段

- 明确任务目标、执行频率、输出类型
- 确认投递目标（频道 + 接收人）
- 低频优先，避免突发调度

### 2. 创建阶段

```
cron.add(
  name: "daily-report",
  schedule: "0 9 * * *",
  action: "agent",
  message: "生成日报并发送到频道",
  channel: "feishu:group_xxx",
  to: "@user",
  agent: "default"
)
```

### 3. 验证阶段

1. `cron.status` — 确认调度器健康
2. `cron.run` — 手动触发一次，验证输出 + 投递
3. 检查结果是否到达正确的频道和接收人

### 4. 投递设计

| 输出类型 | 投递方式 |
|---------|---------|
| 纯文本回复 | cron 内置 `deliver` |
| 文件/图片 | 任务内调用 `send_media` |
| 邮件 | 任务内调用 `send_email` |

**关键**: 始终显式设置 `channel` + `to`，不要依赖 `last`

### 5. 监控阶段

- `cron.runs` — 审计执行历史、错误、投递结果
- `cron.status` — 调度器整体健康

### 6. 维护

- `cron.update` — 调整频率/参数
- `cron.remove` — 退役不再需要的任务

## 与 hooks-agent 的区别

| 维度 | cron | hooks-agent |
|------|------|-------------|
| 触发方式 | 时间驱动 | 事件驱动（HTTP POST） |
| 适用场景 | 定期巡检、日报、数据同步 | 外部系统回调、Webhook 通知 |
| 投递 | 需显式配置 | 可在 mapping 中配置 |

## 常见陷阱

- `status` 健康 ≠ 业务逻辑正确 → 必须检查 `cron.runs` 实际产出
- 手动触发通过 ≠ 无人值守通过 → 常因缺少投递目标
- `wake` 唤醒现有队列，不替代调度设计
