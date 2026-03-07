---
name: argus-screen-reading
description: Argus 读屏：先 read_text，再 describe_scene；capture_screen 只在状态不明时使用
tools: argus_read_text, argus_describe_scene, argus_capture_screen
disable-model-invocation: true
---

# Argus 读屏

适用于“先找文字、确认状态、补一眼上下文”的场景。

## 优先顺序

1. 目标是文本时，先用 `argus_read_text`
2. 只在不知道屏幕大致状态时，再用 `argus_describe_scene`
3. `argus_capture_screen` 只在需要保留上下文或状态仍不清楚时使用

## 避免

- 不要把整屏截图当成每一步的固定前置
- 不要为了读一行字先做全局场景理解

## 故障树

- `argus_read_text` 没读到目标文本：通常是区域太偏、文字过小，先缩小目标区域或先定位再读
- `argus_describe_scene` 很泛：说明问题其实是“找具体文字”，应回到 `argus_read_text`
- `argus_capture_screen` 返回不了有效结果：优先排查屏幕录制权限或 Argus bridge 状态
- 一直靠整屏截图补信息：说明 `task_brief` 缺少目标文本、窗口名或成功标志

## 回滚步骤

- 停止连续整屏截图，先改写成更具体的目标文本或区域线索
- 如果文本位置不清楚，先用 `argus-target-locate` 找字段、标题或弹窗，再回到读屏
- 只有在状态仍不明确时，补一次截图作为上下文
- 仍无法确认时，先回报“读不到什么”，不要继续堆叠截图

## 验收清单

- 目标文本或状态已被直接读出，而不是只得到模糊场景描述
- 如使用截图，其作用只是补上下文，不是整个流程的唯一信息源
- 结果足以支撑下一步点击、输入或汇报
- 没有形成“截图 -> 描述 -> 再截图”的循环
