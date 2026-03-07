---
name: prose
description: "OpenProse：散文工作流、斜杠命令与状态管理"
---

# OpenProse

OpenProse 是一种可移植的、以 Markdown 为核心的工作流格式，用于编排 AI 会话。在创宇太虚中，它作为插件提供，安装 OpenProse 技能包和 `/prose` 斜杠命令。程序保存在 `.prose` 文件中，可以生成多个子智能体并实现显式控制流。

官方网站：[https://www.prose.md](https://www.prose.md)

## 功能

- 多智能体研究 + 综合，支持显式并行。
- 可重复的审批安全工作流（代码审查、事件分类、内容管线）。
- 可复用的 `.prose` 程序，可在多个支持的智能体运行时上运行。

## 安装与启用

捆绑插件默认未启用。启用 OpenProse：

```bash
openacosmi plugins enable open-prose
```

启用插件后重启网关。

开发/本地检出：`openacosmi plugins install -l ./extensions/open-prose`

相关文档：[插件](/tools/plugin)、[插件清单](/plugins/manifest)、[技能](/tools/skills)。

## 斜杠命令

OpenProse 注册 `/prose` 为用户可调用的技能命令。它路由到 OpenProse VM 指令，底层使用创宇太虚工具。

常用命令：

```
/prose help
/prose run <file.prose>
/prose run <handle/slug>
/prose run <https://example.com/file.prose>
/prose compile <file.prose>
/prose examples
/prose update
```

## 示例：一个简单的 `.prose` 文件

```prose
# 两个智能体并行研究 + 综合。

input topic: "我们应该研究什么？"

agent researcher:
  model: sonnet
  prompt: "你负责深入研究并引用来源。"

agent writer:
  model: opus
  prompt: "你负责撰写简洁摘要。"

parallel:
  findings = session: researcher
    prompt: "研究 {topic}。"
  draft = session: writer
    prompt: "总结 {topic}。"

session "将研究结果和草稿合并为最终答案。"
context: { findings, draft }
```

## 文件位置

OpenProse 在工作区内的 `.prose/` 下保存状态：

```
.prose/
├── .env
├── runs/
│   └── {YYYYMMDD}-{HHMMSS}-{random}/
│       ├── program.prose
│       ├── state.md
│       ├── bindings/
│       └── agents/
└── agents/
```

用户级持久化智能体存储在：

```
~/.prose/agents/
```

## 状态模式

OpenProse 支持多种状态后端：

- **filesystem**（默认）：`.prose/runs/...`
- **in-context**：瞬态，适用于小程序
- **sqlite**（实验性）：需要 `sqlite3` 二进制
- **postgres**（实验性）：需要 `psql` 和连接字符串

注意事项：

- sqlite/postgres 为可选启用的实验性功能。
- postgres 凭据会写入子智能体日志；请使用专用的最低权限数据库。

## 远程程序

`/prose run <handle/slug>` 解析为 `https://p.prose.md/<handle>/<slug>`。
直连 URL 按原样获取。这使用 `web_fetch` 工具（或 `exec` 用于 POST）。

## 创宇太虚运行时映射

OpenProse 程序映射到创宇太虚原语：

| OpenProse 概念             | 创宇太虚工具    |
| ------------------------- | -------------- |
| 生成会话 / Task 工具        | `sessions_spawn` |
| 文件读写                    | `read` / `write` |
| 网页获取                    | `web_fetch`      |

若工具白名单禁止了这些工具，OpenProse 程序将无法运行。详见 [技能配置](/tools/skills-config)。

## 安全与审批

应像对待代码一样对待 `.prose` 文件。运行前请先审查。使用创宇太虚的工具白名单和审批门控来控制副作用。

对于确定性的、带审批门控的工作流，可对比 [Lobster](/tools/lobster)。
