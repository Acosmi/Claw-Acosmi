#!/usr/bin/env bash
# ============================================================================
# CrabClaw 一键启动脚本
#
# 功能：环境检查 → 缺失依赖自动安装 → 编译 Gateway → 启动服务 → 打开向导页
# 适用：macOS / Linux
# ============================================================================

set -euo pipefail

# ===== 常量 =====
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GATEWAY_PORT=19001
UI_PORT=26222
UI_URL="http://localhost:${UI_PORT}"
GATEWAY_URL="http://localhost:${GATEWAY_PORT}"
WIZARD_URL="${UI_URL}?onboarding=1"
MAX_WAIT=60  # 最长等待秒数

# 子进程 PID 跟踪
GATEWAY_PID=""
UI_PID=""

# ===== 颜色 =====
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ===== 工具函数 =====
info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[OK]${NC}   $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERR]${NC}  $*"; }

banner() {
    echo -e "${CYAN}${BOLD}"
    echo "================================================"
    echo "          CrabClaw  一键启动"
    echo "================================================"
    echo -e "${NC}"
}

# ===== 清理函数 =====
cleanup() {
    echo ""
    info "正在停止所有服务..."

    if [ -n "$UI_PID" ] && kill -0 "$UI_PID" 2>/dev/null; then
        kill "$UI_PID" 2>/dev/null || true
        wait "$UI_PID" 2>/dev/null || true
        success "前端 UI 已停止"
    fi

    if [ -n "$GATEWAY_PID" ] && kill -0 "$GATEWAY_PID" 2>/dev/null; then
        kill "$GATEWAY_PID" 2>/dev/null || true
        wait "$GATEWAY_PID" 2>/dev/null || true
        success "Gateway 已停止"
    fi

    success "所有服务已关闭，再见！"
    exit 0
}

trap cleanup SIGINT SIGTERM EXIT

# ===== 打开浏览器（跨平台） =====
open_browser() {
    local url="$1"
    if command -v open &>/dev/null; then
        open "$url"                     # macOS
    elif command -v xdg-open &>/dev/null; then
        xdg-open "$url"                 # Linux
    elif command -v wslview &>/dev/null; then
        wslview "$url"                  # WSL
    else
        warn "无法自动打开浏览器，请手动访问: $url"
    fi
}

# ===== 平台检测 =====
OS_TYPE=""
PKG_MGR=""

detect_platform() {
    case "$(uname -s)" in
        Darwin) OS_TYPE="macos" ;;
        Linux)  OS_TYPE="linux" ;;
        *)
            error "不支持的操作系统: $(uname -s)"
            exit 1
            ;;
    esac

    if [ "$OS_TYPE" = "macos" ]; then
        if command -v brew &>/dev/null; then
            PKG_MGR="brew"
        else
            PKG_MGR="none"
        fi
    else
        if command -v apt-get &>/dev/null; then
            PKG_MGR="apt"
        elif command -v dnf &>/dev/null; then
            PKG_MGR="dnf"
        elif command -v yum &>/dev/null; then
            PKG_MGR="yum"
        elif command -v pacman &>/dev/null; then
            PKG_MGR="pacman"
        else
            PKG_MGR="none"
        fi
    fi
}

# ===== 用户确认（默认 yes） =====
confirm() {
    local msg="$1"
    if [ -t 0 ]; then
        echo -en "${YELLOW}${msg} [Y/n] ${NC}"
        read -r answer
        case "$answer" in
            [nN]*) return 1 ;;
            *)     return 0 ;;
        esac
    fi
    # 非交互模式默认 yes
    return 0
}

# ===== macOS: 确保 Homebrew 可用 =====
ensure_brew() {
    if [ "$OS_TYPE" != "macos" ]; then return 0; fi
    if command -v brew &>/dev/null; then
        PKG_MGR="brew"
        return 0
    fi

    info "Homebrew 未安装，正在安装（macOS 包管理器）..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

    # Apple Silicon vs Intel 路径
    if [ -f /opt/homebrew/bin/brew ]; then
        eval "$(/opt/homebrew/bin/brew shellenv)"
    elif [ -f /usr/local/bin/brew ]; then
        eval "$(/usr/local/bin/brew shellenv)"
    fi

    if command -v brew &>/dev/null; then
        PKG_MGR="brew"
        success "Homebrew 安装成功"
    else
        error "Homebrew 安装失败，请手动安装: https://brew.sh"
        exit 1
    fi
}

# ===== 通用安装函数 =====
pkg_install() {
    local pkg_name="$1"
    local display_name="${2:-$pkg_name}"

    info "正在安装 ${display_name}..."

    case "$PKG_MGR" in
        brew)
            brew install "$pkg_name"
            ;;
        apt)
            sudo apt-get update -qq && sudo apt-get install -y -qq "$pkg_name"
            ;;
        dnf)
            sudo dnf install -y "$pkg_name"
            ;;
        yum)
            sudo yum install -y "$pkg_name"
            ;;
        pacman)
            sudo pacman -S --noconfirm "$pkg_name"
            ;;
        *)
            error "无法自动安装 ${display_name}（未找到包管理器）"
            return 1
            ;;
    esac
}

# ===== 单项依赖检查 + 自动安装 =====

try_install_go() {
    if ! confirm "Go 未安装，是否自动安装？"; then
        error "Go 是必需依赖，请手动安装: https://go.dev/dl/"
        return 1
    fi
    if [ "$OS_TYPE" = "macos" ]; then
        ensure_brew
        pkg_install "go" "Go"
    else
        # Linux: golang 包名因发行版而异
        case "$PKG_MGR" in
            apt)     pkg_install "golang-go" "Go" ;;
            dnf|yum) pkg_install "golang" "Go" ;;
            pacman)  pkg_install "go" "Go" ;;
            *)
                error "请手动安装 Go: https://go.dev/dl/"
                return 1
                ;;
        esac
    fi

    # 刷新 PATH（某些安装方式需要）
    hash -r 2>/dev/null || true
    if command -v go &>/dev/null; then
        success "Go 安装成功: $(go version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)"
    else
        error "Go 安装后未在 PATH 中找到，请检查环境变量"
        return 1
    fi
}

try_install_node() {
    if ! confirm "Node.js 未安装，是否自动安装？"; then
        error "Node.js 是必需依赖，请手动安装: https://nodejs.org/"
        return 1
    fi
    if [ "$OS_TYPE" = "macos" ]; then
        ensure_brew
        pkg_install "node" "Node.js"
    else
        case "$PKG_MGR" in
            apt)
                # 使用 NodeSource 安装 Node.js 22 LTS
                if ! command -v curl &>/dev/null; then
                    sudo apt-get update -qq && sudo apt-get install -y -qq curl
                fi
                info "添加 NodeSource 仓库 (Node.js 22)..."
                curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
                sudo apt-get install -y -qq nodejs
                ;;
            dnf|yum)
                if ! command -v curl &>/dev/null; then
                    sudo "$PKG_MGR" install -y curl
                fi
                curl -fsSL https://rpm.nodesource.com/setup_22.x | sudo bash -
                sudo "$PKG_MGR" install -y nodejs
                ;;
            pacman) pkg_install "nodejs-lts-jod" "Node.js 22 LTS" ;;
            *)
                error "请手动安装 Node.js: https://nodejs.org/"
                return 1
                ;;
        esac
    fi

    hash -r 2>/dev/null || true
    if command -v node &>/dev/null; then
        success "Node.js 安装成功: $(node --version)"
    else
        error "Node.js 安装后未在 PATH 中找到"
        return 1
    fi
}

try_install_make() {
    if ! confirm "make 未安装，是否自动安装？"; then
        error "make 是必需依赖"
        return 1
    fi
    if [ "$OS_TYPE" = "macos" ]; then
        info "安装 Xcode Command Line Tools（含 make）..."
        # 非交互检查: 如果 CLT 已装但 make 不在 PATH，尝试 brew
        if xcode-select -p &>/dev/null; then
            warn "Xcode CLT 已安装但 make 未找到，尝试通过 Homebrew 安装..."
            ensure_brew
            pkg_install "make" "make"
        else
            xcode-select --install 2>/dev/null || true
            # xcode-select --install 是异步 GUI 弹窗，需要等待用户操作
            echo ""
            warn "请在弹出的对话框中点击「安装」，安装完成后按回车继续..."
            read -r
        fi
    else
        case "$PKG_MGR" in
            apt)    pkg_install "build-essential" "build-essential (含 make)" ;;
            dnf)    pkg_install "make" "make" ;;
            yum)    sudo yum groupinstall -y "Development Tools" ;;
            pacman) pkg_install "make" "make" ;;
            *)
                error "请手动安装 make"
                return 1
                ;;
        esac
    fi

    hash -r 2>/dev/null || true
    if command -v make &>/dev/null; then
        success "make 安装成功"
    else
        error "make 安装后未在 PATH 中找到"
        return 1
    fi
}

try_install_curl() {
    if ! confirm "curl 未安装，是否自动安装？"; then
        error "curl 是必需依赖"
        return 1
    fi
    # macOS 自带 curl，理论上不会走到这里
    if [ "$OS_TYPE" = "macos" ]; then
        ensure_brew
        pkg_install "curl" "curl"
    else
        case "$PKG_MGR" in
            apt)    pkg_install "curl" "curl" ;;
            dnf)    pkg_install "curl" "curl" ;;
            yum)    pkg_install "curl" "curl" ;;
            pacman) pkg_install "curl" "curl" ;;
            *)
                error "请手动安装 curl"
                return 1
                ;;
        esac
    fi

    hash -r 2>/dev/null || true
    if command -v curl &>/dev/null; then
        success "curl 安装成功"
    else
        error "curl 安装后未在 PATH 中找到"
        return 1
    fi
}

# ===== 端口检查 =====
check_port() {
    local port="$1"
    local name="$2"
    if lsof -iTCP:"$port" -sTCP:LISTEN -t &>/dev/null; then
        local pids
        pids=$(lsof -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null | head -3 | tr '\n' ' ')
        error "端口 ${port} 已被占用 (${name})，占用进程 PID: ${pids}"
        warn "请先停止占用进程: kill ${pids}"
        return 1
    fi
    return 0
}

check_ports() {
    info "检查端口可用性..."
    local conflict=0
    check_port "$GATEWAY_PORT" "Gateway" || conflict=1
    check_port "$UI_PORT" "前端 UI"      || conflict=1
    if [ "$conflict" -ne 0 ]; then
        error "存在端口冲突，请先释放端口后重试。"
        exit 1
    fi
    success "端口 ${GATEWAY_PORT} (Gateway) 和 ${UI_PORT} (UI) 均可用"
    echo ""
}

# ===== 1. 环境检查 + 自动安装 =====
check_env() {
    info "检查开发环境..."
    local fatal=0

    detect_platform
    info "平台: ${OS_TYPE}, 包管理器: ${PKG_MGR}"
    echo ""

    # --- curl（后续安装可能需要） ---
    if command -v curl &>/dev/null; then
        success "curl $(curl --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9.]+' | head -1)"
    else
        warn "curl 未安装"
        try_install_curl || fatal=1
    fi

    # --- make ---
    if command -v make &>/dev/null; then
        success "make $(make --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9.]+' | head -1)"
    else
        warn "make 未安装"
        try_install_make || fatal=1
    fi

    # --- Go ---
    if command -v go &>/dev/null; then
        success "Go $(go version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)"
    else
        warn "Go 未安装"
        try_install_go || fatal=1
    fi

    # --- Node.js ---
    if command -v node &>/dev/null; then
        success "Node.js $(node --version)"
    else
        warn "Node.js 未安装"
        try_install_node || fatal=1
    fi

    # --- npm（随 Node.js 安装） ---
    if command -v npm &>/dev/null; then
        success "npm $(npm --version)"
    else
        if command -v node &>/dev/null; then
            error "npm 未找到（但 Node.js 已安装）。请重新安装 Node.js。"
            fatal=1
        fi
        # 如果 node 也没有，上面 try_install_node 已经处理
    fi

    # --- Rust（可选） ---
    if command -v rustc &>/dev/null; then
        success "Rust $(rustc --version | awk '{print $2}') (Argus 编译需要)"
    else
        warn "Rust 未安装 -- Argus 视觉子智能体不可用（可选）"
    fi

    echo ""

    if [ "$fatal" -ne 0 ]; then
        error "部分必需依赖安装失败，无法继续。"
        exit 1
    fi

    success "所有必需依赖就绪！"
    echo ""
}

# ===== 2. 安装前端依赖 =====
install_ui_deps() {
    local need_install=0
    if [ ! -d "$PROJECT_DIR/ui/node_modules" ]; then
        need_install=1
    elif [ -f "$PROJECT_DIR/ui/package-lock.json" ] && \
         [ "$PROJECT_DIR/ui/package-lock.json" -nt "$PROJECT_DIR/ui/node_modules/.package-lock.json" ]; then
        need_install=1
    fi
    if [ "$need_install" -eq 1 ]; then
        info "安装/更新前端依赖 (npm install)..."
        cd "$PROJECT_DIR/ui" && npm install
        echo ""
    fi
}

# ===== 3. 编译 Gateway =====
build_gateway() {
    info "编译 Gateway..."
    cd "$PROJECT_DIR/backend" && make gateway 2>&1
    if [ ! -f "$PROJECT_DIR/backend/build/acosmi" ]; then
        error "Gateway 编译失败"
        exit 1
    fi
    success "Gateway 编译完成"
    echo ""
}

# ===== 4. 检查 Argus 状态 =====
check_argus() {
    local argus_found=0
    if [ -f "$PROJECT_DIR/Argus/build/Argus.app/Contents/MacOS/argus-sensory" ]; then
        argus_found=1
    elif [ -f "$PROJECT_DIR/Argus/build/argus-sensory" ]; then
        argus_found=1
    fi

    if [ "$argus_found" -eq 1 ]; then
        success "Argus 已构建 -- 视觉子智能体可用"
    else
        warn "Argus 未构建 -- 视觉子智能体不可用"
        warn "如需 Argus，请在另一终端运行: cd Argus && make app"
    fi
    echo ""
}

# ===== 5. 启动 Gateway =====
start_gateway() {
    info "启动 Gateway (port ${GATEWAY_PORT})..."
    cd "$PROJECT_DIR/backend"
    ./build/acosmi -dev &
    GATEWAY_PID=$!

    local i=0
    while [ "$i" -lt 3 ]; do
        if ! kill -0 "$GATEWAY_PID" 2>/dev/null; then
            error "Gateway 进程启动失败（退出码可能非零）"
            exit 1
        fi
        sleep 1
        i=$((i + 1))
    done
    success "Gateway PID: ${GATEWAY_PID}"
}

# ===== 6. 启动前端 UI =====
start_ui() {
    info "启动前端 UI (port ${UI_PORT})..."
    cd "$PROJECT_DIR/ui"
    npm run dev &
    UI_PID=$!
    success "前端 UI PID: ${UI_PID}"
    echo ""
}

# ===== 7. 等待服务就绪 =====
wait_for_ready() {
    info "等待服务就绪..."
    local elapsed=0
    local gateway_ready=0
    local ui_ready=0

    while [ "$elapsed" -lt "$MAX_WAIT" ]; do
        if [ -n "$GATEWAY_PID" ] && ! kill -0 "$GATEWAY_PID" 2>/dev/null; then
            error "Gateway 进程意外退出"
            exit 1
        fi
        if [ -n "$UI_PID" ] && ! kill -0 "$UI_PID" 2>/dev/null; then
            error "前端 UI 进程意外退出"
            exit 1
        fi

        if [ "$gateway_ready" -eq 0 ] && curl -s -o /dev/null --max-time 2 "$GATEWAY_URL" 2>/dev/null; then
            gateway_ready=1
            success "Gateway 已就绪 (${GATEWAY_URL})"
        fi

        if [ "$ui_ready" -eq 0 ] && curl -s -o /dev/null --max-time 2 "$UI_URL" 2>/dev/null; then
            ui_ready=1
            success "前端 UI 已就绪 (${UI_URL})"
        fi

        if [ "$gateway_ready" -eq 1 ] && [ "$ui_ready" -eq 1 ]; then
            echo ""
            success "所有服务已就绪！"
            return 0
        fi

        printf "."
        sleep 1
        elapsed=$((elapsed + 1))
    done

    echo ""
    if [ "$gateway_ready" -eq 0 ]; then
        error "Gateway 未在 ${MAX_WAIT}s 内就绪: ${GATEWAY_URL}"
    fi
    if [ "$ui_ready" -eq 0 ]; then
        error "前端 UI 未在 ${MAX_WAIT}s 内就绪: ${UI_URL}"
    fi
    warn "部分服务未就绪，请手动检查上述地址"
}

# ===== 主流程 =====
main() {
    banner
    check_env
    check_ports
    install_ui_deps
    build_gateway
    check_argus
    start_gateway
    start_ui
    wait_for_ready

    # 打开浏览器 -> 自动进入配置向导页
    info "正在打开浏览器（进入配置向导）..."
    open_browser "$WIZARD_URL"

    echo ""
    echo -e "${CYAN}${BOLD}================================================${NC}"
    echo -e "  ${GREEN}Gateway${NC}:    ${GATEWAY_URL}"
    echo -e "  ${GREEN}前端 UI${NC}:    ${UI_URL}"
    echo -e "  ${GREEN}配置向导${NC}:   ${WIZARD_URL}"
    echo -e "  ${YELLOW}按 Ctrl+C 停止所有服务${NC}"
    echo -e "${CYAN}${BOLD}================================================${NC}"
    echo ""

    # 前台等待（不退出）
    wait
}

main "$@"
