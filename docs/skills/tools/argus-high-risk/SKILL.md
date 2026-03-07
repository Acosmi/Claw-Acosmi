---
name: argus-high-risk
description: Argus 高风险边界：open_url 和 run_shell 仅在任务明确需要且审批允许时使用
tools: argus_open_url, argus_run_shell
disable-model-invocation: true
---

# Argus 高风险边界

适用于视觉流程里极少数需要跳出 UI 的场景。

## 要点

- `argus_open_url` 只在任务明确需要打开链接时使用
- `argus_run_shell` 只在视觉流程确实需要 shell 辅助时使用
- 两者都应服从当前审批与权限边界

## 避免

- 用 `open_url` 代替普通应用内导航
- 用 `run_shell` 绕过本可直接完成的视觉操作

## 故障树

- `argus_open_url` 被阻断：可能是委托合约禁止网络，或当前审批门未放行
- `argus_run_shell` 被拒绝：通常是高风险审批未通过，或当前频道没有可用审批门
- 工具直接报错：先排查 Argus bridge 和审批状态，不要立刻重复执行
- 本来是普通 UI 导航却走到高风险工具：说明技能选型错了，应回到低风险视觉路径

## 回滚步骤

- 一旦被拒绝或阻断，先终止高风险路径，不做静默重试
- 能用应用内导航、`browser` 或普通 Argus UI 动作完成时，优先回退到这些路径
- 只有任务明确要求且审批条件满足时，才重新发起一次高风险动作
- 重试前写清目标 URL、命令目的和预期副作用

## 验收清单

- 高风险工具的使用理由是明确且必要的
- 所需审批或权限已经实际满足
- `open_url` / `run_shell` 只执行了预期的一次动作
- 结果或副作用已经被读回并交给主流程消费
