---
name: 严格边界控制
description: 防止上下文污染与 token 浪费，所有操作精确限定在相关模块范围内
tools: []
metadata:
  emoji: "🔒"
  category: claude
  requires: []
user-invocable: false
disable-model-invocation: true
---

# 技能 1: 严格边界控制

> 在多语言单仓库中防止 token 浪费和上下文污染。

## 适用场景

本代码库是 Rust CLI (`cli-rust/`) + Go 后端 (`backend/`) + 前端 (`ui/`) + macOS 原生 (`apps/macos/`) 的 polyglot 单仓库。
无约束的全局扫描会把不相关代码灌入上下文窗口，降低回复质量并浪费 token。
所有探索必须有明确的路径范围。

## 规则

### 1.1 工作空间边界锁定

| 范围等级 | 允许路径 | 说明 |
|----------|---------|------|
| **主要** | 当前任务涉及的具体模块/目录 | 由任务描述决定 |
| **支撑** | `cli-rust/crates/oa-types/`, `cli-rust/crates/oa-config/`, `cli-rust/crates/oa-infra/` | 共享类型与基础设施 |
| **后端** | `backend/internal/gateway/`, `backend/internal/agents/`, `backend/pkg/types/` | Go 后端核心 |
| **前端** | `ui/src/ui/` | 前端视图与控制器 |
| **文档** | `docs/`, `docs/claude/`, `docs/skills/` | 文档与技能 |

超出这些路径范围需要**明确的用户授权**或已明确的依赖关系。

### 1.2 禁止操作

以下操作在项目根目录 (`/`) **严格禁止**：

- `find .` / `tree` / `ls -R`（递归列目录）
- 无路径约束的 `grep -r` / `rg`
- 对 >500 行文件执行无 `offset`/`limit` 的 `cat` / `Read`
- 根级别的 `**/*` glob 匹配

### 1.3 探索协议

需要当前范围外的信息时：

1. **先确认** — 不确定时向用户确认目标路径
2. **单层 ls** — 每次只探索一层目录
3. **定向读取** — 对大文件使用 `Read` + `offset`/`limit`，只读相关段落
4. **记录原因** — 跨模块访问时在当前追踪文档中注明原因

### 1.4 上下文预算纪律

- 搜索时优先使用 `Grep` + `head_limit`，避免开放式搜索
- 读取 `Cargo.toml` / `go.mod` 只读 `[dependencies]` 段
- 在对话中内联总结发现，而非大段原始文件内容
- 多文件并行搜索时使用 Agent 子智能体，避免主上下文膨胀
