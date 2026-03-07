---
name: config-governance
description: "配置治理：安全使用 config.get/schema/patch/set/apply 完成变更"
---

# 配置治理

用于网关配置的低风险变更流程，避免一次性全量覆盖导致服务异常。

## 覆盖方法
- `config.get`
- `config.schema`
- `config.patch`
- `config.set`
- `config.apply`

## 推荐流程
1. 先调用 `config.get` 获取当前生效配置，记录关键字段与哈希。
2. 调用 `config.schema` 确认目标字段类型与可选值。
3. 小步修改优先使用 `config.patch`，一次只改一个主题块。
4. 仅在需要替换整份配置时使用 `config.set`。
5. 需要立即生效时调用 `config.apply`，并核对返回中的重启/应用结果。

## 变更守则
- 避免一次改动多个高风险区块（鉴权、模型、安全策略）。
- 对敏感项使用最小差异修改，不做无关格式化。
- 失败时优先回滚最近一次 patch，不盲目重复 apply。

## 成功判定
- `config.get` 返回值与目标配置一致。
- 相关模块状态接口（如 `security.get`、`models.list`）读数正常。
