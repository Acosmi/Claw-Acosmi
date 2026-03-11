---
name: mcp-automation
description: "MCP 自动化：远程/本地 MCP 服务器连接管理、工具发现与自动化编排"
---

# MCP 自动化技能

## MCP 架构概览

| 类型 | 前缀 | 发现方式 | 能力树位置 |
|------|------|---------|-----------|
| 远程 MCP | `remote_` | 运行时 AgentRemoteTools() | `dynamic/remote_mcp` |
| 本地 MCP | `mcp_` | 运行时 AgentTools() | `dynamic/local_mcp` |

## 远程 MCP 管理

### 连接状态检查
```
mcp.remote.status → 当前连接状态（init/connecting/ready/degraded/stopped）
mcp.remote.tools → 已发现的工具列表
mcp.remote.connect → 手动重连
```

### 健康监控
- 心跳 ping 自动检测连接质量
- 指数退避自动重连（1s→2s→4s→...→60s）
- 连续 3 次失败标记为 `degraded`

### OAuth 令牌管理
- `mcpremote.OAuthTokenManager` 管理令牌刷新
- 令牌过期自动刷新，无需手动干预

## 本地 MCP 管理

### 服务器生命周期
- 本地 MCP 服务器通过 git 安装到工作区
- 命名规则: `mcp_{server}_{tool}`
- 通过 `McpLocalManager` 管理

### 工具发现
- 服务器启动时自动发现工具
- 工具以 `mcp_` 前缀暴露给主智能体
- 意图路由：`task_light` 即可调用

## 插件集成

### 插件系统
- 插件通过 `plugin.*` RPC 管理
- 支持: 发现、配置、启用/禁用、hooks
- Provider 认证: API key / OAuth
- 频道注册: 插件可注册自定义频道

### 插件操作
```
plugin.list → 已安装插件
plugin.config.get/set → 插件配置
plugin.enable/disable → 启用/停用
```

## 自动化编排模式

### 1. MCP 工具链组合
```
远程 MCP 获取数据 → 本地处理 → 结果投递
```

### 2. 事件驱动自动化
```
hooks-agent 收到 Webhook → 调用 MCP 工具 → 结果发送到频道
```

### 3. 定时 MCP 任务
```
cron 触发 → Agent 调用 remote_*/mcp_* 工具 → 报告结果
```

## 故障排除

| 问题 | 诊断 | 修复 |
|------|------|------|
| 远程不可达 | `mcp.remote.status` | 检查网络 + OAuth 令牌 |
| 工具列表为空 | `mcp.remote.tools` | 确认服务端注册了工具 |
| 本地服务器崩溃 | 日志 + 进程状态 | 重启服务器 / 检查依赖 |
