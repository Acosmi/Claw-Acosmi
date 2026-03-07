---
name: exec-approvals
description: "Exec 审批、白名单和沙箱逃逸提示"
---

# Exec 审批

Exec 审批是**真实主机执行**的安全护栏，用于允许沙箱化的 Agent 在真实主机（`gateway` 或 `node`）上运行命令。可以将其视为安全联锁：只有当策略 + 白名单 +（可选）用户审批全部同意时，命令才被允许。Exec 审批是在工具策略和提权控制**之外**的额外保障（除非 elevated 设为 `full`，会跳过审批）。有效策略取 `tools.exec.*` 和审批默认值中**更严格的**；如果审批字段被省略，使用 `tools.exec` 的值。

## 主路径

- 当前主路径是 `gateway/mac companion` 的审批治理。
- `node host` 仍然是兼容/远端执行路径，不是本阶段默认主验收路径。
- Go Gateway 当前仅完整实现 `exec.approvals.get/set`。
- `exec.approvals.node.get/set` 在 Gateway 侧不再作为管理路径；node 审批文件改为节点主机本地维护。

如果伴侣应用 UI **不可用**，任何需要提示的请求由**审批回退**（默认：deny）决定。

## 适用范围

Exec 审批在执行主机上本地执行：

- **Gateway 主机** → Gateway 机器上的 `openacosmi` 进程
- **node 主机** → node 运行器（macOS 伴侣应用或无头 node 主机）

当前能力边界：

- Gateway 主机审批文件可通过 `exec.approvals.get/set` 正常治理。
- node 主机本地同样有 `~/.openacosmi/exec-approvals.json` 与 `system.execApprovals.get/set` 能力语义。
- 不要把“node 本地有审批文件”误写成“Gateway 已可统一远程管理 node 审批”；当前设计就是 node 本地维护。

macOS 分层：

- **node 主机服务** 通过本地 IPC 将 `system.run` 转发到 **macOS 应用**。
- **macOS 应用** 执行审批 + 在 UI 上下文中执行命令。

## 设置和存储

审批存储在执行主机上的本地 JSON 文件中：

`~/.openacosmi/exec-approvals.json`

示例结构：

```json
{
  "version": 1,
  "socket": {
    "path": "~/.openacosmi/exec-approvals.sock",
    "token": "base64url-token"
  },
  "defaults": {
    "security": "deny",
    "ask": "on-miss",
    "askFallback": "deny",
    "autoAllowSkills": false
  },
  "agents": {
    "main": {
      "security": "allowlist",
      "ask": "on-miss",
      "askFallback": "deny",
      "autoAllowSkills": true,
      "allowlist": [
        {
          "id": "B0C8C0B3-2C2D-4F8A-9A3C-5A4B3C2D1E0F",
          "pattern": "~/Projects/**/bin/rg",
          "lastUsedAt": 1737150000000,
          "lastUsedCommand": "rg -n TODO",
          "lastResolvedPath": "/Users/user/Projects/.../bin/rg"
        }
      ]
    }
  }
}
```

## 策略控制

### Security（`exec.security`）

- **deny**：阻止所有宿主机 exec 请求。
- **allowlist**：仅允许白名单中的命令。
- **full**：允许一切（等同于提权）。

### Ask（`exec.ask`）

- **off**：从不提示。
- **on-miss**：仅在白名单不匹配时提示。
- **always**：每个命令都提示。

### Ask 回退（`askFallback`）

如果需要提示但无 UI 可达，回退决定：

- **deny**：阻止。
- **allowlist**：仅在白名单匹配时允许。
- **full**：允许。

## 白名单（每 Agent）

白名单按**每个 Agent** 隔离。如果存在多个 Agent，在 macOS 应用中切换编辑的 Agent。模式为**不区分大小写的 Glob 匹配**。模式应解析为**二进制路径**（仅文件名的条目会被忽略）。旧版 `agents.default` 条目在加载时迁移到 `agents.main`。

示例：

- `~/Projects/**/bin/peekaboo`
- `~/.local/bin/*`
- `/opt/homebrew/bin/rg`

每个白名单条目跟踪：

- **id** 用于 UI 标识的稳定 UUID（可选）
- **上次使用**时间戳
- **上次使用的命令**
- **上次解析的路径**

## 自动允许技能 CLI

当**自动允许技能 CLI** 启用时，已知技能引用的可执行文件在 node 上被视为白名单（macOS node 或无头 node 主机）。使用 Gateway RPC 上的 `skills.bins` 获取技能 bin 列表。如果需要严格的手动白名单请禁用此功能。

## 安全二进制（仅 stdin）

`tools.exec.safeBins` 定义一小组**仅 stdin** 的二进制文件（例如 `jq`），可在白名单模式下**无需**显式白名单条目运行。安全二进制会拒绝位置文件参数和路径样 Token，因此只能操作传入的流。白名单模式下不自动允许 Shell 链式操作和重定向。

当每个顶级段都满足白名单时（包括安全二进制或技能自动允许），允许 Shell 链（`&&`、`||`、`;`）。白名单模式下仍不支持重定向。白名单解析期间拒绝命令替换（`$()` / 反引号），包括双引号内的；如果需要文字 `$()` 文本请使用单引号。

默认安全二进制：`jq`、`grep`、`cut`、`sort`、`uniq`、`head`、`tail`、`tr`、`wc`。

## 控制 UI 编辑

使用**控制 UI → Nodes → Exec approvals** 卡片编辑默认值、每 Agent 覆盖和白名单。选择范围（默认值或某个 Agent），调整策略，添加/移除白名单模式，然后**保存**。UI 显示每个模式的**上次使用**元数据以便保持列表整洁。

目标选择器选择 **Gateway**（本地审批）或 **Node**。  
但在当前 Go Gateway 下，应默认把 **Gateway** 视为唯一完整打通的管理目标。

Node 侧请区分两件事：

- node 主机本地审批能力存在
- Gateway 不负责远程代理 node 审批管理

因此当前更稳妥的做法是：

- Gateway 审批：用控制 UI / `exec.approvals.get/set`
- node 审批：优先在 node 主机本地维护 `~/.openacosmi/exec-approvals.json`

CLI：`openacosmi approvals` 当前可靠支持本地/Gateway 审批文件；`--node` 已明确不支持（参见 [审批 CLI](/cli/approvals)）。

## 审批流程

当需要提示时，Gateway 向操作者客户端广播 `exec.approval.requested`。控制 UI 和 macOS 应用通过 `exec.approval.resolve` 解决，然后 Gateway 将已批准的请求转发到 node 主机。

当需要审批时，exec 工具立即返回审批 ID。使用该 ID 关联后续系统事件（`Exec finished` / `Exec denied`）。如果在超时前未收到决定，请求被视为审批超时并作为拒绝原因呈现。

确认对话框包含：

- 命令 + 参数
- 工作目录
- Agent ID
- 解析后的可执行文件路径
- 主机 + 策略元数据

操作：

- **允许一次** → 立即运行
- **始终允许** → 添加到白名单 + 运行
- **拒绝** → 阻止

## 审批转发到聊天渠道

您可以将 exec 审批提示转发到任何聊天渠道（包括插件渠道）并通过 `/approve` 审批。使用标准出站投递管道。

配置：

```json5
{
  approvals: {
    exec: {
      enabled: true,
      mode: "session", // "session" | "targets" | "both"
      agentFilter: ["main"],
      sessionFilter: ["discord"], // 子串或正则表达式
      targets: [
        { channel: "slack", to: "U12345678" },
        { channel: "telegram", to: "123456789" },
      ],
    },
  },
}
```

在聊天中回复：

```
/approve <id> allow-once
/approve <id> allow-always
/approve <id> deny
```

### macOS IPC 流程

```
Gateway -> Node Service (WS)
                 |  IPC (UDS + token + HMAC + TTL)
                 v
             Mac App (UI + approvals + system.run)
```

安全说明：

- Unix socket 模式 `0600`，Token 存储在 `exec-approvals.json` 中。
- 同 UID 对等检查。
- 质询/响应（nonce + HMAC token + 请求哈希）+ 短 TTL。

## 系统事件

Exec 生命周期以系统消息呈现：

- `Exec running`（仅当命令超过运行通知阈值时）
- `Exec finished`
- `Exec denied`

这些在 node 报告事件后发布到 Agent 的会话。Gateway 主机 exec 审批在命令完成时发出相同的生命周期事件（可选在运行时间超过阈值时发出）。审批控制的 exec 在这些消息中复用审批 ID 作为 `runId` 以便关联。

## 含义

- **full** 权力很大；尽可能优先使用白名单。
- **ask** 让您保持在循环中同时仍允许快速审批。
- 每 Agent 白名单防止一个 Agent 的审批泄漏到其他 Agent。
- 审批仅适用于**已授权发送者**的宿主机 exec 请求。未授权发送者无法使用 `/exec`。
- `/exec security=full` 是已授权操作者的会话级便利工具，设计上会跳过审批。要硬阻止宿主机 exec，将审批 security 设为 `deny` 或通过工具策略拒绝 `exec` 工具。
- 对 `host=node` 而言，node 主机本地审批策略会生效；不要再把 Gateway 侧 node 审批读取/写入代理当成验收依赖。

相关：

- [Exec 工具](/tools/exec)
- [提权模式](/tools/elevated)
- [技能](/tools/skills)

## 故障树

- Gateway 上的 `exec.approvals.get/set` 生效，但 node 侧行为没变：通常是你改的是 Gateway 本地审批，而 node 仍走节点主机本地文件
- 期望通过 Gateway 管远程 node 审批：当前 `exec.approvals.node.get/set` 已明确不是管理路径，会返回不支持
- 明明在白名单里却仍被拦：要先看解析后的真实二进制路径、shell 分段和 ask/askFallback 是否更严格
- 需要提示但 UI 不可达：最终结果取决于 `askFallback`，不是所有场景都会弹出审批
- 把聊天远程审批或提权审批与 exec 白名单混成一件事：这会导致制度路径和实际执法面错位

## 回滚步骤

- 出现异常放行或异常拒绝时，先把策略收回到更保守的 `allowlist` 或 `deny`
- Gateway 路径先只验本地审批；涉及 node 时，改到 node 主机本地维护审批文件
- 白名单命中异常时，先用最小命令重新验证解析后的二进制路径，不要直接放大到 `full`
- UI/审批通道不稳定时，先确认 `askFallback` 是否符合当前制度预期
- 若本次问题发生在兼容 node 路径，停止把 Gateway 远程代理当成修复入口

## 验收清单

- Gateway 主机上的 exec 审批能正确读取、修改并生效
- 若使用 node 路径，已在 node 主机本地验证审批文件与执行行为一致
- 白名单、ask、askFallback 三者的实际效果与制度设计一致
- 审批事件、拒绝、完成等生命周期可被审计追踪
- 没有再把 Gateway 远程 node 审批代理当成主验收依赖
