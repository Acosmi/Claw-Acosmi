---
name: argus-target-locate
description: Argus 目标定位：先按文案或相邻文本做 locate_element / detect_dialog，再点击
tools: argus_locate_element, argus_detect_dialog
disable-model-invocation: true
---

# Argus 目标定位

适用于“找按钮、找输入框、找弹窗”的场景。

## 要点

- 先提供按钮文案、相邻文本、区域线索
- 弹窗或确认框优先 `argus_detect_dialog`
- 普通控件优先 `argus_locate_element`

## 避免

- 不要先盲点再回头验证
- 不要只给“找那个按钮”这种过于模糊的描述

## 故障树

- `argus_locate_element` 找不到坐标：通常是按钮文案、相邻文本或区域线索不够具体
- `argus_detect_dialog` 未识别到弹窗：先确认当前步骤是否真的触发了弹窗，再补读屏
- 找到多个相似目标：说明缺少父区域、左/右位置或相邻字段信息
- 还没定位清楚就进入点击：会把定位问题误判成操作问题

## 回滚步骤

- 暂停点击，先把当前目标重新表述成“文案 + 相邻文本 + 区域”
- 如果是弹窗场景，先确认触发动作已经发生，再做一次 `argus_detect_dialog`
- 如果目标主要靠文字识别，先回到 `argus-screen-reading` 补 OCR 结果
- 只有定位结果足够唯一时，再进入 `argus-ui-actions`

## 验收清单

- 已得到唯一或低歧义的目标位置/弹窗结果
- 目标的相邻文本、区域或窗口上下文已说清
- 下一步点击或输入不再依赖“猜大概位置”
- 没有在未定位完成时提前执行动作
