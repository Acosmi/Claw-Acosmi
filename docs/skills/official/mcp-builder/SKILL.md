---
name: mcp-builder
description: 创建高质量 MCP（模型上下文协议）服务器的指南，使 LLM 能通过精心设计的工具与外部服务交互。用于构建 MCP 服务器以集成外部 API 或服务，支持 Python（FastMCP）或 Node/TypeScript（MCP SDK）。
license: Complete terms in LICENSE.txt
---

# MCP 服务器开发指南

## 概览

创建 MCP（模型上下文协议）服务器，使 LLM 能通过精心设计的工具与外部服务交互。MCP 服务器的质量取决于它能多好地帮助 LLM 完成实际任务。

---

# 流程

## 🚀 总体工作流

创建高质量 MCP 服务器包含四个主要阶段：

### 阶段 1：深度研究与规划

#### 1.1 理解现代 MCP 设计

**API 覆盖 vs 工作流工具：**
在全面的 API 端点覆盖和专业化工作流工具之间取得平衡。工作流工具对特定任务更方便，而全面覆盖让智能体灵活组合操作。不确定时，优先选择全面的 API 覆盖。

**工具命名与可发现性：**
清晰、描述性的工具名称帮助智能体快速找到正确工具。使用一致的前缀（如 `github_create_issue`、`github_list_repos`）和面向操作的命名。

**上下文管理：**
智能体受益于简洁的工具描述和筛选/分页结果的能力。设计返回聚焦、相关数据的工具。

**可操作的错误消息：**
错误消息应引导智能体找到解决方案，提供具体建议和下一步操作。

#### 1.2 学习 MCP 协议文档

**浏览 MCP 规范：**

从站点地图开始查找相关页面：`https://modelcontextprotocol.io/sitemap.xml`

然后获取带 `.md` 后缀的特定页面（如 `https://modelcontextprotocol.io/specification/draft.md`）。

重点审查页面：

- 规范概览和架构
- 传输机制（Streamable HTTP、stdio）
- 工具、资源和提示词定义

#### 1.3 学习框架文档

**推荐技术栈：**

- **语言**：TypeScript（高质量 SDK 支持，且在多种执行环境中兼容性好。AI 模型擅长生成 TypeScript 代码）
- **传输**：远程服务器使用 Streamable HTTP（无状态 JSON，更易扩展和维护）。本地服务器使用 stdio。

**加载框架文档：**

- **MCP 最佳实践**：[📋 查看最佳实践](./reference/mcp_best_practices.md)

**TypeScript（推荐）：**

- **TypeScript SDK**：通过 WebFetch 加载 `https://raw.githubusercontent.com/modelcontextprotocol/typescript-sdk/main/README.md`
- [⚡ TypeScript 指南](./reference/node_mcp_server.md)

**Python：**

- **Python SDK**：通过 WebFetch 加载 `https://raw.githubusercontent.com/modelcontextprotocol/python-sdk/main/README.md`
- [🐍 Python 指南](./reference/python_mcp_server.md)

#### 1.4 规划实现

**理解 API：**
审查服务的 API 文档，识别关键端点、认证要求和数据模型。按需使用网络搜索和 WebFetch。

**工具选择：**
优先全面 API 覆盖。列出要实现的端点，从最常用的操作开始。

---

### 阶段 2：实现

#### 2.1 设置项目结构

参见语言特定指南：

- [⚡ TypeScript 指南](./reference/node_mcp_server.md) - 项目结构、package.json、tsconfig.json
- [🐍 Python 指南](./reference/python_mcp_server.md) - 模块组织、依赖

#### 2.2 实现核心基础设施

创建共享工具：

- 带认证的 API 客户端
- 错误处理辅助函数
- 响应格式化（JSON/Markdown）
- 分页支持

#### 2.3 实现工具

每个工具需要：

**输入 Schema：**

- 使用 Zod（TypeScript）或 Pydantic（Python）
- 包含约束和清晰描述
- 在字段描述中添加示例

**输出 Schema：**

- 尽可能定义 `outputSchema` 用于结构化数据
- 在工具响应中使用 `structuredContent`（TypeScript SDK 功能）

**工具描述：**

- 功能的简洁摘要
- 参数描述
- 返回类型 schema

**实现：**

- I/O 操作使用 async/await
- 带可操作消息的错误处理
- 在适用时支持分页
- 使用现代 SDK 时同时返回文本内容和结构化数据

**注解：**

- `readOnlyHint`：true/false
- `destructiveHint`：true/false
- `idempotentHint`：true/false
- `openWorldHint`：true/false

---

### 阶段 3：审查与测试

#### 3.1 代码质量

审查：

- 无重复代码（DRY 原则）
- 一致的错误处理
- 完整的类型覆盖
- 清晰的工具描述

#### 3.2 构建与测试

**TypeScript：**

- 运行 `npm run build` 验证编译
- 使用 MCP Inspector 测试：`npx @modelcontextprotocol/inspector`

**Python：**

- 验证语法：`python -m py_compile your_server.py`
- 使用 MCP Inspector 测试

详细测试方法和质量清单参见语言特定指南。

---

### 阶段 4：创建评估

实现 MCP 服务器后，创建全面的评估来测试其效果。

**加载 [✅ 评估指南](./reference/evaluation.md) 获取完整评估指引。**

#### 4.1 理解评估目的

使用评估测试 LLM 能否有效使用你的 MCP 服务器回答真实、复杂的问题。

#### 4.2 创建 10 个评估问题

按评估指南中的流程：

1. **工具检查**：列出可用工具并理解其功能
2. **内容探索**：使用只读操作探索可用数据
3. **问题生成**：创建 10 个复杂、真实的问题
4. **答案验证**：自行解答每个问题以验证答案

#### 4.3 评估要求

确保每个问题：

- **独立**：不依赖其他问题
- **只读**：仅需非破坏性操作
- **复杂**：需要多次工具调用和深入探索
- **真实**：基于人类真正关心的用例
- **可验证**：单一、明确的答案，可通过字符串比较验证
- **稳定**：答案不会随时间改变

#### 4.4 输出格式

创建如下结构的 XML 文件：

```xml
<evaluation>
  <qa_pair>
    <question>查找关于使用动物代号的 AI 模型发布的讨论。一个模型需要特定的安全等级指定，使用 ASL-X 格式。以一种有斑点的野猫命名的模型需要确定的数字 X 是多少？</question>
    <answer>3</answer>
  </qa_pair>
<!-- 更多 qa_pair... -->
</evaluation>
```

---

# 参考文件

## 📚 文档库

开发过程中按需加载以下资源：

### 核心 MCP 文档（优先加载）

- **MCP 协议**：从站点地图 `https://modelcontextprotocol.io/sitemap.xml` 开始，然后获取带 `.md` 后缀的特定页面
- [📋 MCP 最佳实践](./reference/mcp_best_practices.md) - 通用 MCP 指南包括：
  - 服务器和工具命名约定
  - 响应格式指南（JSON vs Markdown）
  - 分页最佳实践
  - 传输选择（Streamable HTTP vs stdio）
  - 安全和错误处理标准

### SDK 文档（阶段 1/2 加载）

- **Python SDK**：从 `https://raw.githubusercontent.com/modelcontextprotocol/python-sdk/main/README.md` 获取
- **TypeScript SDK**：从 `https://raw.githubusercontent.com/modelcontextprotocol/typescript-sdk/main/README.md` 获取

### 语言特定实现指南（阶段 2 加载）

- [🐍 Python 实现指南](./reference/python_mcp_server.md) - 完整 Python/FastMCP 指南
- [⚡ TypeScript 实现指南](./reference/node_mcp_server.md) - 完整 TypeScript 指南

### 评估指南（阶段 4 加载）

- [✅ 评估指南](./reference/evaluation.md) - 完整评估创建指南
