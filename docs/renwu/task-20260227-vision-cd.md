# 视觉理解方案 C+D 任务跟踪

> 创建日期：2026-02-27
> 状态：待启动
> 来源：[视觉理解新方案](../claude/视觉理解新方案.md)

## 背景与目标

实现方案 D（审批机制重构）+ 方案 C（VLA 持续输出）+ 前端子智能体管理菜单，
使视觉读操作跳过审批、主 Agent 从 Buffer 直接读取视觉状态、前端可选择模式。

## 任务清单

### Batch A: 方案 D — 审批机制重构

| 状态 | 编号 | 任务 | 关联文件 |
|------|------|------|----------|
| ⬜ | D1 | ActionRiskLevel 定义 | `runner/action_risk.go` (新) |
| ⬜ | D2 | tool_executor 接入风险分级 | `runner/tool_executor.go` |
| ⬜ | D3 | SubAgentConfig 配置类型 | `types/types_openacosmi.go` |
| ⬜ | D4 | 风险分级单元测试 | `runner/action_risk_test.go` (新) |

### Batch B: 方案 C — ScreenObserver

| 状态 | 编号 | 任务 | 关联文件 |
|------|------|------|----------|
| ⬜ | C1 | VisionObservation 数据结构 | `argus/observation.go` (新) |
| ⬜ | C2 | ObservationBuffer ring buffer | `argus/observation_buffer.go` (新) |
| ⬜ | C3 | VLAClient 接口 | `argus/vla_client.go` (新) |
| ⬜ | C4 | ScreenObserver goroutine | `argus/screen_observer.go` (新) |
| ⬜ | C5 | Bridge 集成 Observer | `argus/bridge.go` |
| ⬜ | C6 | 主 Agent 从 Buffer 读取 | `runner/attempt_runner.go` |
| ⬜ | C7 | Buffer 单元测试 | `argus/observation_buffer_test.go` (新) |

### Batch C: 前端子智能体菜单

| 状态 | 编号 | 任务 | 关联文件 |
|------|------|------|----------|
| ⬜ | F1 | navigation Tab 新增 | `ui/navigation.ts` |
| ⬜ | F2 | subagents 视图 | `ui/views/subagents.ts` (新) |
| ⬜ | F3 | i18n 键值 | `ui/locales/zh.ts`, `en.ts` |
| ⬜ | F4 | app-settings 接入 | `ui/app-settings.ts` |

> **状态标记**：⬜ 待做 → 🔄 进行中 → ✅ 完成 → ⏭️ 跳过

## 风险与注意事项

- AnthropicVisionClient 需要 API Key，无 Key 时 fallback 到"仅截图"模式
- ScreenObserver 的 ring buffer 约 500MB 内存（500帧×1080p PNG），需确认环境可用
- 方案 B（WebSocket 推送）本次不实施，列为未来方向

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-02-27 | 初始创建 |
