---
name: media-agent
description: "Media Agent: delegate media operations (trending discovery, content composition, multi-platform publishing) to spawn_media_agent"
metadata: |
  { "openacosmi": { "emoji": "📰" } }
---

# Media Agent — 媒体子智能体使用指南

Media Agent 是独立的媒体运营子智能体，通过 `spawn_media_agent` 工具委派任务。
它在独立 LLM 会话中运行，负责热点发现、内容创作和多平台发布（微信公众号、小红书、网站等）。
所有发布操作需要审批后才会上线。

## 何时用 spawn_media_agent

| 场景 | 工具 | 原因 |
|------|------|------|
| 热点话题发现与内容创作 | spawn_media_agent | 独立会话 + 媒体专属工具链 |
| 多平台内容发布 | spawn_media_agent | 需要审批流 + 平台适配 |
| 简单文件转发到频道 | send_media 直接使用 | 无需启动子智能体 |
| 编码任务 | spawn_coder_agent | 媒体 agent 不处理编程 |

**规则**: 媒体运营任务 → spawn_media_agent；简单文件发送 → send_media。

## spawn_media_agent 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `task_brief` | 是 | 任务描述（<=500 字符，明确内容主题或目标平台） |
| `scope` | 是 | 允许操作的文件/目录范围及权限 |
| `constraints` | 否 | 限制条件（如 no_network 等） |
| `timeout_ms` | 否 | 超时时间（毫秒） |

## 工作流示例

```
用户: "帮我写一篇关于 AI Agent 发展趋势的公众号文章"

主智能体判断: 内容创作 → 委托 Media Agent
工具调用: spawn_media_agent(
  task_brief="撰写一篇关于 AI Agent 2026 年发展趋势的微信公众号文章，
              包含行业数据引用，适合技术受众阅读",
  scope=[{path: "content/articles/", permissions: ["read", "write"]}]
)

Media Agent 执行 → 返回内容草稿
主智能体审核 → 汇报用户
```

## 权限与边界

- 所有发布操作需经审批，不会自动上线
- 媒体 agent 可访问 scope 范围内的文件
- 不处理编程任务（应使用 spawn_coder_agent）
- 不处理桌面 UI 操作（应使用 spawn_argus_agent）
