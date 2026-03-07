---
name: skills-config
description: 技能配置完整 schema 与示例
---

# 技能配置

所有技能相关配置位于 `openacosmi.json` 的 `skills` 节点下。

```json5
{
  skills: {
    allowBundled: ["gemini", "coder"],  // 捆绑技能白名单（空=全部允许）
    load: {
      extraDirs: ["~/shared/skills", "~/Projects/my-skills"],
      watch: true,            // 监听技能文件变更自动热加载
      watchDebounceMs: 250,
    },
    entries: {
      "my-skill": {
        enabled: true,
        apiKey: "KEY_HERE",
        env: { MY_API_KEY: "KEY_HERE" },
        config: { endpoint: "https://example.com" },
      },
      other-skill: { enabled: false },
    },
  },
}
```

## 字段说明

- `allowBundled`：捆绑技能白名单。设置后只有列出的捆绑技能符合条件（工作区和 extraDirs 技能不受影响）
- `load.extraDirs`：额外扫描的技能目录（最低优先级）
- `load.watch`：监听技能目录变更并热刷新快照（默认 `true`）
- `load.watchDebounceMs`：技能监听防抖间隔（默认 250ms）
- `entries.<skillKey>`：单个技能覆盖配置

单技能字段：

- `enabled`：`false` 禁用技能（即使已捆绑/安装）
- `env`：注入运行环境变量（仅在进程中未设置时生效）
- `apiKey`：声明了 `primaryEnv` 的技能的 API Key 快捷配置
- `config`：自定义字段容器，自定义键必须放在此处

## 注意

- `entries` 中的 key 默认对应技能 `name`。若技能定义了 `metadata.openacosmi.skillKey`，使用该值作为 key
- `watch` 开启时，技能变更在下一个 Agent 轮次生效

## 沙箱技能与环境变量

沙箱会话中，技能进程运行于 Docker 容器内，**不继承宿主机环境变量**。

需要向沙箱注入变量，使用以下任一方式：

- `agents.defaults.sandbox.docker.env`（或单 Agent 的 `agents.list[].sandbox.docker.env`）
- 将变量烘焙到自定义沙箱镜像中

全局 `env` 和 `skills.entries.<skill>.env/apiKey` 仅对宿主机模式生效。
