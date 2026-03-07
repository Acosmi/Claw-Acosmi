---
name: memory-uhms
description: "记忆治理：memory.* 与 memory.uhms.* 的增删查压缩与向量维护"
---

# Memory 与 UHMS

用于太虚记忆系统的直接操作、压缩提交、语义检索与向量维护。

## 覆盖方法
- `memory.list`
- `memory.get`
- `memory.delete`
- `memory.compress`
- `memory.commit`
- `memory.decay.run`
- `memory.import.skills`
- `memory.stats`
- `memory.uhms.status`
- `memory.uhms.search`
- `memory.uhms.add`
- `memory.uhms.llm.get`
- `memory.uhms.llm.set`
- `memory.vector.optimize`

## 推荐流程
1. 用 `memory.stats` 和 `memory.uhms.status` 确认系统状态。
2. 用 `memory.list/get` 定位目标记忆。
3. 清理噪声走 `memory.delete`，结构整理走 `memory.compress/commit`。
4. 语义问题用 `memory.uhms.search` 验证召回质量。
5. 大量写入后可执行 `memory.vector.optimize`。

## 风险控制
- 删除前记录 ID 与内容摘要。
- 变更 UHMS LLM 配置后做一次检索回归。

## 成功判定
- 检索结果稳定，压缩后关键记忆仍可命中。
