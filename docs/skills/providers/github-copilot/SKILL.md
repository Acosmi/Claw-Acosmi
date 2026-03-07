---
name: github-copilot
description: "从创宇太虚使用设备流登录 GitHub Copilot"
---

# GitHub Copilot

## 什么是 GitHub Copilot？

GitHub Copilot 是 GitHub 的 AI 编码助手。它为你的 GitHub 账户和计划提供 Copilot 模型访问。创宇太虚可以通过两种方式使用 Copilot 作为模型供应商。

## 在创宇太虚中使用 Copilot 的两种方式

### 1）内置 GitHub Copilot 供应商（`github-copilot`）

使用原生设备登录流获取 GitHub token，然后在创宇太虚运行时将其交换为 Copilot API token。这是**默认**且最简单的方式，不需要 VS Code。

### 2）Copilot Proxy 插件（`copilot-proxy`）

使用 **Copilot Proxy** VS Code 扩展作为本地桥接。创宇太虚与代理的 `/v1` 端点通信并使用你在那里配置的模型列表。当你已在 VS Code 中运行 Copilot Proxy 或需要通过它路由时选择此方式。需要启用插件并保持 VS Code 扩展运行。

## CLI 设置

```bash
openacosmi models auth login-github-copilot
```

系统会提示你访问一个 URL 并输入一次性代码。在完成前保持终端打开。

### 可选标志

```bash
openacosmi models auth login-github-copilot --profile-id github-copilot:work
openacosmi models auth login-github-copilot --yes
```

## 设置默认模型

```bash
openacosmi models set github-copilot/gpt-4o
```

### 配置示例

```json5
{
  agents: { defaults: { model: { primary: "github-copilot/gpt-4o" } } },
}
```

## 备注

- 需要交互式 TTY；直接在终端中运行。
- Copilot 模型可用性取决于你的计划；若模型被拒绝，尝试其他 ID（例如 `github-copilot/gpt-4.1`）。
- 登录会将 GitHub token 存储在认证配置存储中，并在创宇太虚运行时将其交换为 Copilot API token。
