---
name: argus-app-launch
description: Argus 应用入口：优先用 macOS shortcut、Spotlight 和已知快捷键切应用
tools: argus_macos_shortcut, argus_hotkey
disable-model-invocation: true
---

# Argus 应用入口

适用于“先打开应用、切到窗口、触发现成快捷入口”这类动作。

## 优先顺序

1. 已知快捷指令时，先用 `argus_macos_shortcut`
2. 已知键盘入口时，先用 `argus_hotkey`
3. 只有入口不清楚时，再进入截图或元素定位

## 要点

- 在 `task_brief` 里写清应用名、Spotlight 关键词或已有快捷键
- 不要为了“打开某个应用”先整屏截图
- 切换成功后，再进入读屏或定位步骤

## 故障树

- `argus_macos_shortcut` / `argus_hotkey` 无效果：先检查快捷指令名、组合键或目标应用名是否写对
- 动作执行了但切错应用：通常是入口描述过泛，先把应用名、窗口名或 Spotlight 关键词写得更具体
- 调用被拦住：这两个工具属于中风险动作，Web 频道在默认审批模式下可能需要确认
- 直接报 Argus 不可用或工具错误：先排查 Argus bridge、屏幕录制/辅助功能权限，而不是继续重复按键

## 回滚步骤

- 停止继续发送快捷键，避免重复切换窗口
- 回到最近一个已知安全窗口，再补充更明确的应用入口信息
- 如果入口仍不确定，转回 `argus-screen-reading` 或 `argus-target-locate` 获取当前上下文
- 只有确认入口信息修正后，再重试一次打开或切换

## 验收清单

- 目标应用或窗口已进入前台
- 没有误打开无关应用、URL 或系统设置页
- 下一步所需的文本、按钮或区域已经可见
- 整个入口阶段没有退化成“先截图再猜应用”
