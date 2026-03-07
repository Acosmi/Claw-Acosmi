---
name: pi
description: 创宇太虚（Claw Acosmi） Agent 运行时架构（Rust+Go 混合）
---

# 创宇太虚（Claw Acosmi） Agent 运行时架构

创宇太虚（Claw Acosmi） 采用 **Rust + Go** 混合架构驱动 AI 智能体能力。

## 整体架构

```
┌─────────────────────────────────────────┐
│  前端 UI（TypeScript/Lit）               │
│  WebSocket ↔ Gateway RPC               │
├─────────────────────────────────────────┤
│  Go Gateway（主进程）                    │
│  ├── Agent 运行时（attempt_runner.go）   │
│  ├── 能力注册（capabilities/registry）   │
│  ├── 太虚永忆 记忆系统                      │
│  ├── 委托合约系统                        │
│  └── 多渠道（飞书 WebSocket 等）         │
├─────────────────────────────────────────┤
│  Rust 组件（独立进程/IPC）               │
│  ├── oa-sandbox（原生沙箱，4 平台）      │
│  ├── oa-coder（编程子智能体 MCP）        │
│  └── 持久化 Worker（JSON-Lines 协议）    │
└─────────────────────────────────────────┘
```

## Go Agent 运行时

核心文件：`backend/internal/agents/runner/`

| 文件 | 职责 |
|------|------|
| `attempt_runner.go` | 主运行循环：构建 prompt → 调用 LLM → 执行工具 → 流式回复 |
| `tool_executor.go` | 工具分发：bash/read/write/search/glob/lookup_skill 等 |
| `delegation_contract.go` | 委托合约：能力集、单调衰减、沙箱约束传播 |
| `spawn_coder_agent.go` | spawn_coder_agent 工具：启动 oa-coder 子智能体 |
| `subagent_announce.go` | ThoughtResult 解析，子智能体状态广播 |

## 工具层

`backend/internal/agents/capabilities/registry.go`：单一能力真相源。

核心工具（始终可用）：`bash`、`read_file`、`write_file`、`list_dir`

可选工具（按配置启用）：`browser`、`canvas`、`cron`、`gateway`、`memory_search`、`memory_get`、`message`、`nodes`、`web_fetch`、`web_search`、`image`、`sessions_*`

子智能体入口：`spawn_coder_agent`、`spawn_argus_agent`、`spawn_media_agent`

技能工具：`search_skills`、`lookup_skill`

## System Prompt 构建

`buildSystemPrompt()` 在 `attempt_runner.go` 中动态组装，包含：技能索引、工具说明、沙箱信息、委托合约、记忆上下文、渠道特定内容等。

## 会话管理

- 会话以 JSONL 格式持久化存储：`~/.openacosmi/agents/<agentId>/sessions/`
- 支持会话树（id/parentId 链接）、自动压缩（太虚永忆 Anchored Iterative Summary）
- 多 Auth Profile 轮转，带冷却期追踪和自动故障切换

## Rust 组件集成

### oa-sandbox（原生沙箱）

Go Gateway 通过持久化 Worker（JSON-Lines IPC）与 Rust oa-sandbox 通信：

- `native_bridge.go`：状态机 + 健康监控 + 崩溃恢复
- 沙箱模式：`allowlist`（Docker）| `full`（宿主机）| `deny`（禁止执行）
- NoNetwork 合约 → `docker --network=none` 强制隔离

### oa-coder（编程子智能体）

通过 `spawn_coder_agent` + 委托合约调用：

- Rust MCP Server（JSON-RPC 2.0 stdio）
- 6 工具：edit（9 层模糊匹配）/ read / write / grep（ripgrep）/ glob / bash
- 委托合约控制 scope/constraints/timeout，能力单调衰减

## 多 Provider 支持

`backend/internal/agents/models/providers.go` 注册支持的 LLM 提供商：

| 类型 | 示例 |
|------|------|
| 云端 | anthropic、openai、gemini、qwen、minimax 等 |
| 本地隐私 | ollama（本地模型，零数据外泄）|
| 隐私云端 | venice（服务端隐私保护）|
| 代理 | openrouter、vercel-ai-gateway、cloudflare-ai-gateway |

## 渠道架构

飞书渠道以 WebSocket 长连接模式运行（`larkws.NewClient`），无需公网 URL，支持：

- 文本/富文本/互动卡片消息收发
- 卡片审批交互（权限升级确认、exec approvals）
- `feishu:<chatID>` 独立会话，跨会话事件广播
