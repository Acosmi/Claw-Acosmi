---
name: date-time
description: "日期和时间处理：信封、提示词、工具和连接器"
---

# 日期与时间

创宇太虚默认将**主机本地时间用于传输时间戳**，**用户时区仅在 system prompt 中使用**。
供应商时间戳会被保留，因此工具保持其原生语义（当前时间可通过 `session_status` 获取）。

## 消息信封（默认本地时间）

入站消息会被包装一个时间戳（分钟精度）：

```
[Provider ... 2026-01-05 16:26 PST] 消息文本
```

此信封时间戳**默认使用主机本地时间**，与供应商时区无关。

可通过配置覆盖此行为：

```json5
{
  agents: {
    defaults: {
      envelopeTimezone: "local", // "utc" | "local" | "user" | IANA 时区
      envelopeTimestamp: "on", // "on" | "off"
      envelopeElapsed: "on", // "on" | "off"
    },
  },
}
```

- `envelopeTimezone: "utc"` 使用 UTC。
- `envelopeTimezone: "local"` 使用主机时区。
- `envelopeTimezone: "user"` 使用 `agents.defaults.userTimezone`（回退到主机时区）。
- 使用显式 IANA 时区（如 `"America/Chicago"`）指定固定时区。
- `envelopeTimestamp: "off"` 移除信封头中的绝对时间戳。
- `envelopeElapsed: "off"` 移除已用时间后缀（`+2m` 样式）。

### 示例

**本地（默认）：**

```
[WhatsApp +1555 2026-01-18 00:19 PST] hello
```

**用户时区：**

```
[WhatsApp +1555 2026-01-18 00:19 CST] hello
```

**启用已用时间：**

```
[WhatsApp +1555 +30s 2026-01-18T05:19Z] 后续消息
```

## System Prompt：当前日期与时间

若已知用户时区，system prompt 会包含一个专门的**当前日期与时间**段，仅含**时区**（无时钟/时间格式），以保持 prompt 缓存稳定：

```
Time zone: America/Chicago
```

当智能体需要获取当前时间时，使用 `session_status` 工具；状态卡包含时间戳行。

## 系统事件行（默认本地时间）

插入到智能体上下文中的排队系统事件会加上时间戳前缀，使用与消息信封相同的时区选择（默认：主机本地）。

```
System: [2026-01-12 12:19:17 PST] 模型已切换。
```

### 配置用户时区 + 格式

```json5
{
  agents: {
    defaults: {
      userTimezone: "America/Chicago",
      timeFormat: "auto", // auto | 12 | 24
    },
  },
}
```

- `userTimezone` 设置用于 prompt 上下文的**用户本地时区**。
- `timeFormat` 控制 prompt 中的 **12 小时/24 小时显示**。`auto` 跟随系统偏好。

## 时间格式检测（auto）

当 `timeFormat: "auto"` 时，创宇太虚会检查操作系统偏好（macOS/Windows）并回退到 locale 格式化。检测值在**每个进程中缓存**，避免重复系统调用。

## 工具载荷 + 连接器（原始供应商时间 + 标准化字段）

频道工具返回**供应商原生时间戳**，并添加标准化字段以保持一致性：

- `timestampMs`：纪元毫秒（UTC）
- `timestampUtc`：ISO 8601 UTC 字符串

原始供应商字段会被保留，不会丢失。

- Slack：API 返回的纪元样式字符串
- Discord：UTC ISO 时间戳
- Telegram/WhatsApp：供应商特定的数字/ISO 时间戳

若需要本地时间，使用已知时区在下游进行转换。

## 相关文档

- [系统提示词](/concepts/system-prompt)
- [时区](/concepts/timezone)
- [消息](/concepts/messages)
