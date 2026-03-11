@echo off
chcp 65001 >nul 2>&1
title CrabClaw 快速启动

REM 切换到项目根目录（脚本位于 一键部署脚本/ 子目录）
cd /d "%~dp0.."

echo.
echo ╔══════════════════════════════════════════╗
echo ║          CrabClaw  快速启动              ║
echo ╚══════════════════════════════════════════╝
echo.

REM ===== 环境检查 =====
echo [INFO] 检查开发环境...

set MISSING=0

where go >nul 2>&1
if %errorlevel% equ 0 (
    for /f "tokens=3" %%v in ('go version') do echo   [OK] Go %%v
) else (
    echo   [FAIL] Go 未安装 — 请访问 https://go.dev/dl/ 安装
    set MISSING=1
)

where node >nul 2>&1
if %errorlevel% equ 0 (
    for /f %%v in ('node --version') do echo   [OK] Node.js %%v
) else (
    echo   [FAIL] Node.js 未安装 — 请访问 https://nodejs.org/ 安装
    set MISSING=1
)

where npm >nul 2>&1
if %errorlevel% equ 0 (
    for /f %%v in ('npm --version') do echo   [OK] npm %%v
) else (
    echo   [FAIL] npm 未安装 — 通常随 Node.js 一起安装
    set MISSING=1
)

where rustc >nul 2>&1
if %errorlevel% equ 0 (
    for /f "tokens=2" %%v in ('rustc --version') do echo   [OK] Rust %%v (Argus 编译需要)
) else (
    echo   [WARN] Rust 未安装 — Argus 视觉子智能体不可用（可选）
)

echo.

if %MISSING% equ 1 (
    echo [FAIL] 缺少必要依赖，请先安装后重试。
    pause
    exit /b 1
)

REM ===== 安装前端依赖 =====
if not exist "ui\node_modules" (
    echo [INFO] 首次运行，安装前端依赖...
    cd ui && npm install && cd ..
    echo.
)

REM ===== 编译 Gateway =====
echo [INFO] 编译 Gateway...
cd backend && go build -o build\acosmi.exe ./cmd/acosmi && cd ..
if not exist "backend\build\acosmi.exe" (
    echo [FAIL] Gateway 编译失败
    pause
    exit /b 1
)
echo   [OK] Gateway 编译完成
echo.

REM ===== 启动 Gateway =====
echo [INFO] 启动 Gateway (port 19001)...
start /B "" backend\build\acosmi.exe -dev -port 19001

REM ===== 启动前端 UI =====
echo [INFO] 启动前端 UI (port 26222)...
cd ui
start /B "" cmd /c "npm run dev"
cd ..

REM ===== 等待服务就绪 =====
echo [INFO] 等待服务就绪...
set /a ELAPSED=0
set MAX_WAIT=60

:waitloop
if %ELAPSED% geq %MAX_WAIT% goto timeout
curl -s -o nul http://localhost:26222 2>nul
if %errorlevel% equ 0 goto ready
set /a ELAPSED+=1
timeout /t 1 /noq >nul
echo|set /p=.
goto waitloop

:ready
echo.
echo   [OK] 所有服务已就绪！
echo.
echo [INFO] 正在打开浏览器...
start http://localhost:26222
echo.
echo ════════════════════════════════════════════
echo   Gateway:  http://localhost:19001
echo   前端 UI:  http://localhost:26222
echo   按 Ctrl+C 停止所有服务
echo ════════════════════════════════════════════
echo.
pause
goto end

:timeout
echo.
echo   [WARN] 等待超时 (%MAX_WAIT%s)，服务可能还未完全就绪
echo   [WARN] 请手动检查: Gateway http://localhost:19001  UI http://localhost:26222
start http://localhost:26222
pause

:end
REM 清理进程
taskkill /f /im acosmi.exe >nul 2>&1
