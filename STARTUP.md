# Crab Claw（蟹爪）启动指南

> 展示品牌与仓库目录已统一为 **Crab Claw（蟹爪）/ CrabClaw**；CLI 命令和状态目录仍保留 `openacosmi` 兼容标识。

## 项目结构

```
CrabClaw/
├── 一键部署脚本/            # ⚡ 跨平台一键启动脚本
│   ├── mac-start.command  # macOS 双击启动
│   ├── win-start.bat      # Windows 双击启动
│   └── linux-start.sh     # 核心启动脚本 (macOS/Linux)
├── Makefile               # 根级构建系统 (make start)
├── backend/               # Go Gateway (acosmi)
├── cli-rust/              # Rust CLI (openacosmi)
├── ui/                    # 前端控制面板 (Vite + Lit)
├── Argus/                 # 视觉子智能体
│   └── go-sensory/        # Go 感知服务 (MCP Server)
├── browser-extension/     # Chrome 浏览器扩展
├── coder/                 # Coder 子智能体 (预留)
├── Swabble/               # Swabble 子智能体 (预留)
└── docs/                  # 文档
```

---

## ✨ 核心优势

### 🔒 Rust 原生沙箱 — 三平台全覆盖

自研 `oa-sandbox` 沙箱引擎，使用 Rust 实现 OS 级进程隔离，**同时支持 macOS、Linux、Windows 三大操作系统**，无需 Docker 即可安全执行 AI 生成的代码。

| 平台 | 主要隔离技术 | 降级方案 |
|------|-------------|---------|
| **macOS** | Seatbelt FFI (`sandbox_init_with_parameters`) | Docker |
| **Linux** | Landlock + Seccomp + Namespaces + Cgroups | Docker |
| **Windows** | Restricted Token + Job Object + ACL | Docker |

**自动降级**：每个平台都有多层防线，若原生隔离不可用，Gateway 代码会自动回退至 Docker 容器方案（需本地安装 Docker），确保沙箱始终可用。

> [!NOTE]
> **注意事项**
>
> - **macOS**：需要 Xcode Command Line Tools，以支持 Seatbelt FFI 编译
> - **Linux**：需要 `libseccomp-dev >= 2.5.0`；Landlock 需要内核 5.13+；完整 Namespace 隔离需要非特权用户命名空间支持
> - **Windows**：Restricted Token 方案参考 Chromium 渲染进程隔离设计；AppContainer 因兼容性问题默认不启用
> - **Docker 降级**：当原生后端不可用时自动使用 Docker，需确保 Docker 已安装且当前用户有运行权限
> - 沙箱代码中包含 `unsafe` FFI 调用（用于平台 API 绑定），均附有 `// SAFETY:` 注释并通过审计

---

## ⚡ 一键启动（推荐）

从 Git 克隆后，一键完成：**环境检查 → 依赖安装 → 编译 → 启动服务 → 自动打开浏览器**。

| 平台 | 方式 | 说明 |
|------|------|------|
| **macOS** | 双击 `一键部署脚本/mac-start.command` | Terminal.app 自动打开并执行 |
| **Windows** | 双击 `一键部署脚本/win-start.bat` | CMD 窗口自动打开并执行 |
| **Linux** | `./一键部署脚本/linux-start.sh` | 终端直接运行 |
| **所有平台** | `make start` | 终端运行（需先安装 make） |

启动流程：

```
环境检查 (Go/Node/npm)
  ↓
首次自动 npm install
  ↓
编译 Gateway
  ↓
后台启动 Gateway (:19001) + 前端 UI (:26222)
  ↓
轮询等待就绪 → 自动打开浏览器
  ↓
Ctrl+C 优雅关闭所有服务
```

> Argus 视觉子智能体因编译耗时较长，不包含在一键启动中。如已构建，Gateway 会自动发现并连接。

---

## 🚀 根目录命令速查

> 所有命令均在项目根目录 `CrabClaw/` 下执行，无需 `cd` 到子目录。

```bash
# 进入项目根目录
cd ~/Desktop/CrabClaw
```

| 命令 | 说明 |
|------|------|
| `make start` | ⚡ **一键启动**（编译 + 启动全部服务 + 打开浏览器） |
| `make dev` | 完整编译 Argus + 启动 Gateway（dev mode） |
| `make gateway` | 编译 + 启动 Gateway（需 Argus 已构建） |
| `make argus` | 编译 Argus 全量（Rust FFI + Go + .app 打包 + 签名） |
| `make ui` | 安装依赖 + 启动前端开发服务器（port 26222） |
| `make build` | 编译全部组件（Argus + Gateway + CLI，不启动） |
| `make cli` | 编译 Rust CLI（release） |
| `make test` | 运行全部测试（Go + Argus + Rust CLI） |
| `make lint` | 静态分析（Go + Argus + Rust CLI） |
| `make clean` | 清理全部构建产物 |
| `make check-env` | 检查开发环境（Go/Rust/Node/证书） |
| `make help` | 显示所有可用命令 |

```bash
# 典型开发流程
make check-env   # 首次：检查环境
make start       # 一键启动全部服务
# 或分步：
make argus       # 编译 Argus（首次 / Rust 代码变更后）
make gateway     # 启动 Gateway
make ui          # 另一终端启动前端
```

---

## 手动启动（三终端）

> 如需分别控制各服务的启动和日志，可使用以下手动方式。

### 终端 1 — Go Gateway

```bash
cd backend
make gateway-dev
```

Gateway 以 dev 模式启动在 `localhost:19001`。

| 命令 | 说明 |
|------|------|
| `make gateway` | 仅编译，不启动 |
| `make gateway-run` | 编译并启动（默认端口） |
| `make gateway-dev` | 编译并启动（dev profile, 端口 19001） |

### 终端 2 — 前端 UI

```bash
cd ui
npm install   # 首次需要安装依赖
npm run dev
```

Vite 开发服务器启动在 `http://localhost:26222`，WebSocket 自动代理到 Gateway。

### 终端 3 — Argus 视觉子智能体

Argus 是独立的视觉理解/执行子智能体，通过 MCP 协议（JSON-RPC 2.0 stdio）与 Gateway 通信。

#### 方式 A：.app bundle（推荐 — macOS 授权持久化）

```bash
cd Argus
make app
```

构建产物：`Argus/build/Argus.app`。Gateway 启动时自动发现并连接。

> .app bundle 使用 `CFBundleIdentifier` 追踪 TCC 授权，`go build` 产生新哈希不影响屏幕捕捉等权限。

#### 方式 B：裸二进制（开发调试用）

```bash
# 1. 先构建 Rust 核心库
cd Argus/rust-core && cargo build --release

# 2. 构建 Go 感知服务
cd Argus/go-sensory && go build -o /tmp/argus-sensory ./cmd/server

# 3. 设置环境变量后启动 Gateway
export ARGUS_BINARY_PATH=/tmp/argus-sensory
cd backend && 
```

Gateway 启动时会自动为裸二进制签名（需 Keychain 中有 "Argus Dev" 证书）。

#### 方式 C：单独运行（不走 Gateway 自动管理）

```bash
# MCP Server 模式（stdin/stdout JSON-RPC）
cd Argus/go-sensory && go run ./cmd/server -mcp
```

#### 首次设置：创建开发签名证书

```bash
cd Argus && ./scripts/package/create-dev-cert.sh
```

> 在 Keychain 中创建 "Argus Dev" 自签名证书，用于代码签名。只需执行一次。

#### 验证 Argus 连接状态

Gateway 启动后，观察日志中是否出现：

- `argus: ready, tools=N` — 连接成功，发现 N 个工具
- `argus: using .app bundle binary` — 使用方式 A
- `argus: signing bare binary` — 方式 B 自动签名

#### Argus 路径解析优先级

Gateway 按以下顺序查找 Argus 二进制：

1. `$ARGUS_BINARY_PATH` 环境变量
2. `.app bundle` 搜索（monorepo build/ → /Applications/ → ~/Applications/ → ~/.openacosmi/）
3. `~/.openacosmi/bin/argus-sensory`
4. `$PATH` 中的 `argus-sensory`

---

## 其他常用命令

### 测试

```bash
# Go Gateway 测试
cd backend && make test

# Rust CLI 测试
cd backend && make test-rust

# 全部测试（Go + Rust）
cd backend && make test-all

# 前端测试
cd ui && npm test

# Argus 测试
cd Argus && make test

# Argus Rust 静态分析
cd Argus && make lint
```

### Argus 完整构建

```bash
cd Argus
make build    # Rust + Go 全量编译
make app      # 打包 .app bundle（含签名）
make pkg      # 生成 .pkg 安装包
make console  # 构建 Wails 桌面控制台
```

### Rust CLI

```bash
cd backend && make cli            # 编译 Rust CLI (release)
cd backend && make install-cli    # 安装到 ~/.local/bin
```

### 代码质量

```bash
cd backend
make fmt             # 格式化
make vet             # 静态分析
make lint            # 完整 lint（需 golangci-lint）
```

### 清理

```bash
cd backend && make clean    # 清理 Gateway 构建产物
cd Argus && make clean      # 清理 Argus 构建产物
```

---

## 环境要求

**通用依赖：**

- Go 1.25.7+（`go.mod` 最低要求）— [下载](https://go.dev/dl/)
- Rust 1.85+（`Cargo.toml` MSRV）— [下载](https://rustup.rs/)（可选，Argus 编译需要）
- Node.js 18+ / npm — [下载](https://nodejs.org/)

**macOS：**

- Xcode Command Line Tools（Argus 代码签名 + Rust FFI 编译需要）
- "Argus Dev" 签名证书（首次运行 `create-dev-cert.sh`）

**Linux：**

- `libseccomp-dev >= 2.5.0`（oa-sandbox seccomp 支持）
- 内核 5.13+（Landlock 沙箱支持）

**Windows：**

- [Visual Studio Build Tools](https://visualstudio.microsoft.com/visual-cpp-build-tools/) 或完整 Visual Studio（安装时勾选「使用 C++ 的桌面开发」），Rust 编译 oa-sandbox 需要 MSVC 工具链
- curl（Win10 1803+ 系统自带，`start.bat` 用于探测服务就绪状态）
- Argus 视觉子智能体当前仅支持 macOS，Windows 上不可用（不影响其他功能）
- 沙箱使用 Restricted Token + Job Object 方案，无需额外安装
