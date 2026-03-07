---
name: logging
description: "日志系统：文件日志、控制台输出、CLI 尾随、控制面板"
---

# 日志系统

创宇太虚在两个地方输出日志：

- **文件日志**（JSON 行格式），由网关写入。
- **控制台输出**，显示在终端和控制面板 UI 中。

本文说明日志存储位置、读取方式，以及如何配置日志级别和格式。

## 日志文件位置

默认情况下，网关将滚动日志文件写入：

`/tmp/openacosmi/openacosmi-YYYY-MM-DD.log`

日期使用网关主机的本地时区。

可在 `~/.openacosmi/openacosmi.json` 中覆盖：

```json
{
  "logging": {
    "file": "/path/to/openacosmi.log"
  }
}
```

## 读取日志

### CLI：实时尾随（推荐）

```bash
openacosmi logs follow
```

输出模式：

- **TTY 会话**：美化、彩色、结构化日志行。
- **非 TTY 会话**：纯文本。
- `--json`：行分隔 JSON（每行一个日志事件）。
- `--plain`：在 TTY 会话中强制纯文本。
- `--no-color`：禁用 ANSI 颜色。

在 JSON 模式下，CLI 输出带 `type` 标签的对象：

- `meta`：流元数据（文件、游标、大小）
- `log`：已解析的日志条目
- `notice`：截断/轮换提示
- `raw`：未解析的日志行

若网关不可达，CLI 会输出简短提示建议运行：

```bash
openacosmi doctor
```

### 控制面板 UI（Web）

控制面板 UI 的**日志**标签页使用 `logs.tail` 尾随同一文件。
详见 [控制面板 UI](/web/control-ui) 了解如何打开。

### 频道专用日志

要筛选频道活动（WhatsApp/Telegram 等），使用：

```bash
openacosmi channels logs --channel whatsapp
```

## 日志格式

### 文件日志（JSONL）

日志文件中每行是一个 JSON 对象。CLI 和控制面板 UI 解析这些条目以渲染结构化输出（时间、级别、子系统、消息）。

### 控制台输出

控制台日志**感知 TTY**，按可读性格式化：

- 子系统前缀（如 `gateway/channels/whatsapp`）
- 级别着色（info/warn/error）
- 可选紧凑或 JSON 模式

控制台格式由 `logging.consoleStyle` 控制。

## 配置日志

所有日志配置位于 `~/.openacosmi/openacosmi.json` 中的 `logging` 下。

```json
{
  "logging": {
    "level": "info",
    "file": "/tmp/openacosmi/openacosmi-YYYY-MM-DD.log",
    "consoleLevel": "info",
    "consoleStyle": "pretty",
    "redactSensitive": "tools",
    "redactPatterns": ["sk-.*"]
  }
}
```

### 日志级别

- `logging.level`：**文件日志**（JSONL）级别。
- `logging.consoleLevel`：**控制台**详细程度。

`--verbose` 仅影响控制台输出，不改变文件日志级别。

### 控制台样式

`logging.consoleStyle`：

- `pretty`：人类友好、彩色、带时间戳。
- `compact`：更紧凑的输出（适合长时间会话）。
- `json`：每行 JSON（供日志处理器使用）。

### 脱敏

工具摘要可在输出到控制台前脱敏敏感令牌：

- `logging.redactSensitive`：`off` | `tools`（默认：`tools`）
- `logging.redactPatterns`：正则字符串列表，覆盖默认集

脱敏**仅影响控制台输出**，不修改文件日志。

## 诊断 + OpenTelemetry

诊断是用于模型运行**和**消息流遥测（webhooks、队列、会话状态）的结构化、机器可读事件。它们**不**替代日志；存在的目的是为指标、追踪和其他导出器提供数据。

诊断事件在进程内发出，但导出器仅在诊断 + 导出器插件启用时才挂载。

### OpenTelemetry vs OTLP

- **OpenTelemetry (OTel)**：追踪、指标和日志的数据模型 + SDK。
- **OTLP**：用于将 OTel 数据导出到收集器/后端的传输协议。
- 创宇太虚当前通过 **OTLP/HTTP (protobuf)** 导出。

### 导出的信号

- **指标**：计数器 + 直方图（Token 用量、消息流、队列）。
- **追踪**：模型使用 + webhook/消息处理的 span。
- **日志**：当 `diagnostics.otel.logs` 启用时通过 OTLP 导出。日志量可能很大；注意 `logging.level` 和导出器过滤器。

### 诊断事件目录

模型使用：

- `model.usage`：Token、费用、时长、上下文、供应商/模型/频道、会话 ID。

消息流：

- `webhook.received`：各频道的 webhook 入站。
- `webhook.processed`：webhook 处理完成 + 时长。
- `webhook.error`：webhook 处理器错误。
- `message.queued`：消息入队等待处理。
- `message.processed`：处理结果 + 时长 + 可选错误。

队列 + 会话：

- `queue.lane.enqueue`：命令队列通道入队 + 深度。
- `queue.lane.dequeue`：命令队列通道出队 + 等待时间。
- `session.state`：会话状态转换 + 原因。
- `session.stuck`：会话卡住警告 + 时长。
- `run.attempt`：运行重试/尝试元数据。
- `diagnostic.heartbeat`：聚合计数器（webhooks/队列/会话）。

### 启用诊断（无导出器）

若只需要诊断事件供插件或自定义接收器使用：

```json
{
  "diagnostics": {
    "enabled": true
  }
}
```

### 诊断标志（定向日志）

使用标志来启用额外的定向调试日志，而无需提升 `logging.level`。
标志不区分大小写，支持通配符（如 `telegram.*` 或 `*`）。

```json
{
  "diagnostics": {
    "flags": ["telegram.http"]
  }
}
```

环境变量覆盖（一次性）：

```
OPENACOSMI_DIAGNOSTICS=telegram.http,telegram.payload
```

注意事项：

- 标志日志写入标准日志文件（与 `logging.file` 相同）。
- 输出仍按 `logging.redactSensitive` 进行脱敏。
- 完整指南：[诊断标志](/diagnostics/flags)。

### 导出到 OpenTelemetry

诊断可通过 `diagnostics-otel` 插件（OTLP/HTTP）导出。适用于任何接受 OTLP/HTTP 的 OpenTelemetry 收集器/后端。

```json
{
  "plugins": {
    "allow": ["diagnostics-otel"],
    "entries": {
      "diagnostics-otel": {
        "enabled": true
      }
    }
  },
  "diagnostics": {
    "enabled": true,
    "otel": {
      "enabled": true,
      "endpoint": "http://otel-collector:4318",
      "protocol": "http/protobuf",
      "serviceName": "openacosmi-gateway",
      "traces": true,
      "metrics": true,
      "logs": true,
      "sampleRate": 0.2,
      "flushIntervalMs": 60000
    }
  }
}
```

注意事项：

- 启用插件：`openacosmi plugins enable diagnostics-otel`。
- `protocol` 当前仅支持 `http/protobuf`。`grpc` 会被忽略。
- 指标包括 Token 用量、费用、上下文大小、运行时长，以及消息流计数器/直方图（webhooks、队列、会话状态、队列深度/等待时间）。
- 追踪/指标可通过 `traces` / `metrics` 切换（默认：开启）。启用时追踪包含模型使用 span 和 webhook/消息处理 span。
- 当收集器需要认证时设置 `headers`。
- 支持的环境变量：`OTEL_EXPORTER_OTLP_ENDPOINT`、`OTEL_SERVICE_NAME`、`OTEL_EXPORTER_OTLP_PROTOCOL`。

### 导出的指标（名称 + 类型）

模型使用：

- `openacosmi.tokens`（计数器，属性：`openacosmi.token`、`openacosmi.channel`、`openacosmi.provider`、`openacosmi.model`）
- `openacosmi.cost.usd`（计数器，属性：`openacosmi.channel`、`openacosmi.provider`、`openacosmi.model`）
- `openacosmi.run.duration_ms`（直方图，属性：`openacosmi.channel`、`openacosmi.provider`、`openacosmi.model`）
- `openacosmi.context.tokens`（直方图，属性：`openacosmi.context`、`openacosmi.channel`、`openacosmi.provider`、`openacosmi.model`）

消息流：

- `openacosmi.webhook.received`（计数器，属性：`openacosmi.channel`、`openacosmi.webhook`）
- `openacosmi.webhook.error`（计数器，属性：`openacosmi.channel`、`openacosmi.webhook`）
- `openacosmi.webhook.duration_ms`（直方图，属性：`openacosmi.channel`、`openacosmi.webhook`）
- `openacosmi.message.queued`（计数器，属性：`openacosmi.channel`、`openacosmi.source`）
- `openacosmi.message.processed`（计数器，属性：`openacosmi.channel`、`openacosmi.outcome`）
- `openacosmi.message.duration_ms`（直方图，属性：`openacosmi.channel`、`openacosmi.outcome`）

队列 + 会话：

- `openacosmi.queue.lane.enqueue`（计数器，属性：`openacosmi.lane`）
- `openacosmi.queue.lane.dequeue`（计数器，属性：`openacosmi.lane`）
- `openacosmi.queue.depth`（直方图，属性：`openacosmi.lane` 或 `openacosmi.channel=heartbeat`）
- `openacosmi.queue.wait_ms`（直方图，属性：`openacosmi.lane`）
- `openacosmi.session.state`（计数器，属性：`openacosmi.state`、`openacosmi.reason`）
- `openacosmi.session.stuck`（计数器，属性：`openacosmi.state`）
- `openacosmi.session.stuck_age_ms`（直方图，属性：`openacosmi.state`）
- `openacosmi.run.attempt`（计数器，属性：`openacosmi.attempt`）

### 导出的 Span（名称 + 关键属性）

- `openacosmi.model.usage`
  - `openacosmi.channel`、`openacosmi.provider`、`openacosmi.model`
  - `openacosmi.sessionKey`、`openacosmi.sessionId`
  - `openacosmi.tokens.*`（input/output/cache_read/cache_write/total）
- `openacosmi.webhook.processed`
  - `openacosmi.channel`、`openacosmi.webhook`、`openacosmi.chatId`
- `openacosmi.webhook.error`
  - `openacosmi.channel`、`openacosmi.webhook`、`openacosmi.chatId`、`openacosmi.error`
- `openacosmi.message.processed`
  - `openacosmi.channel`、`openacosmi.outcome`、`openacosmi.chatId`、`openacosmi.messageId`、`openacosmi.sessionKey`、`openacosmi.sessionId`、`openacosmi.reason`
- `openacosmi.session.stuck`
  - `openacosmi.state`、`openacosmi.ageMs`、`openacosmi.queueDepth`、`openacosmi.sessionKey`、`openacosmi.sessionId`

### 采样 + 刷新

- 追踪采样：`diagnostics.otel.sampleRate`（0.0–1.0，仅根 span）。
- 指标导出间隔：`diagnostics.otel.flushIntervalMs`（最小 1000ms）。

### 协议说明

- OTLP/HTTP 端点可通过 `diagnostics.otel.endpoint` 或 `OTEL_EXPORTER_OTLP_ENDPOINT` 设置。
- 若端点已包含 `/v1/traces` 或 `/v1/metrics`，则按原样使用。
- 若端点已包含 `/v1/logs`，日志导出也按原样使用。
- `diagnostics.otel.logs` 启用主日志器输出的 OTLP 日志导出。

### 日志导出行为

- OTLP 日志使用与 `logging.file` 相同的结构化记录。
- 遵循 `logging.level`（文件日志级别）。控制台脱敏**不**适用于 OTLP 日志。
- 高流量实例应优先使用 OTLP 收集器端的采样/过滤。

## 故障排查

- **网关不可达？** 先运行 `openacosmi doctor`。
- **日志为空？** 检查网关是否正在运行并写入 `logging.file` 指定的文件路径。
- **需要更多细节？** 将 `logging.level` 设为 `debug` 或 `trace` 后重试。
