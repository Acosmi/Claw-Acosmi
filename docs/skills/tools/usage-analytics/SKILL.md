---
name: usage-analytics
description: "用量与成本分析：usage.* 与 sessions.usage* 的核对流程"
---

# 用量与成本分析

用于会话粒度、时间序列、成本聚合的核算与排障。

## 覆盖方法
- `sessions.usage`
- `sessions.usage.timeseries`
- `sessions.usage.logs`
- `usage.status`
- `usage.cost`

## 推荐流程
1. 用 `usage.status` 确认统计子系统状态。
2. 用 `sessions.usage` 抓会话维度总览（tokens/cost/messages）。
3. 用 `sessions.usage.timeseries` 分析波峰波谷。
4. 用 `sessions.usage.logs` 回溯异常账单窗口。
5. 用 `usage.cost` 做 provider/model 聚合核对。

## 风险控制
- 成本分析必须注明时间范围与口径。
- 不混用不同模型价格表进行横向对比。

## 成功判定
- 会话统计、时间序列、成本聚合三者口径一致。
