# CrabClaw — 根级构建系统
#
# 架构: Go Gateway + Rust CLI + Argus 视觉子智能体 + 前端 UI
#
# 用法:
#   make dev          # 完整开发启动（编译 Argus + Gateway，启动 Gateway）
#   make gateway      # 仅编译 + 启动 Gateway（需 Argus 已构建）
#   make argus        # 编译 Argus（Rust FFI + Go 感知服务 + .app 打包）
#   make ui           # 安装依赖 + 启动前端开发服务器
#   make build        # 编译全部组件（不启动）
#   make test         # 运行全部测试
#   make clean        # 清理全部构建产物

.PHONY: dev gateway argus argus-rust argus-go argus-app ui build test clean \
        gateway-build cli lint help check-env start

# ===== 变量 =====
ARGUS_APP    := Argus/build/Argus.app/Contents/MacOS/argus-sensory
ARGUS_DYLIB  := Argus/rust-core/target/release/libargus_core.dylib
GATEWAY_BIN  := backend/build/acosmi

# ===== 默认目标 =====
help: ## 显示帮助
	@echo "CrabClaw 构建命令:"
	@echo ""
	@echo "  快速启动:"
	@echo "    make start     一键启动（编译 + 启动全部服务 + 打开浏览器）"
	@echo "    make dev       完整编译 + 启动 Gateway（含 Argus 视觉子智能体）"
	@echo "    make gateway   仅编译 + 启动 Gateway（需 Argus 已构建）"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ===== 一键启动（推荐） =====
start: ## 一键启动（编译 + 启动全部服务 + 打开浏览器）
	@./一键部署脚本/linux-start.sh

# ===== 开发启动 =====
dev: argus gateway ## 完整编译 Argus + 启动 Gateway (dev mode)

# ===== Gateway =====
gateway-build: ## 编译 Gateway（不启动）
	@cd backend && make gateway

gateway: gateway-build ## 编译 + 启动 Gateway (dev mode, port 19001)
	@echo ""
	@if [ -f "$(ARGUS_APP)" ]; then \
		echo "✅ Argus binary: $(ARGUS_APP)"; \
	else \
		echo "⚠️  Argus 未构建，视觉子智能体不可用（运行 make argus 构建）"; \
	fi
	@echo ""
	cd backend && ./build/acosmi -dev -port 19001

# ===== Argus 视觉子智能体 =====
argus: argus-app ## 编译 Argus 全量（Rust + Go + .app 打包 + 签名）

argus-rust: ## 编译 Argus Rust 核心库
	@echo "🦀 编译 Argus Rust 核心库..."
	cd Argus/rust-core && cargo build --release
	@echo "✅ $(ARGUS_DYLIB)"

argus-app: argus-rust ## 编译 Go 感知服务 + 打包 .app bundle
	@echo "📦 构建 Argus.app（含 dylib 重链接 + 签名）..."
	cd Argus && make app
	@echo "✅ $(ARGUS_APP)"

# ===== 前端 UI =====
ui: ## 启动前端开发服务器 (Vite, port 26222)
	cd ui && npm install && npm run dev

# ===== Rust CLI =====
cli: ## 编译 Rust CLI (release)
	cd cli-rust && cargo build --workspace --release

# ===== 全量编译 =====
build: argus gateway-build cli ## 编译全部组件（不启动）
	@echo "✅ 全部编译完成"

# ===== 测试 =====
test: ## 运行全部测试
	@echo "🧪 Go Gateway 测试..."
	cd backend && go test -race -cover ./...
	@echo "🧪 Argus 测试..."
	cd Argus && make test
	@echo "🧪 Rust CLI 测试..."
	cd cli-rust && cargo test --workspace
	@echo "✅ 全部测试通过"

# ===== 代码质量 =====
lint: ## 静态分析（Go + Rust）
	cd backend && make lint
	cd Argus && make lint
	cd cli-rust && cargo clippy --workspace -- -D warnings

# ===== 清理 =====
clean: ## 清理全部构建产物
	cd backend && make clean
	cd Argus && make clean
	@echo "✅ 清理完成（Rust CLI target/ 需手动 cd cli-rust && cargo clean）"

# ===== 环境检查 =====
check-env: ## 检查开发环境
	@echo "检查开发环境..."
	@echo -n "  Go:    " && go version 2>/dev/null || echo "❌ 未安装"
	@echo -n "  Rust:  " && rustc --version 2>/dev/null || echo "❌ 未安装"
	@echo -n "  Cargo: " && cargo --version 2>/dev/null || echo "❌ 未安装"
	@echo -n "  Node:  " && node --version 2>/dev/null || echo "❌ 未安装"
	@echo -n "  npm:   " && npm --version 2>/dev/null || echo "❌ 未安装"
	@echo -n "  Xcode CLT: " && xcode-select -p 2>/dev/null || echo "❌ 未安装"
	@echo -n "  Docker: " && docker --version 2>/dev/null || echo "⚠️  未安装（可选）"
	@echo ""
	@echo "签名证书检查:"
	@security find-identity -v -p codesigning 2>/dev/null | grep -q "Argus Dev" \
		&& echo "  ✅ 'Argus Dev' 证书已安装" \
		|| echo "  ⚠️  'Argus Dev' 证书未安装（运行 cd Argus && ./scripts/package/create-dev-cert.sh）"
