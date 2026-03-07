---
name: coder
description: "Open Coder: delegate multi-file coding tasks to an independent sub-agent via spawn_coder_agent"
metadata: |
  { "openacosmi": { "emoji": "💻" } }
---

# Open Coder — 编程子智能体使用指南

Open Coder 是独立的编程子智能体，通过 `spawn_coder_agent` 工具委派任务。
它在独立 LLM 会话中运行，拥有自己的工具集（文件读写、沙箱 bash、grep、glob），通过委托合约（Delegation Contract）与主智能体协商。

## 何时用 spawn_coder_agent（vs 直接编辑）

| 场景 | 工具 | 原因 |
|------|------|------|
| 多文件编辑、重构、测试编写 | spawn_coder_agent | 独立会话 + 完整上下文 |
| 单文件 <50 行简单修改 | write_file 直接操作 | 无需启动子智能体 |
| 运行测试、构建、lint | bash 直接执行 | 简单命令无需委托 |
| 需要沙箱隔离的命令 | spawn_coder_agent | 子智能体 bash 默认沙箱化 |

**规则**: 多文件或复杂编码 → spawn_coder_agent；单文件简单改动 → 直接操作。

## spawn_coder_agent 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `task_brief` | 是 | 任务描述（清晰、具体） |
| `scope` | 是 | 允许操作的文件/目录范围（必填，定义子智能体可访问的路径） |
| `constraints` | 否 | 限制条件（如 no_network, read_only 等） |
| `parent_contract` | 否 | 恢复协商时使用：上次挂起的合约 ID |

## 协商循环

子智能体可能返回以下状态：

- **completed**: 任务完成 → 审核结果并汇报用户
- **partial**: 部分完成 → 检查 partial_artifacts，决定是否继续
- **needs_auth**: 需要授权 → 评估 auth_request 的风险，LOW 直接重新委派，HIGH 询问用户
- 最多 3 轮协商，超过后上报用户

## 工作流示例

```
用户: "帮我重构 backend/internal/media/ 下所有文件的错误处理"

主智能体判断: 多文件重构 → 委托 Open Coder
工具调用: spawn_coder_agent(
  task_brief="重构 backend/internal/media/ 目录下所有 Go 文件的错误处理，
              将 fmt.Errorf 改为 thiserror 风格的 typed error",
  scope="backend/internal/media/"
)

Open Coder 执行 → 返回 completed + 修改清单
主智能体审核 → 汇报用户
```
