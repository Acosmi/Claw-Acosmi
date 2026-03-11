---
name: SOP 工作流
description: 文档驱动的任务全生命周期管理，从规划到执行到审计到归档的完整可追溯链路
tools: []
metadata:
  emoji: "📋"
  category: claude
  requires: []
user-invocable: false
disable-model-invocation: true
---

# 技能 2: SOP 工作流

> 每个任务必须可追溯：从立项、执行、审计到归档的完整链路。

## 适用场景

未文档化的决策在跨 session 时会蒸发。SOP 工作流确保连续性，防止重复工作，
并为所有变更创建可审计的轨迹。

## 目录结构

```
docs/claude/
  renwu/        # 任务追踪（活跃任务分解与进度）
  tracking/     # 专项追踪（长期追踪文档）
  goujia/       # 架构文档（shenji-* 审计报告、arch-* 架构说明）
  guidang/      # 已归档（只读，已审计通过的完结项）
  deferred/     # 延迟项（TODO、阻塞项、技术债、待处理项）
  audit/        # 审计报告（技能 3 输出）
  archive/      # 历史归档
```

## 规则

### 2.1 任务生命周期

```
 [规划] ──► [追踪] ──► [执行] ──► [审计] ──► [归档]
   │           │          │          │          │
   │      创建追踪     更新进度    生成审计    移动到
   │      文档 [ ]     勾选 [x]   报告       guidang/
   │
   └─ 确认任务范围与依赖
```

| 阶段 | 动作 | 输出位置 |
|------|------|---------|
| 规划 | 分解任务为细粒度 checkbox 条目 | `renwu/` 或 `tracking/` |
| 追踪 | 每步完成后勾选 `[x]` | 原地更新 |
| 延迟 | 发现但不在当前范围的阻塞项/TODO | `deferred/` |
| 审计 | 逐行代码审查（触发技能 3） | `audit/` |
| 归档 | 审计通过后移至归档 | `guidang/` |

### 2.2 延迟项策略

执行中遇到以下任何情况，**立即**在 `deferred/` 创建或追加：

- 发现但不在当前范围的 bug 或边界情况
- 代码中留下的 TODO / FIXME
- 设计顾虑或技术债观察
- 依赖升级或 API 废弃通知
- 需要未来测试的平台特定行为

**绝不**只把延迟项留在对话上下文中 — 它们会丢失。

### 2.3 归档门控（与技能 3 联锁）

**任何条目未经审计报告不得移至归档。**

归档检查清单：
- [ ] 追踪文档所有 checkbox 已勾选 `[x]`
- [ ] `audit/` 中存在对应审计报告且覆盖所有代码变更
- [ ] 审计报告 verdict 为 Pass 或 Pass with Notes
- [ ] 所有 FAIL 级发现已修复并复审
- [ ] 原追踪文档标注审计报告路径

违反此门控是**硬停止** — 继续操作前先向用户确认。

### 2.4 命名规范

| 文档类型 | 命名格式 | 示例 |
|----------|---------|------|
| 审计报告 | `audit-YYYY-MM-DD-<组件>-<简述>.md` | `audit-2026-03-08-models-dual-track.md` |
| 追踪文档 | `tracking-<功能名>-YYYY-MM-DD.md` | `tracking-browser-automation-retrofit-2026-03-07.md` |
| 架构文档 | `arch-<主题>.md` | `arch-system-prompt-design.md` |
| 审计速记 | `shenji-NNN-<简述>.md` | `shenji-021-p3-clearance-sprint6-audit.md` |
