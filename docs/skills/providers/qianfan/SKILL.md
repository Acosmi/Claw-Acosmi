---
name: qianfan
description: "通过千帆统一 API 在创宇太虚中访问多种模型"
---

# 千帆供应商指南

千帆是百度的 MaaS 平台，提供**统一 API**，通过单一端点和 API 密钥将请求路由到多种模型。它兼容 OpenAI，大多数 OpenAI SDK 只需切换 base URL 即可使用。

## 前提条件

1. 拥有千帆 API 访问权限的百度云账号
2. 从千帆控制台获取的 API 密钥
3. 系统上已安装创宇太虚

## 获取 API 密钥

1. 访问[千帆控制台](https://console.bce.baidu.com/qianfan/ais/console/apiKey)
2. 创建新应用或选择已有应用
3. 生成 API 密钥（格式：`bce-v3/ALTAK-...`）
4. 复制 API 密钥用于创宇太虚

## CLI 设置

```bash
openacosmi onboard --auth-choice qianfan-api-key
```

## 相关文档

- [创宇太虚配置](/gateway/configuration)
- [模型供应商](/concepts/model-providers)
- [智能体设置](/concepts/agent)
- [千帆 API 文档](https://cloud.baidu.com/doc/qianfan-api/s/3m7of64lb)
