# Crab Claw（蟹爪）— 构建系统
#
# 架构: Rust CLI (crabclaw，兼容别名 openacosmi) + Go Gateway (acosmi)
# 参见 docs/adr/001-rust-cli-go-gateway.md
#
# 用法:
#   make gateway        # 编译 Go Gateway
#   make cli            # 编译 Rust CLI (release)
#   make build-all      # 编译 Gateway + CLI + C++
#   make test           # 运行全部测试
#   make lint           # 静态分析
#   make clean          # 清理构建产物

.PHONY: gateway cli build-all build-cpp test test-multimedia report-multimedia lint clean fmt vet help \
        gateway-dev gateway-run test-rust install-cli gateway-ffi \
        gen-frontend native-host install-native-host

# 版本信息
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# 目录
CPP_BUILD_DIR := build/cpp
GATEWAY_BIN := build/acosmi
RUST_FFI_WORKSPACE := ../cli-rust/libs/openviking-rs

# --- 默认目标 ---
help: ## 显示帮助信息
	@echo "Crab Claw（蟹爪）构建命令："
	@echo ""
	@echo "  架构: Rust CLI (crabclaw，兼容别名 openacosmi) + Go Gateway (acosmi)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""

# --- Go Gateway 构建 ---
gateway-ffi: ## 编译 Gateway 所需的 Rust FFI (openviking-ffi)
ifeq ($(CGO_ENABLED),0)
	@echo "ℹ️  CGO_ENABLED=0，跳过 Rust FFI 编译，使用纯 Go fallback"
else
	@echo ""
	@echo "🦀 ══════════════════════════════════════════════════════════════"
	@echo "🦀  编译 Rust FFI (openviking-ffi)"
	@echo "🦀  ⏱  首次编译约 5-10 分钟（含 qdrant 向量引擎），请耐心等待"
	@echo "🦀  💡 后续增量编译仅需数秒"
	@echo "🦀 ══════════════════════════════════════════════════════════════"
	@echo ""
	@START_TIME=$$(date +%s); \
	cd $(RUST_FFI_WORKSPACE) && cargo build -p openviking-ffi --release; \
	END_TIME=$$(date +%s); \
	ELAPSED=$$((END_TIME - START_TIME)); \
	echo ""; \
	echo "✅ Rust FFI 编译完成（耗时 $${ELAPSED}s）"
endif

gateway: gateway-ffi fmt vet ## 编译 Go Gateway (acosmi)
	@echo "🔨 编译 Go Gateway..."
	go build $(LDFLAGS) -o $(GATEWAY_BIN) ./cmd/acosmi
	@echo "✅ 编译完成: $(GATEWAY_BIN)"

gateway-run: gateway ## 编译并启动 Gateway
	./$(GATEWAY_BIN)

gateway-dev: gateway ## 编译并启动 Gateway (dev profile)
	./$(GATEWAY_BIN) -dev -port 19001

# --- Rust CLI 构建 ---
cli: ## 编译 Rust CLI (release)
	@echo "🦀 编译 Rust CLI..."
	cd ../cli-rust && cargo build --workspace --release
	@echo "✅ Rust CLI 编译完成: ../cli-rust/target/release/{crabclaw,openacosmi}"

test-rust: ## 运行 Rust CLI 测试
	cd ../cli-rust && cargo test --workspace

install-cli: cli ## 安装 Rust CLI 到 ~/.local/bin
	@mkdir -p $(HOME)/.local/bin
	cp ../cli-rust/target/release/crabclaw $(HOME)/.local/bin/crabclaw
	cp ../cli-rust/target/release/openacosmi $(HOME)/.local/bin/openacosmi
	@echo "✅ Rust CLI 已安装:"
	@echo "   - $(HOME)/.local/bin/crabclaw"
	@echo "   - $(HOME)/.local/bin/openacosmi (兼容别名)"

# --- C++ 构建 ---
build-cpp: ## 编译 C++ 库
	@echo "🔨 编译 C++ 库..."
	@mkdir -p $(CPP_BUILD_DIR)
	cd $(CPP_BUILD_DIR) && cmake ../.. -DCMAKE_BUILD_TYPE=Release && make -j$$(nproc 2>/dev/null || sysctl -n hw.ncpu)
	@echo "✅ C++ 库编译完成"

# --- Native Messaging Host ---
NATIVE_HOST_BIN := build/crabclaw-native-host

native-host: fmt vet ## 编译 Chrome Native Messaging Host
	@echo "🔨 编译 Native Messaging Host..."
	go build $(LDFLAGS) -o $(NATIVE_HOST_BIN) ./cmd/native-host
	@echo "✅ 编译完成: $(NATIVE_HOST_BIN)"

install-native-host: native-host ## 安装 Native Messaging Host 到 Chrome
	./$(NATIVE_HOST_BIN) --install
	@echo "✅ Native Messaging Host 已安装"

# --- 完整构建 ---
build-all: gateway cli native-host ## 编译 Gateway + Rust CLI + Native Host
	@echo "✅ 全部编译完成"

# --- 测试 ---
test: ## 运行全部 Go 测试
	@echo "🧪 运行测试..."
	go test -race -cover ./...
	@echo "✅ 测试通过"

test-multimedia: ## 运行多媒体关键回归测试（飞书/钉钉/企微/网关管道）
	@echo "🧪 运行多媒体关键回归..."
	go test ./internal/channels/feishu ./internal/channels/dingtalk ./internal/channels/wecom ./internal/channels ./internal/agents/runner ./internal/gateway -run 'TestFeishuSendMessage_|TestWeComSendMessage_|TestWecomMediaURLFallbackText|TestDingTalkSendMessage_|TestDingTalkUploadHelpers|TestBuildDingTalkImageMsgParam_|TestSendErrorExportedAccessors|TestMapSendErrorToShape|TestHandleSend_|TestHandlePoll_|TestProcessFeishuMessage_|TestProcessAttachmentsForChat_|TestSendMedia_ChannelSendError_UsesStructuredMessage|TestBuildPayloads_|TestExtractMediaListFromReplies_|TestHandleRemoteApprovalTest_|TestRemoteApprovalNotifier_TestProvider_'
	@echo "✅ 多媒体关键回归通过"

report-multimedia: ## 生成多媒体回归报告（写入 docs/codex）
	@../scripts/generate_multimedia_regression_report.sh

test-verbose: ## 运行测试（详细输出）
	go test -race -cover -v ./...

test-all: test test-rust ## 运行 Go + Rust 全部测试

# --- 前端生成 (Phase 2 D6/D7/D8) ---
gen-frontend: ## 从能力树生成前端 TS/JSON 产物
	@echo "🔄 从能力树生成前端产物..."
	go run ./cmd/gen-frontend
	@echo "✅ 前端生成完成"

# --- 代码质量 ---
fmt: ## 格式化代码
	@gofmt -w .

vet: ## 静态分析
	go vet ./...

lint: vet ## 完整 lint 检查（需安装 golangci-lint）
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run ./... || echo "⚠️  golangci-lint 未安装，跳过"

# --- 清理 ---
clean: ## 清理构建产物
	@echo "🧹 清理..."
	rm -rf build/
	@echo "✅ 清理完成"

# --- 开发工具 ---
dev: ## 开发模式（热重载，需安装 air）
	@which air > /dev/null 2>&1 && air || echo "⚠️  air 未安装，使用 make gateway-run"
