---
name: pi-dev
description: 创宇太虚（Claw Acosmi） 开发工作流：构建、测试、调试（Rust+Go+TypeScript 混合架构）
---

# 创宇太虚（Claw Acosmi） 开发工作流

创宇太虚（Claw Acosmi） 是 **Rust + Go + TypeScript** 混合架构：

- **Rust**：原生沙箱（oa-sandbox）、编程子智能体（oa-coder）、持久化 Worker
- **Go**：网关（Gateway）、Agent 运行时、记忆系统（太虚永忆）、工具层
- **TypeScript**：前端 UI（`ui/`）

## 构建命令

```bash
make dev          # 完整编译 Argus + 启动 Gateway（dev 模式）
make gateway      # 仅编译 + 启动 Go Gateway（port 19001）
make build        # 编译全部组件（不启动）
make argus        # 仅构建 Argus 视觉子智能体

# Rust 组件
cd cli-rust && cargo build --workspace --release

# 前端
cd ui && npm install && npm run dev
```

## 运行 Agent

```bash
# 直接调用网关二进制
./backend/build/acosmi -dev -port 19001

# 或通过 Makefile
make gateway
```

## 测试

```bash
# Go 测试
go test ./backend/internal/agents/...
go test ./backend/internal/agents/runner/...
go test ./backend/internal/gateway/...

# Rust 测试
cd cli-rust && cargo test --workspace

# 前端测试
cd ui && npm test
```

## 关键路径

| 组件 | 路径 | 语言 |
|------|------|------|
| Gateway 主入口 | `backend/cmd/` | Go |
| Agent 运行时 | `backend/internal/agents/runner/` | Go |
| 工具层 | `backend/internal/agents/tools/` | Go |
| 原生沙箱 | `cli-rust/crates/oa-sandbox/` | Rust |
| oa-coder | `cli-rust/crates/oa-coder/` | Rust |
| 太虚永忆 记忆 | `backend/internal/memory/uhms/` | Go |
| 委托合约 | `backend/internal/agents/runner/delegation_contract.go` | Go |
| 前端 UI | `ui/src/` | TypeScript |

## 状态目录

状态存储于 `~/.openacosmi`（或 `$OPENACOSMI_STATE_DIR`）：

- `openacosmi.json` — 配置
- `credentials/` — 认证 profile 和 token
- `agents/<agentId>/sessions/` — 会话历史
- `agents/<agentId>/sessions.json` — 会话索引

仅重置会话：删除 `agents/<agentId>/sessions/` 和 `agents/<agentId>/sessions.json`，保留 `credentials/`。

## 开发调试技巧

- Gateway 默认 dev 端口 19001，前端 dev 默认连接此端口
- Rust 组件调试：`RUST_LOG=debug cargo run`
- Go 组件调试：`slog` 结构化日志，可用 `-v` 标志提升详细度
- 沙箱测试：`oa-cmd-sandbox run <cmd>` 验证沙箱隔离效果
