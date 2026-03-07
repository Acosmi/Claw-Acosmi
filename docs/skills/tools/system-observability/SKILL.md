---
name: system-observability
description: "系统可观测与恢复：health/status/logs/system.reset 运行手册"
---

# 系统可观测与恢复

用于网关运行状态检查、日志定位、备份恢复与系统重置。

## 覆盖方法
- `health`
- `status`
- `logs.tail`
- `system.backup.list`
- `system.backup.restore`
- `system.reset.preview`
- `system.reset`

## 推荐流程
1. 用 `health`、`status` 建立当前健康基线。
2. 出现异常时用 `logs.tail` 定位错误窗口与上下文。
3. 执行恢复前先 `system.reset.preview` 评估影响面。
4. 可回滚场景优先 `system.backup.restore`。
5. 仅在确认需要时执行 `system.reset`。

## 风险控制
- `system.reset` 前必须保留备份清单与操作记录。
- 不在未知根因情况下反复重置。

## 成功判定
- 健康检查恢复正常。
- 日志中关键错误不再持续出现。

## 故障树

- 把 `health` / `status` 当成完整根因分析：当前它们只给粗粒度基线，不会自动解释具体任务为何卡住或哪个工具步骤失败。
- `logs.tail` 看不到关键报错：可能不是“没报错”，而是日志轮转、cursor 失效或窗口过窄；要看返回里的 `reset` / `truncated`。
- 期望中途进度能从系统观测里直接同步到远程聊天渠道：当前长任务进度更偏 WebSocket/UI 的 `agent` / `task.*` 事件，不应假设远程频道稳定看到中途状态。
- 把 `system.backup.restore` 和 `system.reset` 当成同一件事：前者是配置回滚，后者当前 level 1 只重置部分运行时文件与内存状态，不是全系统清空。
- 把 `system.reset` 当“万能修复”：当前 level 1 只删除 `exec-approvals.json`、截断 `escalation-audit.log`、删除 UHMS `boot.json` 并重置提权内存态，不能替代真正排障。

## 回滚步骤

1. 先用 `health`、`status` 建基线，再用 `logs.tail` 拉当前错误窗口；不要一上来就 `system.reset`。
2. 如果问题是某个长任务或异步运行，先关联 `runId`、任务会话和 `agent` / `task.*` 事件，再判断是执行问题还是系统问题。
3. 配置类问题优先 `system.backup.restore`；运行时重置前必须先跑 `system.reset.preview`，确认影响范围。
4. `logs.tail` 若出现 `reset` / `truncated`，先重建日志窗口再继续排障，不要把“窗口断了”误判成“问题消失了”。

## 验收清单

- 能区分“系统健康基线异常”“任务执行异常”“配置异常”三类问题，而不是全都归到一个入口。
- `logs.tail` 的 cursor、窗口截断和轮转行为已被正确理解，排障结论不是建立在残缺日志上。
- 执行 `system.backup.restore` 或 `system.reset` 前后，影响范围与 preview/设计边界一致，没有把局部重置误当成全量恢复。
- 修复后 `health`、`status`、关键错误日志和任务事件链路相互一致，不再出现持续性异常信号。
