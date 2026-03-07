---
name: creating-skills
description: 创建自定义技能：目录结构、frontmatter、调试与发布
---

# 创建自定义技能

技能是扩展 创宇太虚（Claw Acosmi） 能力的首选方式。一个技能即一个目录，内含 `SKILL.md` 文件和可选的脚本/资源。

## 技能是什么？

`SKILL.md` 包含 YAML frontmatter 元数据和 Markdown 指令，系统加载后作为 system prompt 的一部分注入智能体上下文。智能体通过 `lookup_skill` 工具按需获取完整内容。

## 快速开始：创建第一个技能

### 1. 创建目录

Agent 专属技能放在工作区的 `.agent/skills/` 下：

```bash
mkdir -p {workspace}/.agent/skills/hello-world
```

共享技能（跨 Agent）放在 `skills.load.extraDirs` 配置的目录，或项目 `docs/skills/{分类}/` 下。

### 2. 编写 SKILL.md

```markdown
---
name: hello-world
description: 问候技能示例。
---

# Hello World 技能

当用户请求问候时，用 `bash` 工具执行 `echo "Hello from Claw Acosmi!"`，并将输出直接回复给用户。
```

frontmatter 必填：`name`（唯一标识符）、`description`（80 字符内，用于技能索引摘要）。

### 3. 可选：添加工具调用

在 SKILL.md 的 Markdown 部分描述如何使用现有工具（`bash`、`read_file`、`web_fetch` 等）或告知模型使用场景。技能本身不定义新工具，而是教导模型何时及如何组合现有工具完成任务。

### 4. 热加载生效

`skills.load.watch` 默认开启，保存文件后下一个 Agent 轮次自动加载无需重启。

## frontmatter 完整格式

```markdown
---
name: my-skill
description: 技能简介（尽量用中文，更节省 token）
metadata: {"openacosmi": {"emoji": "🛠", "requires": {"bins": ["rg"]}, "primaryEnv": "MY_KEY"}}
user-invocable: true
disable-model-invocation: false
---
```

## 门控（按条件加载）

通过 `metadata.openacosmi.requires` 指定前提条件，不满足条件时技能自动跳过：

```markdown
metadata: {"openacosmi": {"requires": {"bins": ["ffmpeg"], "env": ["OPENAI_API_KEY"]}}}
```

- `bins` — PATH 中必须存在的可执行文件
- `env` — 必须存在的环境变量（或在 `skills.entries.<name>.apiKey` 中配置）
- `config` — `openacosmi.json` 中必须为 truthy 的路径

## 技能目录结构示例

```
docs/skills/tools/my-skill/
├── SKILL.md          # 必需：技能定义
├── README.md         # 可选：人类阅读文档
├── scripts/
│   └── helper.py     # 可选：辅助脚本
└── examples/
    └── demo.md       # 可选：使用示例
```

在 `SKILL.md` 中用 `{baseDir}` 引用技能目录路径。

## 最佳实践

- **简洁明确**：指令告诉模型*做什么*，而非如何成为 AI
- **中文优先**：中文描述比英文更节省 token，语义同等清晰
- **安全第一**：使用 `bash` 工具时，确保 prompt 不允许用户输入任意命令
- **本地测试**：通过 `lookup_skill my-skill` 验证技能加载；或在对话中触发相关场景观察模型行为

## 发布到技能商店

使用 nexus-v4 技能商店 RPC 发布和分享技能：通过 `skills.store.*` 接口管理技能生命周期。
