---
name: skill-creator
description: "技能创建与配置：SKILL.md frontmatter 规范、目录结构、工具绑定契约、热加载与调试"
---

# 技能创建与配置

## 技能结构

```
skill-name/
├── SKILL.md          # 必需：frontmatter + Markdown 指令
├── scripts/          # 可选：辅助脚本
├── references/       # 可选：按需加载的参考文档
└── assets/           # 可选：模板、图片等输出资源
```

## Frontmatter 规范

### 最小必填

```yaml
---
name: my-skill
description: 技能简介（≤120 字符，用于技能索引摘要）
---
```

### 工具绑定（对齐能力树）

```yaml
---
name: my-tool-skill
description: 工具技能描述
tools: tool_name
metadata:
  tree_id: group/tool_name
  tree_group: group
  min_tier: task_light
  approval_type: none
---
```

**绑定契约**:
- `tools:` 的值必须与能力树 `CapabilityNode.Name` 完全一致
- 只有 `Bindable: true` 的节点可被绑定
- 动态工具（`argus_*`、`remote_*`、`mcp_*`）可绑定但无静态节点
- 第一个绑定的技能优先（first-wins）
- 绑定成功后，技能 description 注入工具 Description: `[Skill: <desc>]`

### 可选字段

```yaml
user-invocable: true              # 用户可通过 /skill-name 直接调用
disable-model-invocation: true    # 禁止模型自动触发（仅手动调用）
metadata:
  emoji: "🛠"
  category: claude
  requires:
    bins: ["ffmpeg"]              # PATH 中必须存在
    env: ["OPENAI_API_KEY"]       # 必须存在的环境变量
    config: ["features.foo"]      # openacosmi.json 中必须为 truthy
```

## 技能类型

| 类型 | 目录 | tools: 字段 | 说明 |
|------|------|------------|------|
| 工具技能 | tools/ | 必填 | 1:1 绑定能力树工具 |
| 运维技能 | operations/ | 无 | 工作流指引，无工具绑定 |
| 元技能 | meta/ | 无 | 关于技能系统本身 |
| 子系统技能 | subsystems/ | 无 | 子系统使用指南 |
| 内部技能 | claude/ | 无 | Claude 内部工作规范 |

## 配置（openacosmi.json）

```json5
{
  skills: {
    allowBundled: ["gemini", "coder"],  // 捆绑白名单（空=全部允许）
    load: {
      extraDirs: ["~/shared/skills"],   // 额外技能目录
      watch: true,                       // 热加载（默认 true）
      watchDebounceMs: 250,
    },
    entries: {
      "my-skill": {
        enabled: true,
        apiKey: "KEY_HERE",
        env: { MY_API_KEY: "KEY_HERE" },
        config: { endpoint: "https://example.com" },
      },
    },
  },
}
```

## 创建流程

1. **确定类型** — 工具技能 vs 运维/元/子系统技能
2. **创建目录** — `docs/skills/{类型}/{skill-name}/`
3. **编写 SKILL.md** — frontmatter + Markdown 指令
4. **工具绑定**（仅工具技能）— 确认 `tools:` 与能力树对齐
5. **热加载验证** — 保存后下一轮次自动生效，通过 `lookup_skill` 验证

## 编写原则

- **简洁** — 上下文窗口是公共资源，只写模型不具备的知识
- **指令式** — 告诉模型*做什么*，而非解释概念
- **渐进披露** — metadata 始终在上下文（~100词），body 按需加载（<500行），resources 更按需
- **中文优先** — 中文描述更节省 token
