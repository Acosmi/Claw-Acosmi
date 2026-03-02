<div align="center">

  <img src="./claw-crab-logo.png" alt="Claw Acosmi 创宇太虚架构图 / Architecture Diagram" width="160" />
  
  # <span style="color: #c9ccc6;">Claw Acosmi (创宇太虚)</span>

**「在虚空中创建新秩序，构筑太虚之境」**<br>
**"Creating new order in the void, building the realm of Acosmi"**<br><br>
**🌐 官网 / Official Website: [Acosmi.com](https://acosmi.com)**

[English](#english) | [中文](#中文)

</div>

---

<h2 id="中文">🇨🇳 中文介绍</h2>

## 🌌 愿景与破局

**Claw Acosmi (创宇太虚)** 致力于打破传统 AI 开发框架的束缚。在混沌的“虚空”中，我们以底层重构的姿态，为您创建全新的秩序。这是一个真正面向未来的**多模态、强化隔离、原生分布式**的复合 Agent 联邦架构。

与传统的 Node.js/Python 脚本级 Agent 框架相比，创宇太虚在**系统级安全控制**、**记忆检索效率**和**技能扩展域**上实现了升维打击。

---

## ✨ 核心能力与架构升维

通过采用跨越语言边界的深度混合架构 (`Rust` + `Go`)，创宇太虚做到了：

### 1. 🛡️ 坚如磐石：自研 Rust 原生沙箱

我们没有妥协于粗粒度的 Docker 容器。太虚底层使用了 **全自研的 `oa-sandbox` 引擎**，通过 Rust 语言特性实现了真正的操作系统 (OS) 级进程隔离。

- **三平台原生支持**：深层对接 macOS (Seatbelt FFI)、Linux (Landlock/Seccomp/Namespaces)、Windows (Restricted Token)。
- **微秒级启动**：抛弃了启动笨重的完整虚拟化环境，AI 生成的未知代码和命令将在微秒级延迟内于极致安全的“狱境（Jail）”中执行。
- *【越维打击】*：哪怕 AI 生成了具有破坏性、甚至尝试提权的恶意系统级代码，都会被原生内核边界无情拦截。

### 2. 🧠 过目不忘：结合字节技术的分布式记忆系统

摒弃了单体 JSON 文件与粗糙的本地 SQLite 以应对巨量上下文交互。

- 引入了**高维分布式向量存储与检索引擎 (UHMS)**（融合字节跳动相关检索技术理念）。
- 采用最前沿的全文检索 (FTS5) 与多维度的 Embedding 混合搜索。让 Agent 实现了上下文的永不脱节，拥有了如同人类般深邃的长线记忆。
- *【越维打击】*：在面对高达数十万 Tokens 的超大型代码仓库或复杂文档库分析时，系统仍可支持毫秒级的上下文精准召回，永不丢失核心推理链条。

### 3. 🌳 繁华如海：技能树 (Skill Tree) 与 工具树 (Tool Tree)

太虚抛弃了业界主流的扁平化 Tool 挂载机制，引入了**极具生命力的技能与工具双轨架构**。

- **技能树 (Skill Tree)**：自上而下指导 Agent 的思维方式与宏观策略（例如：一套完整的高级代码重构范式、深度的安全漏洞审查逻辑）。
- **工具树 (Tool Tree)**：自下而上赋予 Agent 与现实世界物理交互的原子能力 (例如：无缝执行 bash、精准抓取网页、自动化截取屏幕）。
- 它们支持热插拔、动态依赖注入，并伴随着 MCP (Model Context Protocol) 协议的深度集成，实现了 Agent 物理接入域的无限扩展。

### 4. 🪆 混沌初开：庞大的子智能体群 (Sub-Agents)

创宇太虚不再是一个孤独的思考者。

- **视觉感知智能体 (Argus)**：基于 Go+Rust 混合编译的视觉中枢，能够直视用户的桌面、感知屏幕的变化流，赋予系统真正的“视觉”与像素级多模态流解析能力。
- **更多核心子智能体 (即将接入)**：如 Swabble、Acosmo-Code 等将陆续接入。各个子智能体将在由沙箱划定的安全通信隔离区内，进行大规模的并行工作、互相博弈与协同决策。
- *【越维打击】*：从单一的大语言模型的纯文本处理，正式进化为具备**感知、推理、代码生成、视觉审查**的全能 Agent 兵团，在复杂的人效任务流中实现真正意义上的「分布式多核运作」。

---

## 🚀 快速开始

本项目支持 macOS、Windows、Linux 跨平台一键启动。

```bash
# 进入项目根目录
cd OpenAcosmi-rust+go

# 一键安装依赖、编译并启动全部联邦服务
make start
```

如果你是 **macOS** 或 **Windows** 用户，体验极致简单，直接双击项目根目录下的系统专属启动脚本即可：

- macOS: 双击 `start.command`
- Windows: 双击 `start.bat`

详细的底层开发环境配置与多终端极客启动方式，请参阅 [启动指南 (STARTUP.md)](./STARTUP.md)。

---

<h2 id="english">🇬🇧 English Introduction</h2>

## 🌌 Vision & Breakthrough

**Claw Acosmi** is dedicated to breaking the constraints of traditional AI development frameworks. In the chaos of the "void" (Acosmism), we construct a completely new order from the ground up. This is a truly future-oriented, **multi-modal, hyper-isolated, and natively distributed** complex Agent Federation architecture.

Compared to legacy Node.js/Python script-level Agent frameworks, Claw Acosmi achieves a dimensional strike in **system-level security control**, **memory retrieval efficiency**, and **infinite skill expansibility**.

---

## ✨ Core Capabilities & Architectural Dimensional Strike

By adopting a deep hybrid architecture crossing language boundaries (`Rust` + `Go`), Claw Acosmi has accomplished the following:

### 1. 🛡️ Rock-Solid: Self-Developed Rust Native Sandbox

We did not compromise with coarse-grained Docker containers. At the lowest level, Acosmi uses our **fully self-developed `oa-sandbox` engine**, leveraging Rust to achieve true Operating System (OS)-level process isolation.

- **Native Cross-Platform Support**: Deeply integrates with macOS (Seatbelt FFI), Linux (Landlock/Seccomp/Namespaces), and Windows (Restricted Token).
- **Microsecond Startup**: Discarding heavy full-virtualization OS startups, AI-generated unknown code and commands are executed with microsecond latency within an extremely secure "Jail" environment.
- *[Dimensional Strike]*: Even if the AI generates destructive or privilege-escalating malicious shellcode, it will be mercilessly intercepted at the native OS kernel boundary.

### 2. 🧠 Unforgettable: Distributed Memory System

Abandoning monolithic JSON files and rough local SQLite for handling massive context interactions.

- Introduces the **Ultra-High Dimensional Vector Storage and Retrieval Engine (UHMS)** (incorporating retrieval concepts inspired by ByteDance's tech stack).
- Employs cutting-edge Full-Text Search (FTS5) combined with multi-dimensional Embedding hybrid search. This ensures the Agent never loses context, possessing a deep, long-term memory akin to humans.
- *[Dimensional Strike]*: When navigating codebases with hundreds of thousands of tokens or complex document libraries, the system still supports millisecond-level precise context recall, never losing the chain of thought.

### 3. 🌳 Boundless Growth: Skill Tree & Tool Tree

Acosmi discards the industry-mainstream flat Tool mounting mechanism, introducing a highly vital **dual-track architecture for Skills and Tools**.

- **Skill Tree**: Top-down guidance for the Agent's way of thinking and macro-strategy (e.g., a complete advanced code refactoring paradigm, or deep security vulnerability review logic).
- **Tool Tree**: Bottom-up empowerment of atomic abilities for the Agent to interact physically with the real world (e.g., seamlessly executing bash, accurately scraping web pages, taking screenshots).
- They support hot-swapping, dynamic dependency injection, and tightly integrate with the MCP (Model Context Protocol), achieving unbounded expansion for the Agent.

### 4. 🪆 Dawn of Chaos: The Vast Swarm of Sub-Agents

Claw Acosmi is no longer a lone thinker.

- **Visual Perception Agent (Argus)**: A visual nexus built on Go+Rust, capable of staring directly at the user's desktop and perceiving the stream of screen changes, granting the system true "vision" and pixel-level multi-modal stream parsing abilities.
- **More Core Sub-Agents (Incoming)**: Agents like Swabble and Acosmo-Code will be connected sequentially. Within secure communication zones delineated by the sandbox, these sub-agents perform large-scale parallel work, mutual gaming, and collaborative decision-making.
- *[Dimensional Strike]*: Evolving from single LLM pure text processing to an omnipotent Agent Legion equipped with **perception, inference, code generation, and visual auditing**, achieving true "distributed multi-core operation" in complex workflows.

---

## 🚀 Quick Start

This project supports cross-platform one-click startup on macOS, Windows, and Linux.

```bash
# Navigate to the project root
cd OpenAcosmi-rust+go

# One-click install dependencies, compile, and start all federation services
make start
```

If you are a **macOS** or **Windows** user, experience extreme simplicity by just double-clicking the OS-specific startup script in the root directory:

- macOS: Double-click `start.command`
- Windows: Double-click `start.bat`

For detailed developer environments and multi-terminal startup methods, please refer to the [Startup Guide (STARTUP.md)](./STARTUP.md).

---

## 🤝 Contributing to Acosmi

Establishing order from chaos requires the strength of every developer.
Welcome to submit Issues and Pull Requests to explore the ultimate form of Large Language Model systems with us.

## 📄 License

This project is licensed under the [MIT License](LICENSE).

## 🤝 参与构筑太虚

在混沌中建立秩序离不开每一位开发者的力量。
欢迎提交 Issue 和 Pull Request，与我们一同探索大语言模型系统的最终形态。

## 📄 许可证

本项目采用 [MIT 许可证](LICENSE) 开源。
