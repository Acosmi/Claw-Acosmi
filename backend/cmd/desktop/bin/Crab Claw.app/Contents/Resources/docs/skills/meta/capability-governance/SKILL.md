---
name: capability-governance
description: "能力树治理：查看、诊断、提案修改能力树节点与工具绑定，使用 capability_manage 工具"
---

# 能力树治理

## 能力树概览

能力树（Capability Tree）是 Crab Claw 的工具注册与路由中枢。所有工具、子智能体、权限、提示词、策略均从树派生。

### 节点维度

| 维度 | 说明 |
|------|------|
| Runtime | 所属组件、启用条件 |
| Prompt | 摘要、排序、委托说明 |
| Routing | 意图层级、关键词、优先级 |
| Perms | 安全等级、审批类型、提权提示 |
| Skills | 可绑定性、已绑定技能 |
| Display | 图标、标题、动词 |
| Policy | 策略组、安全配置文件 |

### 节点类型

- **静态工具** (24个): bash, read_file, write_file 等 — `Bindable: true`
- **非绑定工具** (8个): search_skills, capability_manage 等 — `Bindable: false`
- **动态组** (3个): argus_*, remote_mcp, local_mcp — 运行时动态注册

## capability_manage 操作

### 只读操作

| 动作 | 用途 |
|------|------|
| `list` | 列出所有节点（含过滤） |
| `show` | 查看单个节点完整详情 |
| `derive` | 执行派生目标（D1-D9） |
| `diagnose` | 健康检查（L1-L3） |
| `diff` | 对比两个节点 |
| `search` | 按关键词搜索节点 |

### 写入操作（需 exec_escalation 审批）

| 动作 | 用途 |
|------|------|
| `propose_register` | 提案注册新节点 |
| `propose_update` | 提案更新节点属性 |
| `propose_routing` | 提案修改路由规则 |
| `propose_binding` | 提案修改技能绑定 |
| `apply_patch` | 应用 TreePatch（需审批） |

### 派生目标（D1-D9）

| ID | 用途 |
|----|------|
| D1 | 提示词工具索引 |
| D2 | 前端工具策略 |
| D3 | 意图路由 tier 表 |
| D4 | 前端工具展示 JSON |
| D5 | 后端工具注册顺序 |
| D6 | 前端配置向导 |
| D7 | 前端 agents 视图(go+json) |
| D8 | 后端策略组 |
| D9 | 技能绑定验证 |

## 诊断流程

```
capability_manage diagnose → 检查 L1(结构) + L2(一致性) + L3(绑定)
  L1: 节点 ID 唯一性、Parent 存在性、必填字段
  L2: 派生输出与树一致性
  L3: 技能绑定有效性（tools: 对齐 Bindable）
```

## TreePatch 格式

```json
{
  "operations": [
    {"op": "add", "path": "group/new_tool", "value": { "Name": "...", ... }},
    {"op": "replace", "path": "runtime/bash", "field": "Routing.MinTier", "value": "task_write"},
    {"op": "remove", "path": "deprecated/old_tool"}
  ]
}
```

## 治理原则

1. **单一真相源** — 树是工具定义的唯一来源，代码不应硬编码工具名
2. **派生而非复制** — 前端、提示词、路由等均从树派生，不独立维护
3. **审批保护** — 写入操作需 exec_escalation 审批
4. **自动验证** — apply_patch 后自动执行 L1-L3 检查
