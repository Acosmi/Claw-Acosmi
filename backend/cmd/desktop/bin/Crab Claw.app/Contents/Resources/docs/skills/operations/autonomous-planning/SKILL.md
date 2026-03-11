---
name: autonomous-planning
description: "自主规划工作流：意图分析→能力树查询→动作序列→审批→执行→验证的完整闭环"
---

# 自主规划技能

## 规划链路

```
用户输入 → classifyIntent(tier) → analyzeIntent(IR) → GeneratePlanSteps → 审批 → 执行
```

## 意图层级（6 级）

| Tier | 说明 | 典型场景 |
|------|------|---------|
| `greeting` | 问候 | "你好"、"hi" |
| `question` | 问答 | "这是什么？"、"帮我查一下" |
| `task_light` | 轻量任务 | 读文件、搜索、发消息 |
| `task_write` | 写入任务 | 编写代码、创建文件、发邮件 |
| `task_delete` | 删除任务 | 删除文件、清理目录 |
| `task_multimodal` | 多模态任务 | 浏览器操作、桌面自动化、复合流程 |

## 规划原则

1. **能力树驱动**：从 `DefaultTree()` 查询可用工具，不硬编码工具名
2. **权限前置**：在规划阶段就检查 `ApprovalType`，提前告知用户需要什么审批
3. **最小权限**：选择满足任务的最低 tier 工具，不过度提权
4. **失败回退**：规划多候选路径，首选失败时自动尝试备选

## IntentAnalysis IR 结构

```
IntentAnalysis {
  Tier:            意图层级
  RequiredActions: 抽象动作序列 (send_file, find_file, write_code, browse_web...)
  Targets:         涉及资源 (file/url/channel)
  RiskHints:       风险提示 (workspace外路径, 需提权...)
}
```

## 多步任务规划

1. 分解为原子动作序列
2. 每个动作映射到能力树工具节点
3. 检查每个工具的 `MinTier`、`ApprovalType`、`EscalationHints`
4. 生成 `PlanSteps` 供用户确认
5. 审批通过后按序执行
6. 每步验证结果，失败时回退或报告

## 委托决策

| 条件 | 决策 |
|------|------|
| 多文件代码任务 | → `spawn_coder_agent` |
| 桌面/视觉任务 | → `spawn_argus_agent` |
| 媒体运营任务 | → `spawn_media_agent` |
| 简单单步操作 | → 直接调用工具 |
