---
document_type: Audit
status: Completed
created: 2026-02-27
last_updated: 2026-02-27
tracking_doc: docs/claude/tracking/impl-plan-global-boot-skill-vfs-2026-02-27.md
verdict: CONDITIONAL PASS — 主链路可用，存在死代码与文档虚标问题
---

# 审计: Global Boot + VFS 分级技能加载 — 复核审计

**审计员**: Claude Code (Sonnet 4.6)
**日期**: 2026-02-27
**本次为第二轮审计**（第一轮标记了 PASS，本轮发现第一轮结论不准确）

---

## 审计范围

本轮对所有修改文件进行逐行交叉核对，并追踪 **每段代码实际是否被调用**。

---

## 1. 实际连通性核查（最重要）

### 主链路: skills.distribute 触发路径

```
前端按钮 → skills.distribute RPC
→ handleSkillsDistribute (server_methods_skills.go:418)
→ skills.DistributeSkills(ctx, vfs, vectorIndex, entries)   ← 直接调用老函数
→ distributeOneSkill → vfs.WriteSystemEntry + vectorIndex.Upsert(zeroVec)
→ UpdateBootSkillsInfo(path, info)                          ← 直接写 boot.json
```

**Boot 模式触发路径**:
```
agent 启动 → server.go:909 skillVFSBridgeAdapter{mgr}
→ SkillVFSBridge.IsReady()
→ mgr.SystemDistributionStatus("sys_skills")   ← 我们新增的方法 ✅
→ return status.Indexed && status.TotalEntries > 0
```

**search_skills 工具路径**:
```
LLM 调用 search_skills → executeSearchSkills
→ SkillVFSBridge.SearchSkills(ctx, query, topK)
→ mgr.SearchSystem(ctx, "sys_skills", query, topK)  ← 我们新增的方法 ✅
→ SearchSystemEntries(Qdrant) OR searchSystemVFSFallback
```

### 结论: 新增的核心方法（SearchSystem、ReadSystemL0/L1/L2、SystemDistributionStatus）全部被有效调用。主链路连通。

---

## 2. 死代码

### D1 — SkillDistributor.Distribute() 从未被调用 [HIGH]

**位置**: `agents/skills/skill_distributor.go:49-88`

`handleSkillsDistribute` 完全绕过 `SkillDistributor.Distribute()`，直接调用 `skills.DistributeSkills(ctx, vfs, vectorIndex, entries)`（老函数）。

- 我们新写的 `SkillDistributor` 结构体从未被实例化
- `SkillDistributor.Distribute` 内部的"正确 IndexSystemEntry 路径"和"BootManager 注入"从未执行
- 实际 Boot 文件更新走的是 `UpdateBootSkillsInfo(bootPath, bootInfo)` (server_methods_skills.go:483)，不是 `bootManager.MarkSkillsIndexed`

**影响**:
- `SkillDistributor` 中修复的"正确零向量 IndexSystemEntry"路径实际上没有生效
- 实际仍用 `vectorIndex.Upsert(ctx, "sys_skills", id, make([]float32, 0), payload)`（零长度切片）
- 若 Qdrant 拒绝 dim=0，Qdrant 索引静默失败（但 VFS 写入成功，Boot 模式不受影响）
- `SkillDistributeResult.Updated` 字段永远为 0（无论何时）

**建议**: 删除 `SkillDistributor` 结构体，或让 `handleSkillsDistribute` 改用它（选其一，不能都存在）。

### D2 — relativeSystemPath() 从未被调用 [LOW]

**位置**: `vfs.go:568-570`

定义了 `relativeSystemPath(namespace, category, id string)` 但在任何调用处都未使用（包括 SkillDistributor，实际上也没有 SkillDistributor）。

---

## 3. 代码正确性

### F5 — context.Canceled 不应触发 VFS fallback [FIXED]

已在本次审计第一轮修复。`SearchSystem` 现在在 fallback 前检查 `ctx.Err()`。✅

### F9 — Qdrant upsert 用 make([]float32, 0)（零长度切片）[LOW]

**位置**: `skill_distributor.go:179` （但此代码因 D1 不被调用）
**实际位置**: `distributeOneSkill` 被 `DistributeSkills` 调用，`vectorIndex.Upsert(ctx, "sys_skills", id, make([]float32, 0), payload)`

如果 Qdrant Segment 要求维度 > 0 才能创建 point，此处会静默失败（已有 `non-fatal` 处理）。Boot 模式基于 VFS，不依赖 Qdrant，所以功能不受阻断。

---

## 4. 文档虚标问题

### DOC1 — skill5_verified: true 是虚假的 [HIGH]

跟踪文档头部设置了 `skill5_verified: true`，但文档末尾的 Skill 5 在线验证区域（第 1133-1143 行）所有条目全部是"待查询/待填写"。

Skill 5 验证从未执行过。这一字段本次修改（第二轮审计）前被错误地设为 `true`。

### DOC2 — P0.2/P0.5/P1.1/P1.2/P1.3/P1.5/P1.6/P1.7 body checkboxes 全部未更新 [MEDIUM]

跟踪文档摘要表（顶部）标记了 ✅，但各章节 body 内的 checkbox 列表全是 `[ ]`（空）。

具体对比:

| 章节 | 摘要表状态 | Body checkbox 状态 |
|------|-----------|-------------------|
| P0.2 BootManager | ✅ 已完成（预有） | 全部 `[ ]` |
| P0.5 VectorAdapter | ✅ 已完成（预有） | 全部 `[ ]` |
| P1.1 SkillDistributor | ✅ 已完成（预有+扩展） | 全部 `[ ]` |
| P1.2 RPC | ✅ 已完成（预有） | 全部 `[ ]` |
| P1.3 attempt_runner | ✅ 已完成（预有） | 全部 `[ ]` |
| P1.5-P1.7 前端 | ✅ 已完成（预有） | 全部 `[ ]` |

### DOC3 — 审计报告第一轮结论不准确 [MEDIUM]

第一轮审计报告标记了 `verdict: PASS`，但没有检查 SkillDistributor 是否被实际调用，也没有发现 skill5_verified 虚标问题。本报告为修正版本。

---

## 5. 测试覆盖

| 测试项 | 状态 |
|--------|------|
| P0.2 BootManager 单元测试 (`boot_test.go`) | ✅ 预有 |
| P0.1 VFS system 测试 (`vfs_system_test.go`) | ✅ 预有 |
| P0.4 SearchSystem / SystemDistributionStatus | ❌ 缺失 |
| SkillDistributor.Distribute | ❌ 缺失（且为死代码） |
| skills.distribute RPC 集成测试 | ❌ 缺失 |

---

## 6. 真实完成度评估

| 目标 | 状态 | 说明 |
|------|------|------|
| VFS `_system/` 命名空间 | ✅ 100% | 所有方法存在且被调用 |
| BootManager | ✅ 100% | 预有，tests 存在 |
| Config 扩展 | ✅ 100% | 预有 |
| Manager 检索接口 (SearchSystem 等) | ✅ 100% | 已实现并被 skillVFSBridgeAdapter 调用 |
| skills.distribute RPC | ✅ 90% | 功能可用，但绕过 SkillDistributor（Qdrant 索引路径有 dim=0 风险） |
| skills.distribution.status RPC | ✅ 100% | 已实现 |
| Boot 模式 agent 运行时 | ✅ 100% | 预有 + 我们的新方法正确连入 |
| 前端 P1.5/P1.6/P1.7 | ✅ 100% | 预有 |
| SkillDistributor.Distribute 实现 | ❌ 死代码，0% 有效 |
| 单元测试（新增部分） | ❌ 0% |
| Skill 5 在线验证 | ❌ 0% |
| 跟踪文档 checkbox 准确性 | ❌ ~30%（仅 P0.3/P0.4 准确） |

**整体 Phase 1 功能完成度: ~75%**（主链路可用，SkillDistributor 和测试缺失）

---

## 7. 行动项

| 优先级 | 行动 | 类型 |
|--------|------|------|
| P0 | 更新跟踪文档：修正所有 body checkbox，回写 `skill5_verified: false` | 文档 |
| P1 | 决策：删除 `SkillDistributor.Distribute` 死代码 OR 让 RPC 改用它 | 代码 |
| P2 | 补充 SearchSystem/SystemDistributionStatus 单元测试 | 测试 |
| P3 | 执行 Skill 5 验证（Qdrant payload-only 支持性确认） | 验证 |

---

## 总体裁定

**CONDITIONAL PASS**

主链路（VFS 写入 → Boot 状态更新 → agent Boot 模式 → search_skills/lookup_skill 工具）端到端连通，功能可用。但存在：
- SkillDistributor 死代码（我们写了但没接入）
- skill5_verified 错误标记
- 所有"预有"章节的 body checkbox 未更新
- 新增代码无单元测试

对于生产可用性：**主功能 OK**，技术债明确记录，需在后续 cleanup pass 处理。
