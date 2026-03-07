---
name: multi-agent-sandbox-tools
description: "每 Agent 沙箱 + 工具限制、优先级和示例"
---

# 多 Agent 沙箱与工具配置

## 概述

多 Agent 配置中，每个 Agent 可以有自己的：

- **沙箱配置**（`agents.list[].sandbox` 覆盖 `agents.defaults.sandbox`）
- **工具限制**（`tools.allow` / `tools.deny`，加上 `agents.list[].tools`）

这允许您以不同的安全配置运行多个 Agent：

- 拥有完全访问权限的个人助手
- 工具受限的家庭/工作 Agent
- 在沙箱中运行的公开 Agent

`setupCommand` 属于 `sandbox.docker`（全局或每 Agent），在容器创建时运行一次。

认证按 Agent 隔离：每个 Agent 从自己的 `agentDir` 认证存储读取：

```
~/.openacosmi/agents/<agentId>/agent/auth-profiles.json
```

凭据**不会**在 Agent 间共享。绝不要在 Agent 间复用 `agentDir`。如需共享凭据，将 `auth-profiles.json` 复制到其他 Agent 的 `agentDir`。

了解沙箱运行时行为，参见 [沙箱](/gateway/sandboxing)。调试"为什么被阻止"，参见 [沙箱 vs 工具策略 vs 提权](/gateway/sandbox-vs-tool-policy-vs-elevated) 和 `openacosmi sandbox explain`。

---

## 配置示例

### 示例 1：个人 + 受限家庭 Agent

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "default": true,
        "name": "Personal Assistant",
        "workspace": "~/.openacosmi/workspace",
        "sandbox": { "mode": "off" }
      },
      {
        "id": "family",
        "name": "Family Bot",
        "workspace": "~/.openacosmi/workspace-family",
        "sandbox": {
          "mode": "all",
          "scope": "agent"
        },
        "tools": {
          "allow": ["read"],
          "deny": ["exec", "write", "edit", "apply_patch", "process", "browser"]
        }
      }
    ]
  },
  "bindings": [
    {
      "agentId": "family",
      "match": {
        "provider": "whatsapp",
        "accountId": "*",
        "peer": {
          "kind": "group",
          "id": "120363424282127706@g.us"
        }
      }
    }
  ]
}
```

**结果：**

- `main` Agent：在宿主机上运行，完全工具访问
- `family` Agent：在 Docker 中运行（每 Agent 一个容器），仅 `read` 工具

---

### 示例 2：共享沙箱的工作 Agent

```json
{
  "agents": {
    "list": [
      {
        "id": "personal",
        "workspace": "~/.openacosmi/workspace-personal",
        "sandbox": { "mode": "off" }
      },
      {
        "id": "work",
        "workspace": "~/.openacosmi/workspace-work",
        "sandbox": {
          "mode": "all",
          "scope": "shared",
          "workspaceRoot": "/tmp/work-sandboxes"
        },
        "tools": {
          "allow": ["read", "write", "apply_patch", "exec"],
          "deny": ["browser", "gateway", "discord"]
        }
      }
    ]
  }
}
```

---

### 示例 2b：全局编码 Profile + 仅消息 Agent

```json
{
  "tools": { "profile": "coding" },
  "agents": {
    "list": [
      {
        "id": "support",
        "tools": { "profile": "messaging", "allow": ["slack"] }
      }
    ]
  }
}
```

**结果：**

- 默认 Agent 获得编码工具
- `support` Agent 仅消息（+ Slack 工具）

---

### 示例 3：每 Agent 不同的沙箱模式

```json
{
  "agents": {
    "defaults": {
      "sandbox": {
        "mode": "non-main",
        "scope": "session"
      }
    },
    "list": [
      {
        "id": "main",
        "workspace": "~/.openacosmi/workspace",
        "sandbox": {
          "mode": "off"
        }
      },
      {
        "id": "public",
        "workspace": "~/.openacosmi/workspace-public",
        "sandbox": {
          "mode": "all",
          "scope": "agent"
        },
        "tools": {
          "allow": ["read"],
          "deny": ["exec", "write", "edit", "apply_patch"]
        }
      }
    ]
  }
}
```

---

## 配置优先级

当全局（`agents.defaults.*`）和 Agent 特定（`agents.list[].*`）配置同时存在时：

### 沙箱配置

Agent 特定设置覆盖全局：

```
agents.list[].sandbox.mode > agents.defaults.sandbox.mode
agents.list[].sandbox.scope > agents.defaults.sandbox.scope
agents.list[].sandbox.workspaceRoot > agents.defaults.sandbox.workspaceRoot
agents.list[].sandbox.workspaceAccess > agents.defaults.sandbox.workspaceAccess
agents.list[].sandbox.docker.* > agents.defaults.sandbox.docker.*
agents.list[].sandbox.browser.* > agents.defaults.sandbox.browser.*
agents.list[].sandbox.prune.* > agents.defaults.sandbox.prune.*
```

**说明：**

- `agents.list[].sandbox.{docker,browser,prune}.*` 覆盖该 Agent 的 `agents.defaults.sandbox.{docker,browser,prune}.*`（当沙箱 scope 解析为 `"shared"` 时忽略）。

### 工具限制

过滤顺序：

1. **工具 Profile**（`tools.profile` 或 `agents.list[].tools.profile`）
2. **Provider 工具 Profile**（`tools.byProvider[provider].profile` 或 `agents.list[].tools.byProvider[provider].profile`）
3. **全局工具策略**（`tools.allow` / `tools.deny`）
4. **Provider 工具策略**（`tools.byProvider[provider].allow/deny`）
5. **Agent 特定工具策略**（`agents.list[].tools.allow/deny`）
6. **Agent Provider 策略**（`agents.list[].tools.byProvider[provider].allow/deny`）
7. **沙箱工具策略**（`tools.sandbox.tools` 或 `agents.list[].tools.sandbox.tools`）
8. **子 Agent 工具策略**（`tools.subagents.tools`，如适用）

每一级只能进一步限制工具，不能恢复先前级别拒绝的工具。如果设置了 `agents.list[].tools.sandbox.tools`，它会替换该 Agent 的 `tools.sandbox.tools`。如果设置了 `agents.list[].tools.profile`，它会覆盖该 Agent 的 `tools.profile`。Provider 工具键接受 `provider`（如 `google-antigravity`）或 `provider/model`（如 `openai/gpt-5.2`）。

### 工具组（简写）

工具策略（全局、Agent、沙箱）支持 `group:*` 条目，展开为多个具体工具：

- `group:runtime`：`bash`
- `group:fs`：`read_file`、`write_file`、`list_dir`
- `group:sessions`：`sessions_list`、`sessions_history`、`sessions_send`、`sessions_spawn`、`session_status`
- `group:memory`：`memory_search`、`memory_get`
- `group:ui`：`browser`、`canvas`
- `group:automation`：`cron`、`gateway`
- `group:messaging`：`message`
- `group:nodes`：`nodes`
- `group:openacosmi`：所有内置 Claw Acosmi 工具（不含 Provider 插件）

### 提权模式

`tools.elevated` 是全局基线（基于发送者的白名单）。`agents.list[].tools.elevated` 可以进一步限制特定 Agent 的提权（两者都必须允许）。

缓解模式：

- 拒绝不信任 Agent 的 `exec`（`agents.list[].tools.deny: ["exec"]`）
- 避免将发送者白名单路由到受限 Agent
- 如果只需要沙箱执行，全局禁用提权（`tools.elevated.enabled: false`）
- 为敏感 Profile 禁用每 Agent 提权（`agents.list[].tools.elevated.enabled: false`）

---

## 从单 Agent 迁移

**之前（单 Agent）：**

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.openacosmi/workspace",
      "sandbox": {
        "mode": "non-main"
      }
    }
  },
  "tools": {
    "sandbox": {
      "tools": {
        "allow": ["read", "write", "apply_patch", "exec"],
        "deny": []
      }
    }
  }
}
```

**之后（不同 Profile 的多 Agent）：**

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "default": true,
        "workspace": "~/.openacosmi/workspace",
        "sandbox": { "mode": "off" }
      }
    ]
  }
}
```

旧版 `agent.*` 配置由 `openacosmi doctor` 迁移；今后优先使用 `agents.defaults` + `agents.list`。

---

## 工具限制示例

### 只读 Agent

```json
{
  "tools": {
    "allow": ["read"],
    "deny": ["exec", "write", "edit", "apply_patch", "process"]
  }
}
```

### 安全执行 Agent（不修改文件）

```json
{
  "tools": {
    "allow": ["read", "exec", "process"],
    "deny": ["write", "edit", "apply_patch", "browser", "gateway"]
  }
}
```

### 仅通信 Agent

```json
{
  "tools": {
    "allow": ["sessions_list", "sessions_send", "sessions_history", "session_status"],
    "deny": ["exec", "write", "edit", "apply_patch", "read", "browser"]
  }
}
```

---

## 常见陷阱："non-main"

`agents.defaults.sandbox.mode: "non-main"` 基于 `session.mainKey`（默认 `"main"`），而非 Agent ID。群组/渠道会话总是有自己的键，因此被视为 non-main 并将被沙箱化。如果希望某个 Agent 从不沙箱，设置 `agents.list[].sandbox.mode: "off"`。

---

## 测试

配置多 Agent 沙箱和工具后：

1. **检查 Agent 路由：**

   ```exec
   openacosmi agents list --bindings
   ```

2. **验证沙箱容器：**

   ```exec
   docker ps --filter "name=openacosmi-sbx-"
   ```

3. **测试工具限制：**
   - 发送需要受限工具的消息
   - 验证 Agent 无法使用被拒绝的工具

4. **监控日志：**

   ```exec
   tail -f "${OPENACOSMI_STATE_DIR:-$HOME/.openacosmi}/logs/gateway.log" | grep -E "routing|sandbox|tools"
   ```

---

## 故障排除

### 尽管 `mode: "all"` Agent 未被沙箱化

- 检查是否有全局 `agents.defaults.sandbox.mode` 覆盖了它
- Agent 特定配置优先，因此设置 `agents.list[].sandbox.mode: "all"`

### 尽管有拒绝列表工具仍然可用

- 检查工具过滤顺序：全局 → Agent → 沙箱 → 子 Agent
- 每一级只能进一步限制，不能恢复
- 通过日志验证：`[tools] filtering tools for agent:${agentId}`

### 容器未按 Agent 隔离

- 在 Agent 特定沙箱配置中设置 `scope: "agent"`
- 默认为 `"session"`，每个会话创建一个容器

---

## 另请参阅

- [多 Agent 路由](/concepts/multi-agent)
- [沙箱配置](/gateway/configuration#agentsdefaults-sandbox)
- [会话管理](/concepts/session)
