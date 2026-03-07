---
name: network
description: "网络枢纽：网关接口、配对、发现与安全"
---

# 网络枢纽

本枢纽文档链接了创宇太虚如何跨 localhost、局域网和 Tailnet 进行连接、配对和安全通信的核心文档。

## 核心模型

- [网关架构](/concepts/architecture)
- [网关协议](/gateway/protocol)
- [网关运维手册](/gateway)
- [Web 接口与绑定模式](/web)

## 配对与身份

- [配对概览（DM + 节点）](/channels/pairing)
- [网关拥有的节点配对](/gateway/pairing)
- [设备 CLI（配对 + 令牌轮换）](/cli/devices)
- [配对 CLI（DM 审批）](/cli/pairing)

本地信任：

- 本地连接（回环地址或网关主机自身的 Tailnet 地址）可以自动批准配对，以保持同主机上的流畅体验。
- 非本地的 Tailnet/局域网客户端仍然需要显式配对审批。

## 发现与传输

- [发现与传输](/gateway/discovery)
- [Bonjour / mDNS](/gateway/bonjour)
- [远程访问（SSH）](/gateway/remote)
- [Tailscale](/gateway/tailscale)

## 节点与传输

- [节点概览](/nodes)
- [Bridge 协议（旧版节点）](/gateway/bridge-protocol)
- [节点运维手册：iOS](/platforms/ios)
- [节点运维手册：Android](/platforms/android)

## 安全

- [安全概览](/gateway/security)
- [网关配置参考](/gateway/configuration)
- [故障排查](/gateway/troubleshooting)
- [诊断工具 Doctor](/gateway/doctor)
