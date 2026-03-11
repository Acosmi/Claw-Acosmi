---
name: media
description: "媒体子系统：通过 spawn_media_agent 委托热点发现、内容创作与多平台发布"
---

# 媒体子系统

## 架构定位

Media Agent 是独立的媒体运营子智能体，通过 `spawn_media_agent` 委托任务。
它在独立 LLM 会话中运行，负责热点发现、内容创作和多平台发布。

**不在能力树中注册静态节点**，仅作为 subagent 通过 `spawn_media_agent`（`Bindable: false`）调用。

## 何时使用 spawn_media_agent

| 场景 | 工具 | 原因 |
|------|------|------|
| 热点话题发现与内容创作 | `spawn_media_agent` | 独立会话 + 媒体专属工具链 |
| 多平台内容发布 | `spawn_media_agent` | 需要审批流 + 平台适配 |
| 简单文件转发到频道 | `send_media` 直接使用 | 无需启动子智能体 |
| 编码任务 | `spawn_coder_agent` | 媒体 agent 不处理编程 |

**规则**: 媒体运营任务 → `spawn_media_agent`；简单文件发送 → `send_media`。

## spawn_media_agent 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `task_brief` | 是 | 任务描述（≤500 字符，明确内容主题或目标平台） |
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
- 不处理编程任务（用 `spawn_coder_agent`）
- 不处理桌面 UI 操作（用 `spawn_argus_agent`）

## 相关运营技能

如需平台级 SOP，请继续查阅：

- `media-trending`：热点抓取、筛选与选题判断
- `wechat-mp-ops`：公众号授权、规则与 API 发布
- `xiaohongshu-ops`：小红书 Cookie 登录、发布与互动
