---
name: security-governance
description: "安全与审批治理：security/exec approvals/escalation/rules/remote approval 全链路"
---

# 安全与审批治理

用于安全级别、提权审批、命令规则、远程审批、任务预设的统一治理。

## 主路径
- 默认先治理 `gateway/mac companion` 的审批与提权主链路。
- `node host` 只在命令必须落到远端机器时进入；不要把它当默认主路径。
- 当前 `exec.approvals.node.get/set` 仍有网关侧 stub 边界，node 审批相关项不应作为主验收依赖。

## 覆盖方法
- `security.get`
- `exec.approvals.get`
- `exec.approvals.set`
- `security.escalation.request`
- `security.escalation.resolve`
- `security.escalation.status`
- `security.escalation.audit`
- `security.escalation.revoke`
- `security.rules.list`
- `security.rules.add`
- `security.rules.remove`
- `security.rules.test`
- `security.remoteApproval.config.get`
- `security.remoteApproval.config.set`
- `security.remoteApproval.test`
- `security.taskPresets.list`
- `security.taskPresets.add`
- `security.taskPresets.update`
- `security.taskPresets.remove`
- `security.taskPresets.match`

## 推荐流程
1. 先用 `security.get` 与 `exec.approvals.get` 确认 gateway 主链路基线。
2. 规则治理先 `security.rules.test`，再 `add/remove`。
3. 临时提权走 `security.escalation.request`，完成后立即 `resolve` 或 `revoke`。
4. 远程审批先 `config.get/set`，再 `security.remoteApproval.test` 做连通验证。
5. 任务授权模板用 `security.taskPresets.*` 管理，避免重复人工审批。
6. 只有在确实需要远端执行时，才检查 node 侧审批能力是否已真实接通。

## 风险控制
- 提权默认走临时 TTL，不直接长期放开。
- 规则变更前后必须做 `security.rules.test` 对比。
- 远程审批通道变更后立即做测试发送。
- 不要把 node 兼容路径写成主制度路径。

## 故障树

- 提权请求能发起但制度效果不对：先区分是 `security.escalation.*` 的 TTL/状态问题，还是 exec 本地审批策略问题
- `security.rules.add/remove` 后结果反常：通常是变更前没做 `security.rules.test`，导致命中范围判断错了
- `security.remoteApproval.test` 成功但真实审批流程仍不顺：测试发送只证明远程审批通道可达，不等于完整决策链都闭环
- node 相关配置看起来齐全但执行仍不符合预期：当前主制度路径不是 node，node 项不能当主验收前提
- 任务预设匹配异常：要先检查 preset 匹配条件，而不是直接放大到全局提权

## 回滚步骤

- 制度变更后出现异常时，先撤回最近的 escalation、rules 或 remote approval 配置变更
- 规则问题先回滚到变更前状态，再用 `security.rules.test` 做对比复盘
- 远程审批异常时，先保留本地审批主链路可用，再单独修远程通道
- node 相关问题先从主制度验收中剥离，避免兼容路径拖垮主链路排障
- 对高风险开放项，优先回落到更短 TTL 或更窄范围的预设

## 验收清单

- 审批事件可触发、可决策、可审计回放
- 提权 TTL、规则命中、远程审批测试、任务预设匹配都能被单独验证
- 远程审批测试结果与真实制度边界被正确区分
- node 兼容路径没有被误写成主验收依赖
- 文档主路径与当前系统实际主链路一致
