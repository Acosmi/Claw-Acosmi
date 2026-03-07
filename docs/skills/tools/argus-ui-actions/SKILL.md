---
name: argus-ui-actions
description: Argus UI 操作：点击、输入、按键后做最小验证，避免盲点连续操作
tools: argus_click, argus_double_click, argus_type_text, argus_press_key, argus_scroll, argus_mouse_position
disable-model-invocation: true
---

# Argus UI 操作

适用于已知道目标位置后的执行动作。

## 要点

- 先定位，再点击或输入
- 输入前说明目标字段，输入后只做必要验证
- 连续按键前写清组合键和预期结果

## 避免

- 没确认焦点就直接输入
- 状态未确认时连续多次点击
- 用滚动替代本可直接定位的目标

## 故障树

- 点击或输入后没反应：通常是焦点不在目标控件，或上一轮定位还不够准
- 连续点击后状态更乱：说明缺少动作后的最小验证，应立即停手
- 输入进错字段：说明没有先确认焦点或字段标签
- 动作被拦住：点击、键入、快捷键属于中风险动作，默认审批模式下可能需要确认

## 回滚步骤

- 先停止重复点击或输入，避免把一次误操作放大
- 用 `argus-target-locate` 或 `argus_mouse_position` 重新确认目标和焦点
- 如果误输文本且任务允许编辑，优先用应用原生撤销/退格修正，再立即验证
- 状态仍不清时，转到 `argus-verify-retry` 或 `argus-screen-reading` 做一次最小复查

## 验收清单

- 动作只执行了必要次数，没有重复提交或重复点击
- 输入内容进入了正确字段，或点击命中了正确控件
- 动作后已做一次最小验证，而不是盲目继续
- 若发生修正，修正动作本身也已被验证
