# Desktop Manual Regression Checklist

状态说明：

- 本清单为 `P4` 的手工验收模板
- 当前仅归档，不自动执行
- 所有步骤都以“不改现有运行代码”为前提

## 1. 附着已有健康 Gateway

目标：

- 桌面壳发现固定端口上的健康 Gateway
- 不重复拉起新的同端口实例
- 退出桌面壳后，不关闭附着的现有 Gateway

建议记录：

- 固定端口值
- 现有 Gateway PID
- 桌面壳启动日志
- 桌面壳退出后 Gateway 是否仍在

## 2. 非 Gateway 端口占用

目标：

- 固定端口被其它进程占用时，桌面壳应明确失败
- 错误提示应能区分“已有健康 Gateway”与“非 Gateway 冲突”

建议记录：

- 占端口进程类型
- 桌面壳报错文案
- 是否错误进入附着状态

## 3. 深链刷新

目标：

- `/ui/chat`
- `/ui/overview`
- `/ui/overview/settings`

都应在刷新后正常回退到 `index.html`，而不是 404。

补充检查：

- 带扩展名但不存在的资源仍返回 404
- 已存在资源如 `/ui/assets/app.js` 正常返回静态文件

## 4. 首次启动与已配置启动分流

目标：

- 配置缺失时，初始 URL 带 `?onboarding=true`
- 配置存在时，初始 URL 不带 onboarding 参数
- 托盘“重新配置向导”可强制进入 onboarding 流程

建议记录：

- 配置文件存在性
- 初始 URL
- 托盘点击后的 URL 变化

## 5. 托盘退出与 runtime 回收

目标：

- 主窗口关闭时应隐藏，不直接退出
- 托盘显式退出时应触发正常关闭
- 若桌面壳拥有 runtime，则应关闭该 runtime
- 若桌面壳仅附着已有 Gateway，则不应关闭对方 runtime

建议记录：

- attachedExisting 标志
- 关闭窗口后的进程状态
- 托盘退出后的 Gateway 存活状态

## 6. Control UI 资源缺失

目标：

- 在没有 `ControlUIDir`、没有嵌入资源、没有磁盘候选目录时
- 桌面壳应直接失败，不启动一个打不开页面的 Gateway

建议记录：

- 资源来源判定结果
- 最终错误文案

## 7. /ui/ 探测失败后的回收

目标：

- 新启动 Gateway 后若 `/ui/` 探测失败
- 桌面壳应回收 runtime 并返回错误

建议记录：

- 探测失败原因
- runtime 是否已关闭
- 是否留下悬挂进程
