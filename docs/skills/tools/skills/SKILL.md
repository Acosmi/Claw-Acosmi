---
name: skills
description: 技能系统：加载路径、优先级、门控规则与配置
---

# 技能系统（创宇太虚（Claw Acosmi））

创宇太虚（Claw Acosmi） 使用兼容 AgentSkills 格式的技能目录向智能体注入额外能力。每个技能是一个包含 `SKILL.md` 的目录，文件包含 YAML frontmatter 元数据和 Markdown 指令。系统启动时按优先级加载技能，依据环境、配置和可执行文件可用性过滤。

## 加载路径与优先级

技能从以下位置加载（同名技能按先加载者优先）：

1. **工作区技能**：`{workspace}/.agent/skills/` — Agent 专属自定义技能（最高优先级）
2. **捆绑技能**：网关二进制文件同级 `skills/` 目录（生产发行包，开发环境通常不存在）
3. **额外目录**：`openacosmi.json` 中 `skills.load.extraDirs` 配置的目录
4. **项目技能**：相对工作区或当前目录自动扫描的 `docs/skills/`（monorepo 开发环境主要来源）

> 注意：旧版 `~/.openacosmi/skills` 托管目录在当前 Go 版本中**未启用**。如需共享技能，使用 `skills.load.extraDirs`。

## 多 Agent 下的技能共享

多 Agent 配置下，每个 Agent 拥有独立工作区，因此：

- **Agent 专属技能**：放置在该 Agent 的 `{workspace}/.agent/skills/`，仅该 Agent 可见
- **跨 Agent 共享**：通过 `skills.load.extraDirs` 指向公共技能目录（所有 Agent 均适用）

## 插件技能

插件可通过在 `openacosmi.plugin.json` 中声明 `skills` 目录路径来携带自身技能。插件技能随插件启用生效，参与正常优先级规则。

## 技能商店（nexus-v4）

创宇太虚（Claw Acosmi） 内置技能商店客户端，连接 nexus-v4 MCP Server，支持浏览、拉取、刷新、链接技能：

```
skills.store.browse  — 按分类/关键词浏览
skills.store.pull    — 拉取指定技能到本地
skills.store.refresh — 刷新已安装技能
skills.store.link    — 链接远程技能目录
```

配置：在 `openacosmi.json` 中设置 `skills.store.url` 和 `skills.store.token`。

## 格式（AgentSkills 兼容）

`SKILL.md` 必须包含：

```markdown
---
name: my-skill
description: 技能功能的简洁描述
---
```

可选 frontmatter 字段：

- `homepage` — 展示在 Skills UI 的网址
- `user-invocable` — `true|false`（默认 `true`）。`true` 时技能作为用户斜杠命令暴露
- `disable-model-invocation` — `true|false`（默认 `false`）。`true` 时技能不进入模型 prompt（仍可用户调用）
- `command-dispatch` — `tool`（可选）。设为 `tool` 时斜杠命令跳过模型直接调度到工具
- `command-tool` — `command-dispatch: tool` 时调用的工具名
- `command-arg-mode` — `raw`（默认）。原始参数字符串透传给工具

  工具调用参数：`{ command: "<raw args>", commandName: "<斜杠命令>", skillName: "<技能名>" }`

## 门控（加载时过滤）

在 frontmatter 中用 `metadata` 单行 JSON 声明门控条件：

```markdown
---
name: my-skill
description: 示例技能
metadata: {"openacosmi": {"requires": {"bins": ["rg"], "env": ["MY_API_KEY"]}, "primaryEnv": "MY_API_KEY"}}
---
```

`metadata.openacosmi` 支持字段：

- `always: true` — 跳过所有门控，始终加载
- `emoji` — Skills UI 显示的 emoji
- `homepage` — 展示网址
- `os` — 平台限制列表（`darwin`、`linux`、`win32`）
- `requires.bins` — 所有程序必须存在于 `PATH`
- `requires.anyBins` — 至少一个程序存在于 `PATH`
- `requires.env` — 环境变量必须存在或在配置中提供
- `requires.config` — `openacosmi.json` 路径列表，必须为 truthy
- `primaryEnv` — 关联 `skills.entries.<name>.apiKey` 的环境变量名
- `install` — 安装器规格（brew/go/download）

无 `metadata.openacosmi` 时，技能始终符合条件（除非在配置中禁用）。

## 配置覆盖（openacosmi.json）

```json5
{
  skills: {
    entries: {
      "my-skill": {
        enabled: true,
        apiKey: "KEY_HERE",
        env: { MY_API_KEY: "KEY_HERE" },
        config: { endpoint: "https://example.com" },
      },
      other-skill: { enabled: false },
    },
    allowBundled: ["skill-a", "skill-b"],
  },
}
```

规则：

- `enabled: false` — 禁用技能（即使已捆绑/安装）
- `env` — 仅在进程中未设置该变量时注入到 Agent 运行环境
- `apiKey` — 声明了 `primaryEnv` 的技能的快捷配置
- `config` — 自定义技能字段的容器，自定义键必须放在此处
- `allowBundled` — 捆绑技能白名单（只有列出的捆绑技能才符合条件；工作区/extraDirs 技能不受影响）

## 环境注入（每次 Agent 运行）

Agent 运行时，创宇太虚（Claw Acosmi）：

1. 读取技能元数据
2. 将 `skills.entries.<key>.env` 或 `skills.entries.<key>.apiKey` 注入到运行环境变量（如该变量尚未设置）
3. 构建包含**符合条件**技能的 system prompt
4. 运行结束后恢复原始环境

这是**运行作用域**的注入，不影响全局进程环境。

## 会话快照（性能）

创宇太虚（Claw Acosmi） 在**会话开始时**对符合条件的技能拍摄快照，在同一会话的后续轮次中复用该列表。技能或配置的变更在下一个新会话生效。

## Token 影响（技能索引）

技能符合条件时，创宇太虚（Claw Acosmi） 将紧凑索引注入 system prompt：

```
<available_skills>
- skill-name: 技能描述（不超过 80 字符）
- ...
</available_skills>
```

Agent 通过 `lookup_skill` 按需获取完整 SKILL.md 内容。使用中文描述可显著降低 token 消耗。

## 配置参考

详见 [技能配置](/tools/skills-config)。
