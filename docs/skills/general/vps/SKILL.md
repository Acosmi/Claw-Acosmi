---
name: vps
description: "VPS 云服务器托管（Oracle/Fly/Hetzner/GCP/exe.dev）"
---

# VPS 云服务器托管

本枢纽文档链接了支持的 VPS/托管指南，并从宏观层面说明了云端部署的工作原理。

## 选择供应商

- **Railway**（一键 + 浏览器设置）：[Railway](/install/railway)
- **Northflank**（一键 + 浏览器设置）：[Northflank](/install/northflank)
- **Oracle Cloud（永久免费层）**：[Oracle](/platforms/oracle) — $0/月（Always Free, ARM；容量/注册可能不稳定）
- **Fly.io**：[Fly.io](/install/fly)
- **Hetzner（Docker）**：[Hetzner](/install/hetzner)
- **GCP（Compute Engine）**：[GCP](/install/gcp)
- **exe.dev**（虚拟机 + HTTPS 代理）：[exe.dev](/install/exe-dev)
- **AWS（EC2/Lightsail/免费层）**：同样适用。视频教程：
  [https://x.com/techfrenAJ/status/2014934471095812547](https://x.com/techfrenAJ/status/2014934471095812547)

## 云端部署工作原理

- **网关在 VPS 上运行**，拥有状态和工作区。
- 你通过**控制面板 UI** 或 **Tailscale/SSH** 从笔记本/手机连接。
- 将 VPS 视为数据源头，并**备份**状态和工作区。
- 安全默认值：保持网关在回环地址监听，通过 SSH 隧道或 Tailscale Serve 访问。
  若绑定到 `lan`/`tailnet`，需设置 `gateway.auth.token` 或 `gateway.auth.password`。

远程访问：[网关远程访问](/gateway/remote)
平台枢纽：[平台](/platforms)

## 在 VPS 上使用节点

你可以将网关部署在云端，并在本地设备（Mac/iOS/Android/无头服务器）上配对**节点**。节点提供本地屏幕/摄像头/画布和 `system.run` 能力，而网关留在云端。

文档：[节点](/nodes)、[节点 CLI](/cli/nodes)
