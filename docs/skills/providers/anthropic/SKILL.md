---
name: anthropic
description: "通过 API 密钥或 setup-token 在创宇太虚中使用 Anthropic Claude"
---

# Anthropic (Claude)

Anthropic 开发 **Claude** 模型家族并通过 API 提供访问。在创宇太虚中，你可以通过 API 密钥或 **setup-token** 进行认证。

## 方案 A：Anthropic API 密钥

**适用于：** 标准 API 访问和按量计费。
在 Anthropic 控制台创建 API 密钥。

### CLI 设置

```bash
openacosmi onboard
# 选择：Anthropic API key

# 或非交互式
openacosmi onboard --anthropic-api-key "$ANTHROPIC_API_KEY"
```

### 配置示例

```json5
{
  env: { ANTHROPIC_API_KEY: "sk-ant-..." },
  agents: { defaults: { model: { primary: "anthropic/claude-opus-4-6" } } },
}
```

## 提示缓存（Anthropic API）

创宇太虚支持 Anthropic 的提示缓存功能。这是**仅 API** 功能；订阅认证不支持缓存设置。

### 配置

在模型配置中使用 `cacheRetention` 参数：

| 值 | 缓存时长 | 说明 |
| --- | --- | --- |
| `none` | 不缓存 | 禁用提示缓存 |
| `short` | 5 分钟 | API Key 认证的默认值 |
| `long` | 1 小时 | 扩展缓存（需要 beta 标志） |

```json5
{
  agents: {
    defaults: {
      models: {
        "anthropic/claude-opus-4-6": {
          params: { cacheRetention: "long" },
        },
      },
    },
  },
}
```

### 默认值

使用 Anthropic API Key 认证时，创宇太虚自动为所有 Anthropic 模型应用 `cacheRetention: "short"`（5 分钟缓存）。你可以在配置中显式设置 `cacheRetention` 来覆盖。

### 旧版参数

旧版 `cacheControlTtl` 参数仍支持向后兼容：

- `"5m"` 映射到 `short`
- `"1h"` 映射到 `long`

建议迁移到新的 `cacheRetention` 参数。

创宇太虚在 Anthropic API 请求中包含 `extended-cache-ttl-2025-04-11` beta 标志；如覆盖供应商头需保留它（参见 [/gateway/configuration](/gateway/configuration)）。

## 方案 B：Claude setup-token

**适用于：** 使用 Claude 订阅。

### 获取 setup-token

setup-token 由 **Claude Code CLI** 创建，非 Anthropic 控制台。可在**任何机器**上运行：

```bash
claude setup-token
```

将 token 粘贴到创宇太虚（向导：**Anthropic token (paste setup-token)**），或在网关主机上运行：

```bash
openacosmi models auth setup-token --provider anthropic
```

若在其他机器上生成了 token：

```bash
openacosmi models auth paste-token --provider anthropic
```

### CLI 设置（setup-token）

```bash
# 在 onboarding 期间粘贴 setup-token
openacosmi onboard --auth-choice setup-token
```

### 配置示例（setup-token）

```json5
{
  agents: { defaults: { model: { primary: "anthropic/claude-opus-4-6" } } },
}
```

## 备注

- 使用 `claude setup-token` 生成 token 并粘贴，或在网关主机上运行 `openacosmi models auth setup-token`。
- 如看到 "OAuth token refresh failed …"（Claude 订阅），使用 setup-token 重新认证。参见 [/gateway/troubleshooting#oauth-token-refresh-failed-anthropic-claude-subscription](/gateway/troubleshooting#oauth-token-refresh-failed-anthropic-claude-subscription)。
- 认证详情和复用规则参见 [/concepts/oauth](/concepts/oauth)。

## 故障排查

**401 错误 / token 突然失效**

- Claude 订阅认证可能过期或被撤销。重新运行 `claude setup-token` 并粘贴到**网关主机**。
- 若 Claude CLI 登录在其他机器上，在网关主机上使用 `openacosmi models auth paste-token --provider anthropic`。

**未找到供应商 "anthropic" 的 API 密钥**

- 认证是**按智能体**的。新智能体不继承主智能体的密钥。
- 为该智能体重新运行 onboarding，或在网关主机上粘贴 setup-token / API 密钥，然后用 `openacosmi models status` 验证。

**未找到配置 `anthropic:default` 的凭据**

- 运行 `openacosmi models status` 查看当前活跃的认证配置。
- 重新运行 onboarding，或为该配置粘贴 setup-token / API 密钥。

**无可用认证配置（全部在冷却/不可用）**

- 检查 `openacosmi models status --json` 中的 `auth.unusableProfiles`。
- 添加另一个 Anthropic 配置或等待冷却。

更多：[/gateway/troubleshooting](/gateway/troubleshooting) 和 [/help/faq](/help/faq)。
