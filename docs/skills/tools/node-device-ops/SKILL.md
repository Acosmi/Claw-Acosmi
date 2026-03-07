---
name: node-device-ops
description: "节点与设备治理：node.* 与 device.* 配对、调用、令牌管理"
---

# Node 与 Device 运维

用于节点配对、节点调用、设备令牌轮换和吊销。

## 当前边界

- 节点连通、配对成功，不等于 Gateway 已能远程治理该节点的 exec approvals。
- 当前 Go Gateway 不负责 `exec.approvals.node.get/set` 的远程管理代理。
- 因此 node 运维与 node 审批文件治理要分开看：前者可验证配对/调用，后者当前更偏 node 主机本地维护。

## 覆盖方法
- `node.pair.request`
- `node.pair.list`
- `node.pair.approve`
- `node.pair.reject`
- `node.pair.verify`
- `node.list`
- `node.describe`
- `node.invoke`
- `node.invoke.result`
- `node.event`
- `node.rename`
- `device.pair.list`
- `device.pair.approve`
- `device.pair.reject`
- `device.token.rotate`
- `device.token.revoke`

## 推荐流程
1. 用 `node.pair.list` / `device.pair.list` 检查待审批项。
2. 对可信来源执行 `approve`，未知来源执行 `reject`。
3. 用 `node.list` 与 `node.describe` 验证连通性和能力集。
4. 用 `node.invoke` 执行最小探测命令并通过 `node.invoke.result` 收敛结果。
5. 定期轮换 `device.token.rotate`，发现泄漏时立刻 `revoke`。

## 风险控制
- `node.invoke` 仅调用在能力白名单内的命令。
- 令牌只做短时展示，避免落盘明文外泄。
- 不要把“节点在线”误判成“node 审批代理已闭环”。

## 成功判定
- 节点 `connected=true` 且能力字段完整。
- 令牌轮换后旧令牌不可再访问。

## 故障树

- 配对通过但 node 仍不可调用：节点未连接、状态不是 `connected=true`，或未上报可执行的能力集。
- `node.invoke` 失败：目标命令不在允许列表、目标 node 不支持该命令，或结果链路未通过 `node.invoke.result` 收回。
- 令牌流程异常：轮换后仍沿用旧 token，或怀疑泄漏后只 rotate 未 revoke，导致旧凭据仍被误保留。
- 误把 node 运维当成 node 审批治理：节点在线、可调用，不等于 Gateway 能远程管理该节点的 exec approvals。

## 回滚步骤

1. 先用 `node.list` / `node.describe` 验证目标 node 的连通性与能力，再决定是否继续 `node.invoke`。
2. `node.invoke` 先发最小探测命令，再切换到真实任务，避免把“命令不支持”和“业务失败”混在一起。
3. 遇到来源不明的 pair request 直接拒绝；发现 token 疑似泄漏时先 `device.token.revoke`，再重新申请或轮换。
4. 涉及 `system.run` 的审批问题，直接转到 node 主机本地维护；不要反复在 Gateway 侧寻找 `exec.approvals.node.*` 代理入口。

## 验收清单

- 配对请求已按可信度完成 approve/reject，node/device 列表状态与预期一致。
- 目标 node `connected=true` 且具备预期能力；最小 `node.invoke` 能拿到明确的 `node.invoke.result`。
- 失败时能区分连接问题、能力缺失、命令受限和命令执行失败，不把它们都归因为“节点坏了”。
- 令牌轮换后旧 token 已失效，团队操作手册不再假设 Gateway 可以远程维护 node 审批文件。
