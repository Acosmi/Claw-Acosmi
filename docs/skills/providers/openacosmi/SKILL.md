---
name: openacosmi
description: 创宇太虚（Claw Acosmi） Zen 精选模型访问：配置与使用
---

# 创宇太虚（Claw Acosmi） Zen

创宇太虚（Claw Acosmi） Zen 是 创宇太虚（Claw Acosmi） 团队精选的模型访问通道，通过 `openacosmi` provider 路由。

## 快速配置

```bash
openacosmi onboard --auth-choice openacosmi-zen
# 或非交互式
openacosmi onboard --openacosmi-zen-api-key "$OPENACOSMI_API_KEY"
```

## 配置片段

```json5
{
  env: { OPENACOSMI_API_KEY: "sk-..." },
  agents: { defaults: { model: { primary: "openacosmi/claude-opus-4-6" } } },
}
```

## 注意

- `OPENACOSMI_ZEN_API_KEY` 同样支持
- 登录 创宇太虚（Claw Acosmi） 仪表盘，填写账单信息后复制 API Key
- 按请求计费，详见 创宇太虚（Claw Acosmi） 仪表盘
