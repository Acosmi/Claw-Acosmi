---
name: qwen
description: "在创宇太虚中使用 Qwen OAuth（免费套餐）"
---

# Qwen

Qwen 为 Qwen Coder 和 Qwen Vision 模型提供免费套餐 OAuth 流程（每天 2,000 次请求，受 Qwen 速率限制约束）。

## 启用插件

```bash
openacosmi plugins enable qwen-portal-auth
```

启用后重启网关。

## 认证

```bash
openacosmi models auth login --provider qwen-portal --set-default
```

这会运行 Qwen device-code OAuth 流程，并将供应商条目写入你的 `models.json`（以及一个 `qwen` 别名用于快速切换）。

## 模型 ID

- `qwen-portal/coder-model`
- `qwen-portal/vision-model`

切换模型：

```bash
openacosmi models set qwen-portal/coder-model
```

## 复用 Qwen Code CLI 登录

若你已通过 Qwen Code CLI 登录，创宇太虚会在加载认证存储时从 `~/.qwen/oauth_creds.json` 同步凭据。你仍需有 `models.providers.qwen-portal` 条目（使用上述登录命令创建）。

## 备注

- Token 自动刷新；刷新失败或访问被撤销时重新运行登录命令。
- 默认 base URL：`https://portal.qwen.ai/v1`（若 Qwen 提供不同端点，可通过 `models.providers.qwen-portal.baseUrl` 覆盖）。
- 供应商通用规则参见 [模型供应商](/concepts/model-providers)。
