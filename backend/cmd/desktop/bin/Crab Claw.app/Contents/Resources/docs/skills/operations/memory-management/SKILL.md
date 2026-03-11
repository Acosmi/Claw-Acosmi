---
name: memory-management
description: "记忆提取与治理：UHMS 增删查压缩、向量优化与语义检索质量保障"
---

# 记忆提取与治理技能

## 记忆系统架构

太虚记忆系统 (UHMS) 提供长期语义记忆存储，支持跨会话的知识积累与检索。

## 核心操作

| 操作 | 工具 | 用途 |
|------|------|------|
| 语义搜索 | `memory_search` | 按关键词检索相关记忆 |
| 精确获取 | `memory_get` | 按 ID/路径获取完整记忆条目 |
| 列表浏览 | `memory.list` | 列出记忆条目（分页） |
| 删除 | `memory.delete` | 删除指定条目（需先记录 ID + 摘要） |
| 压缩 | `memory.compress` | 合并冗余条目，降低存储占用 |
| 提交 | `memory.commit` | 将待处理记忆持久化 |
| 添加 | `memory.uhms.add` | 新增语义记忆条目 |
| 向量优化 | `memory.vector.optimize` | 批量写入后重建向量索引 |

## 标准治理流程

### 1. 检查阶段
```
memory.stats → 了解当前存储规模与健康度
memory.list → 定位目标条目
memory_search → 验证检索质量
```

### 2. 清理阶段
```
memory.delete → 移除过期/无效条目（先记录 ID + 内容摘要）
memory.compress → 合并冗余
memory.commit → 持久化变更
```

### 3. 验证阶段
```
memory_search → 回归测试：关键记忆是否仍可检索
memory.vector.optimize → 批量操作后重建索引
```

## 关键规则

1. **删除前必须记录**：先获取条目 ID + 内容摘要，再执行删除
2. **压缩后必须验证**：运行搜索回归测试，确保关键记忆未丢失
3. **配置变更后必须测试**：UHMS config 修改后立即做搜索质量验证
4. **批量写入后优化向量**：大量新增条目后运行 `memory.vector.optimize`

## 记忆衰减机制

- 系统支持自动衰减（decay），低频访问的记忆权重逐渐降低
- 定期检查衰减状态，对重要记忆手动刷新访问权重
- 衰减不等于删除——衰减的记忆仍可检索，只是排序靠后
