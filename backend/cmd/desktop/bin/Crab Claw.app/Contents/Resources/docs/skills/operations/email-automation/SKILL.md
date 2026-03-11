---
name: email-automation
description: "邮件自动化工作流：收件处理、自动回复、定时发送与线程管理"
---

# 邮件自动化技能

## 邮件能力范围

| 能力 | 当前支持 | 工具 |
|------|---------|------|
| 发送新邮件 | ✅ | `send_email` |
| 回复邮件线程 | ✅ | `send_email` (reply_to_session) |
| 多账号发送 | ✅ | `send_email` (account) |
| 收件通知 | ✅ | 邮件频道入站 |
| 定时发送 | ✅ | `cron` + `send_email` |
| 附件发送 | ❌ V1 不支持 | — |
| HTML 富文本 | ❌ 仅纯文本 | — |

## 自动邮件工作流

### 1. 定时邮件报告

```
cron.add(
  name: "weekly-digest",
  schedule: "0 9 * * 1",
  action: "agent",
  message: "生成本周摘要并发送邮件给 team@company.com"
)
```

Agent 执行时调用 `send_email(to: "team@company.com", subject: "Weekly Digest", body: "...")`

### 2. 收件自动回复

邮件频道收到入站消息 → Agent 分析内容 → 按规则调用 `send_email` 回复

### 3. 线程回复

使用 `reply_to_session` 参数自动恢复邮件 thread headers，确保回复出现在同一线程中。

## 安全约束

- 每账号有**频率限制**（小时/天上限）
- 自动添加 `Auto-Submitted` 头防止邮件循环
- NoNetwork 委托下不可用
- 发送前需 **plan_confirm** 审批

## 最佳实践

1. 始终确认收件人地址正确后再发送
2. 定时邮件先手动触发一次验证投递
3. 线程回复始终使用 `reply_to_session` 而非手动拼 headers
4. 批量发送注意频率限制，避免被标记为垃圾邮件
