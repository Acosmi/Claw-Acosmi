---
name: exec
description: "Exec 工具用法、stdin 模式和 TTY 支持"
tools: bash
---

# Exec 工具

在工作区中运行 Shell 命令。通过 `process` 支持前台 + 后台执行。如果 `process` 被禁止，`exec` 同步运行并忽略 `yieldMs`/`background`。后台会话按 Agent 隔离；`process` 只能看到同一 Agent 的会话。

## 参数

- `command`（必填）
- `workdir`（默认为 cwd）
- `env`（键/值覆盖）
- `yieldMs`（默认 10000）：延迟后自动转后台
- `background`（bool）：立即转后台
- `timeout`（秒，默认 1800）：超时后终止
- `pty`（bool）：可用时在伪终端中运行（TTY-only CLI、编程 Agent、终端 UI）
- `host`（`sandbox | gateway | node`）：执行位置
- `security`（`deny | allowlist | full`）：`gateway`/`node` 的执行策略
- `ask`（`off | on-miss | always`）：`gateway`/`node` 的审批提示
- `node`（string）：`host=node` 时的 node ID/名称
- `elevated`（bool）：请求提权模式（Gateway 宿主机）；仅当 elevated 解析为 `full` 时才强制 `security=full`

说明：

- `host` 默认为 `sandbox`。
- 沙箱关闭时忽略 `elevated`（exec 已在宿主机运行）。
- `gateway`/`node` 审批由 `~/.openacosmi/exec-approvals.json` 控制。
- 需要“有制度约束的宿主机执行”时，优先使用 `host=gateway`。
- `node` 需要已配对的 node（伴侣应用或无头 node 主机）。
- `host=node` 只在命令必须落到远端机器时使用；它是兼容/远端路径，不是默认主路径。
- 如果有多个 node 可用，设置 `exec.node` 或 `tools.exec.node` 选择一个。
- 在非 Windows 主机上，exec 在设置了 `SHELL` 时使用它；如果 `SHELL` 是 `fish`，优先从 `PATH` 中选择 `bash`（或 `sh`）以避免 fish 不兼容的脚本，如果都不存在则回退到 `SHELL`。
- 宿主机执行（`gateway`/`node`）拒绝 `env.PATH` 和加载器覆盖（`LD_*`/`DYLD_*`）以防止二进制劫持或注入代码。
- 重要：沙箱**默认关闭**。如果沙箱关闭，`host=sandbox` 直接在 Gateway 宿主机上运行（无容器）且**不需要审批**。要启用审批，使用 `host=gateway` 并配置 exec 审批（或启用沙箱）。

## 配置

- `tools.exec.notifyOnExit`（默认 true）：为 true 时，后台化的 exec 会话在退出时排队系统事件并请求心跳。
- `tools.exec.approvalRunningNoticeMs`（默认 10000）：审批控制的 exec 运行超过此时间后发出单个"运行中"通知（0 禁用）。
- `tools.exec.host`（默认 `sandbox`）
- `tools.exec.security`（默认：sandbox 为 `deny`，gateway + node 未设置时为 `allowlist`）
- `tools.exec.ask`（默认 `on-miss`）
- `tools.exec.node`（默认未设置）
- `tools.exec.pathPrepend`：exec 运行时要添加到 `PATH` 前面的目录列表。
- `tools.exec.safeBins`：仅 stdin 的安全二进制文件，无需显式白名单即可运行。

示例：

```json5
{
  tools: {
    exec: {
      pathPrepend: ["~/bin", "/opt/oss/bin"],
    },
  },
}
```

### PATH 处理

- `host=gateway`：将您的 login-shell `PATH` 合并到 exec 环境中。宿主机执行拒绝 `env.PATH` 覆盖。守护进程本身仍使用最小 `PATH`：
  - macOS：`/opt/homebrew/bin`、`/usr/local/bin`、`/usr/bin`、`/bin`
  - Linux：`/usr/local/bin`、`/usr/bin`、`/bin`
- `host=sandbox`：在容器内运行 `sh -lc`（登录 Shell），因此 `/etc/profile` 可能会重置 `PATH`。Claw Acosmi 通过内部环境变量在 profile 加载后前置 `env.PATH`（无 Shell 插值）；`tools.exec.pathPrepend` 也在此生效。
- `host=node`：只有您传递的非阻止 env 覆盖会发送到 node。宿主机执行拒绝 `env.PATH` 覆盖。无头 node 主机仅在 `PATH` 前置 node 主机 PATH 时接受（不接受替换）。macOS node 完全丢弃 `PATH` 覆盖。

每 Agent node 绑定（使用配置中的 Agent 列表索引）：

```bash
openacosmi config get agents.list
openacosmi config set agents.list[0].tools.exec.node "node-id-or-name"
```

控制 UI：Nodes 标签页包含一个小型"Exec node binding"面板用于相同设置。

## 会话覆盖（`/exec`）

使用 `/exec` 设置 `host`、`security`、`ask` 和 `node` 的**每会话**默认值。发送不带参数的 `/exec` 显示当前值。

示例：

```
/exec host=gateway security=allowlist ask=on-miss node=mac-1
```

## 授权模型

`/exec` 仅对**已授权发送者**生效（渠道白名单/配对加 `commands.useAccessGroups`）。它仅更新**会话状态**，不写入配置。要硬禁用 exec，通过工具策略拒绝（`tools.deny: ["exec"]` 或每 Agent）。除非您显式设置 `security=full` 和 `ask=off`，否则宿主机审批仍然适用。

## Exec 审批（伴侣应用 / node 主机）

沙箱化的 Agent 可以要求在 `exec` 在 Gateway 或 node 宿主机上运行前进行逐请求审批。参见 [Exec 审批](/tools/exec-approvals) 了解策略、白名单和 UI 流程。

主路径建议：

- 默认先治理 `host=gateway` 的审批与 allowlist。
- 只有在命令必须落到远端机器时才切到 `host=node`。
- 当前 gateway 侧 `exec.approvals.node.*` 仍不适合作为主验收依赖；不要把 node 审批流写成默认操作心智。

当需要审批时，exec 工具立即返回 `status: "approval-pending"` 和审批 ID。审批通过（或拒绝/超时）后，Gateway 发出系统事件（`Exec finished` / `Exec denied`）。如果命令在 `tools.exec.approvalRunningNoticeMs` 后仍在运行，会发出单个 `Exec running` 通知。

## 白名单 + 安全二进制

白名单执行仅匹配**解析后的二进制路径**（不匹配基本名称）。当 `security=allowlist` 时，Shell 命令仅在每个管道段都在白名单中或是安全二进制时才自动允许。白名单模式下拒绝链式操作（`;`、`&&`、`||`）和重定向。

## 示例

前台：

```json
{ "tool": "exec", "command": "ls -la" }
```

后台 + 轮询：

```json
{"tool":"exec","command":"npm run build","yieldMs":1000}
{"tool":"process","action":"poll","sessionId":"<id>"}
```

发送按键（tmux 风格）：

```json
{"tool":"process","action":"send-keys","sessionId":"<id>","keys":["Enter"]}
{"tool":"process","action":"send-keys","sessionId":"<id>","keys":["C-c"]}
{"tool":"process","action":"send-keys","sessionId":"<id>","keys":["Up","Up","Enter"]}
```

提交（仅发送 CR）：

```json
{ "tool": "process", "action": "submit", "sessionId": "<id>" }
```

粘贴（默认使用括号粘贴模式）：

```json
{ "tool": "process", "action": "paste", "sessionId": "<id>", "text": "line1\nline2\n" }
```

## apply_patch（实验性）

`apply_patch` 是 `exec` 的子工具，用于结构化多文件编辑。需显式启用：

```json5
{
  tools: {
    exec: {
      applyPatch: { enabled: true, allowModels: ["gpt-5.2"] },
    },
  },
}
```

说明：

- 仅适用于 OpenAI/OpenAI Codex 模型。
- 工具策略仍然适用；`allow: ["exec"]` 隐式允许 `apply_patch`。
- 配置位于 `tools.exec.applyPatch` 下。

## 故障树

- `host=sandbox` 与预期不符：当前环境未启用沙箱时，命令实际落在 Gateway 宿主机且不走审批；若目标是“受治理的宿主机执行”，应改用 `host=gateway`。
- `host=node` 无法运行：没有已配对 node、存在多个 node 但未显式指定 `node`，或目标 node 不支持 `system.run`。
- 审批/allowlist 判断和预期不一致：`host=gateway` 看 Gateway 本地审批文件；`host=node` 看 node 主机本地 `~/.openacosmi/exec-approvals.json`，Gateway 不再代理远程读取。
- 命令在宿主机失败但不是审批问题：`workdir` 不存在、可执行文件不在 PATH、`env.PATH`/`LD_*`/`DYLD_*` 覆盖被拒绝，或目标 shell 与脚本假设不一致。

## 回滚步骤

1. 先退回最小命令（如 `pwd`、`which <bin>`、`echo ok`）验证 `host`、`workdir`、PATH 与 shell，再恢复原命令。
2. 若需要“有制度约束的本机执行”，统一切回 `host=gateway security=allowlist ask=on-miss`；不要把 `host=sandbox` 当成审批主路径。
3. 若命令必须在远端执行，显式指定 `node`，并先确认目标 node 的 `system.run` 能力与本地审批文件配置，再放大到真实任务。
4. 排障时优先缩小命令与环境差异，不要为了验证链路直接放宽到 `security=full ask=off`，除非已有明确授权。

## 验收清单

- 执行位置选择正确：受治理的宿主机命令走 `host=gateway`，只有真实远端任务才走 `host=node`。
- 最小命令已确认 `workdir`、shell、可执行文件解析和输出路径符合预期。
- 审批/allowlist 行为与 `host` 一致：Gateway 看 Gateway 本地审批，node 看 node 本地审批，不依赖 Gateway 代理。
- 失败时能从返回结果区分审批未过、节点不可用、环境缺失和命令本身失败，不把它们混写成同一种故障。
