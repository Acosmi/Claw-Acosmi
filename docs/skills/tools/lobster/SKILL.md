---
name: lobster
description: "Claw Acosmi 的类型化工作流运行时，支持可恢复的审批门控。"
---

# Lobster

Lobster 是一个工作流 Shell，让 Claw Acosmi 将多步骤工具序列作为单一、确定性的操作运行，并带有显式审批检查点。

## 亮点

您的助手可以构建管理自身的工具。请求一个工作流，30 分钟后您就有了一个 CLI 加管道，作为一次调用运行。Lobster 是缺失的拼图：确定性管道、显式审批和可恢复状态。

## 为什么

目前，复杂工作流需要多次来回的工具调用。每次调用消耗 Token，LLM 必须编排每一步。Lobster 将编排移入类型化运行时：

- **一次调用代替多次**：Claw Acosmi 运行一次 Lobster 工具调用并获得结构化结果。
- **内置审批**：副作用（发送邮件、发布评论）会暂停工作流，直到显式批准。
- **可恢复**：暂停的工作流返回一个 Token；批准后恢复而无需重新运行所有步骤。

## 为什么选择 DSL 而非普通程序？

Lobster 刻意保持精简。目标不是"一门新语言"，而是一个可预测的、AI 友好的管道规范，带有一等公民的审批和恢复 Token。

- **审批/恢复是内置的**：普通程序可以提示人类，但无法在不自行发明运行时的情况下_暂停并以持久 Token 恢复_。
- **确定性 + 可审计**：管道是数据，因此容易记录、比较、重放和审查。
- **为 AI 限定的表面**：微型语法 + JSON 管道减少了"创意性"代码路径，使验证变得现实。
- **内置安全策略**：超时、输出上限、沙箱检查和白名单由运行时执行，而非每个脚本。
- **仍然可编程**：每一步可以调用任何 CLI 或脚本。如果您需要 JS/TS，可以从代码生成 `.lobster` 文件。

## 工作原理

Claw Acosmi 在**工具模式**下启动本地 `lobster` CLI，并从 stdout 解析 JSON 信封。如果管道因审批而暂停，工具返回 `resumeToken` 以便稍后继续。

## 模式：小型 CLI + JSON 管道 + 审批

构建输出 JSON 的微型命令，然后将它们链入单个 Lobster 调用。（下面是示例命令名 — 换成您自己的。）

```bash
inbox list --json
inbox categorize --json
inbox apply --json
```

```json
{
  "action": "run",
  "pipeline": "exec --json --shell 'inbox list --json' | exec --stdin json --shell 'inbox categorize --json' | exec --stdin json --shell 'inbox apply --json' | approve --preview-from-stdin --limit 5 --prompt 'Apply changes?'",
  "timeoutMs": 30000
}
```

如果管道请求审批，使用 Token 恢复：

```json
{
  "action": "resume",
  "token": "<resumeToken>",
  "approve": true
}
```

AI 触发工作流；Lobster 执行步骤。审批门控使副作用显式且可审计。

示例：将输入项映射为工具调用：

```bash
gog.gmail.search --query 'newer_than:1d' \
  | openacosmi.invoke --tool message --action send --each --item-key message --args-json '{"provider":"telegram","to":"..."}'
```

## JSON-only LLM 步骤（llm-task）

对于需要**结构化 LLM 步骤**的工作流，启用可选的 `llm-task` 插件工具并从 Lobster 调用它。这保持工作流确定性，同时仍允许使用模型进行分类/摘要/草稿。

启用工具：

```json
{
  "plugins": {
    "entries": {
      "llm-task": { "enabled": true }
    }
  },
  "agents": {
    "list": [
      {
        "id": "main",
        "tools": { "allow": ["llm-task"] }
      }
    ]
  }
}
```

在管道中使用：

```lobster
openacosmi.invoke --tool llm-task --action json --args-json '{
  "prompt": "Given the input email, return intent and draft.",
  "input": { "subject": "Hello", "body": "Can you help?" },
  "schema": {
    "type": "object",
    "properties": {
      "intent": { "type": "string" },
      "draft": { "type": "string" }
    },
    "required": ["intent", "draft"],
    "additionalProperties": false
  }
}'
```

详见 [LLM Task](/tools/llm-task) 了解详情和配置选项。

## 工作流文件（.lobster）

Lobster 可以运行 YAML/JSON 工作流文件，包含 `name`、`args`、`steps`、`env`、`condition` 和 `approval` 字段。在 Claw Acosmi 工具调用中，将 `pipeline` 设为文件路径。

```yaml
name: inbox-triage
args:
  tag:
    default: "family"
steps:
  - id: collect
    command: inbox list --json
  - id: categorize
    command: inbox categorize --json
    stdin: $collect.stdout
  - id: approve
    command: inbox apply --approve
    stdin: $categorize.stdout
    approval: required
  - id: execute
    command: inbox apply --execute
    stdin: $categorize.stdout
    condition: $approve.approved
```

说明：

- `stdin: $step.stdout` 和 `stdin: $step.json` 传递前一步骤的输出。
- `condition`（或 `when`）可以根据 `$step.approved` 控制步骤执行。

## 安装 Lobster

在运行 Claw Acosmi Gateway 的**同一主机**上安装 Lobster CLI（参见 [Lobster 仓库](https://github.com/openacosmi/lobster)），并确保 `lobster` 在 `PATH` 中。如果要使用自定义二进制位置，在工具调用中传递**绝对**`lobsterPath`。

## 启用工具

Lobster 是一个**可选的**插件工具（默认未启用）。

推荐（累加，安全）：

```json
{
  "tools": {
    "alsoAllow": ["lobster"]
  }
}
```

或每 Agent：

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "tools": {
          "alsoAllow": ["lobster"]
        }
      }
    ]
  }
}
```

避免使用 `tools.allow: ["lobster"]`，除非您打算在限制性白名单模式下运行。

注意：白名单对可选插件是可选的。如果您的白名单仅列出插件工具（如 `lobster`），Claw Acosmi 保持核心工具启用。要限制核心工具，也需在白名单中包含您需要的核心工具或组。

## 示例：邮件分类

无 Lobster：

```
用户："帮我检查邮件并起草回复"
→ openacosmi 调用 gmail.list
→ LLM 总结
→ 用户："给 #2 和 #5 起草回复"
→ LLM 起草
→ 用户："发送 #2"
→ openacosmi 调用 gmail.send
（每天重复，不记住已分类的内容）
```

有 Lobster：

```json
{
  "action": "run",
  "pipeline": "email.triage --limit 20",
  "timeoutMs": 30000
}
```

返回 JSON 信封（截断）：

```json
{
  "ok": true,
  "status": "needs_approval",
  "output": [{ "summary": "5 need replies, 2 need action" }],
  "requiresApproval": {
    "type": "approval_request",
    "prompt": "Send 2 draft replies?",
    "items": [],
    "resumeToken": "..."
  }
}
```

用户批准 → 恢复：

```json
{
  "action": "resume",
  "token": "<resumeToken>",
  "approve": true
}
```

一个工作流。确定性。安全。

## 工具参数

### `run`

在工具模式下运行管道。

```json
{
  "action": "run",
  "pipeline": "gog.gmail.search --query 'newer_than:1d' | email.triage",
  "cwd": "/path/to/workspace",
  "timeoutMs": 30000,
  "maxStdoutBytes": 512000
}
```

运行带参数的工作流文件：

```json
{
  "action": "run",
  "pipeline": "/path/to/inbox-triage.lobster",
  "argsJson": "{\"tag\":\"family\"}"
}
```

### `resume`

审批后继续暂停的工作流。

```json
{
  "action": "resume",
  "token": "<resumeToken>",
  "approve": true
}
```

### 可选输入

- `lobsterPath`：Lobster 二进制的绝对路径（省略则使用 `PATH`）。
- `cwd`：管道的工作目录（默认为当前进程工作目录）。
- `timeoutMs`：超过此时间后终止子进程（默认 20000）。
- `maxStdoutBytes`：stdout 超过此大小后终止子进程（默认 512000）。
- `argsJson`：传递给 `lobster run --args-json` 的 JSON 字符串（仅工作流文件）。

## 输出信封

Lobster 返回一个 JSON 信封，包含三种状态之一：

- `ok` → 成功完成
- `needs_approval` → 已暂停；需要 `requiresApproval.resumeToken` 来恢复
- `cancelled` → 被显式拒绝或取消

工具在 `content`（格式化 JSON）和 `details`（原始对象）中呈现信封。

## 审批

如果存在 `requiresApproval`，检查提示并决定：

- `approve: true` → 恢复并继续副作用
- `approve: false` → 取消并完成工作流

使用 `approve --preview-from-stdin --limit N` 将 JSON 预览附加到审批请求，无需自定义 jq/heredoc 粘合。恢复 Token 现在更紧凑：Lobster 在其状态目录下存储工作流恢复状态，并返回一个小的 Token 键。

## OpenProse

OpenProse 与 Lobster 搭配良好：使用 `/prose` 编排多 Agent 准备工作，然后运行 Lobster 管道进行确定性审批。如果 Prose 程序需要 Lobster，通过 `tools.subagents.tools` 为子 Agent 允许 `lobster` 工具。参见 [OpenProse](/prose)。

## 安全

- **仅本地子进程** — 插件本身不进行网络调用。
- **无密钥** — Lobster 不管理 OAuth；它调用执行 OAuth 的 Claw Acosmi 工具。
- **沙箱感知** — 工具上下文在沙箱中时禁用。
- **加固** — 如指定，`lobsterPath` 必须为绝对路径；强制执行超时和输出上限。

## 故障排除

- **`lobster subprocess timed out`** → 增加 `timeoutMs`，或拆分长管道。
- **`lobster output exceeded maxStdoutBytes`** → 提高 `maxStdoutBytes` 或减少输出大小。
- **`lobster returned invalid JSON`** → 确保管道在工具模式下运行且仅打印 JSON。
- **`lobster failed (code …)`** → 在终端中运行相同管道以检查 stderr。

## 了解更多

- [插件](/tools/plugin)
- [插件工具开发](/plugins/agent-tools)

## 案例研究：社区工作流

一个公开示例："第二大脑" CLI + Lobster 管道，管理三个 Markdown 知识库（个人、伴侣、共享）。CLI 输出 JSON 用于统计、收件箱列表和过期扫描；Lobster 将这些命令链入 `weekly-review`、`inbox-triage`、`memory-consolidation` 和 `shared-task-sync` 等工作流，每个都带审批门控。AI 在可用时处理判断（分类），不可用时回退到确定性规则。

- 帖子：[https://x.com/plattenschieber/status/2014508656335770033](https://x.com/plattenschieber/status/2014508656335770033)
- 仓库：[https://github.com/bloomedai/brain-cli](https://github.com/bloomedai/brain-cli)
