---
name: mcp-remote-ops
description: "远程 MCP 运维：连接状态、工具列表与重连控制"
---

# MCP Remote 运维

用于远程 MCP 网桥的状态检查、工具枚举与连接控制。

## 覆盖方法
- `mcp.remote.status`
- `mcp.remote.tools`
- `mcp.remote.connect`

## 推荐流程
1. 先看 `mcp.remote.status` 确认连接状态机。
2. 用 `mcp.remote.tools` 核对已暴露工具数量与前缀。
3. 故障时用 `mcp.remote.connect` 执行 `refresh/reconnect/connect`。

## 风险控制
- 反复 reconnect 前先定位鉴权或网络问题。
- 工具列表突变需要做权限复核。

## 成功判定
- 状态稳定在已连接，工具列表可持续返回。
