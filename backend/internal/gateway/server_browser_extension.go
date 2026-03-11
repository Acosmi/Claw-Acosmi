package gateway

// server_browser_extension.go — 浏览器扩展安装引导 HTTP 端点
//
// 提供:
// 1. GET  /browser-extension/          — 安装引导页（独立 HTML）
// 2. GET  /browser-extension/status    — Relay 状态 JSON（端口/token/连接状态）
// 3. GET  /browser-extension/download  — 扩展文件打包下载（.zip）

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
)

// BrowserExtensionHandlerConfig 扩展引导页配置。
type BrowserExtensionHandlerConfig struct {
	// ExtensionDir 扩展源码目录（browser-extension/）。
	// 为空时自动从可执行文件相对路径推断。
	ExtensionDir string
	// ExpectedRelayPort 是安装引导页展示的默认 Relay 端口。
	ExpectedRelayPort int
	// ExpectedRelayURL 是安装引导页展示的默认 Relay 地址。
	ExpectedRelayURL string
	// GetRelayInfo 获取当前 relay 状态。
	GetRelayInfo func() *RelayStatusInfo
}

// RelayStatusInfo Relay 连接信息。
type RelayStatusInfo struct {
	Port      int    `json:"port"`
	Token     string `json:"token"`
	Connected bool   `json:"connected"`
	RelayURL  string `json:"relayUrl"`
}

// RegisterBrowserExtensionRoutes 注册扩展相关 HTTP 路由到 mux。
func RegisterBrowserExtensionRoutes(mux *http.ServeMux, cfg BrowserExtensionHandlerConfig) {
	mux.HandleFunc("/browser-extension/", func(w http.ResponseWriter, r *http.Request) {
		// 精确匹配根路径 → 引导页
		path := strings.TrimPrefix(r.URL.Path, "/browser-extension")
		path = strings.TrimPrefix(path, "/")
		if path == "" || path == "index.html" {
			serveBrowserExtensionGuide(w, r, cfg)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/browser-extension/status", func(w http.ResponseWriter, r *http.Request) {
		serveBrowserExtensionStatus(w, r, cfg)
	})

	mux.HandleFunc("/browser-extension/download", func(w http.ResponseWriter, r *http.Request) {
		serveBrowserExtensionDownload(w, r, cfg)
	})
}

// ---------- 引导页 ----------

func serveBrowserExtensionGuide(w http.ResponseWriter, r *http.Request, cfg BrowserExtensionHandlerConfig) {
	relayPort, relayURL := resolveExpectedRelay(cfg)

	html := browserExtensionGuideHTML(relayURL, relayPort)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html)) //nolint:errcheck
}

// ---------- Relay 状态 ----------

func serveBrowserExtensionStatus(w http.ResponseWriter, r *http.Request, cfg BrowserExtensionHandlerConfig) {
	info := &RelayStatusInfo{}
	if cfg.GetRelayInfo != nil {
		if ri := cfg.GetRelayInfo(); ri != nil {
			info = ri
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info) //nolint:errcheck
}

func resolveExpectedRelay(cfg BrowserExtensionHandlerConfig) (int, string) {
	port := cfg.ExpectedRelayPort
	if port <= 0 {
		port = browser.ResolveRelayPort()
	}
	url := strings.TrimSpace(cfg.ExpectedRelayURL)
	if url == "" {
		url = fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	}
	return port, url
}

// ---------- 扩展打包下载 ----------

func serveBrowserExtensionDownload(w http.ResponseWriter, r *http.Request, cfg BrowserExtensionHandlerConfig) {
	extDir := resolveExtensionDir(cfg.ExtensionDir)
	if extDir == "" {
		http.Error(w, "browser-extension directory not found", http.StatusNotFound)
		return
	}

	// 将 browser-extension/ 目录打包为 zip
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	err := filepath.WalkDir(extDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(extDir, path)
		// 在 zip 内加上 browser-extension/ 前缀
		zipPath := filepath.Join("browser-extension", relPath)
		zipPath = filepath.ToSlash(zipPath) // 统一为 /

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fw, err := zw.Create(zipPath)
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	})
	if err != nil {
		slog.Warn("browser-extension: zip packaging failed", "error", err)
		http.Error(w, "failed to package extension", http.StatusInternalServerError)
		return
	}
	if err := zw.Close(); err != nil {
		http.Error(w, "failed to finalize zip", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=crabclaw-browser-extension.zip")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.Write(buf.Bytes()) //nolint:errcheck
}

// resolveExtensionDir 查找 browser-extension/ 目录。
func resolveExtensionDir(configured string) string {
	exe := ""
	if resolved, err := os.Executable(); err == nil {
		exe = resolved
	}
	cwd := ""
	if resolved, err := os.Getwd(); err == nil {
		cwd = resolved
	}
	return resolveExtensionDirFromPaths(configured, exe, cwd)
}

func resolveExtensionDirFromPaths(configured, execPath, cwd string) string {
	for _, candidate := range extensionDirCandidates(configured, execPath, cwd) {
		if extensionDirExists(candidate) {
			return candidate
		}
	}
	return ""
}

func extensionDirExists(dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "manifest.json"))
	return err == nil
}

func extensionDirCandidates(configured, execPath, cwd string) []string {
	seen := make(map[string]struct{})
	var candidates []string

	appendCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			return
		}
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		candidates = append(candidates, abs)
	}

	appendCandidate(configured)

	if execPath != "" {
		execDir := filepath.Dir(execPath)
		appendCandidate(filepath.Join(execDir, "..", "Resources", "browser-extension"))
		appendCandidate(filepath.Join(execDir, "..", "browser-extension"))
		appendCandidate(filepath.Join(execDir, "..", "..", "browser-extension"))
		appendCandidate(filepath.Join(execDir, "browser-extension"))
	}

	if cwd != "" {
		appendCandidate(filepath.Join(cwd, "browser-extension"))
		appendCandidate(filepath.Join(cwd, "..", "browser-extension"))
	}

	return candidates
}

// ---------- 引导页 HTML ----------

func browserExtensionGuideHTML(relayURL string, relayPort int) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Crab Claw by Acosmi - 浏览器扩展安装引导</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif;
    background: #f5f5f7;
    color: #1d1d1f;
    line-height: 1.6;
  }
  .container { max-width: 720px; margin: 0 auto; padding: 40px 24px; }
  .header {
    text-align: center;
    margin-bottom: 40px;
  }
  .header h1 {
    font-size: 28px;
    font-weight: 700;
    background: linear-gradient(135deg, #FF4500, #FF6B35);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    margin-bottom: 8px;
  }
  .header p { color: #86868b; font-size: 15px; }
  .status-card {
    background: white;
    border-radius: 12px;
    padding: 20px 24px;
    margin-bottom: 24px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.08);
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .status-dot {
    width: 12px; height: 12px;
    border-radius: 50%%;
    flex-shrink: 0;
  }
  .status-dot.ok { background: #30d158; }
  .status-dot.warn { background: #ff9f0a; }
  .status-dot.err { background: #ff3b30; }
  .status-info { flex: 1; }
  .status-info .label { font-size: 13px; color: #86868b; }
  .status-info .value { font-size: 15px; font-weight: 500; }
  .step-card {
    background: white;
    border-radius: 12px;
    padding: 24px;
    margin-bottom: 16px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.08);
  }
  .step-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }
  .step-num {
    width: 32px; height: 32px;
    border-radius: 50%%;
    background: linear-gradient(135deg, #FF4500, #FF6B35);
    color: white;
    font-weight: 700;
    font-size: 15px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .step-title { font-size: 17px; font-weight: 600; }
  .step-body { padding-left: 44px; font-size: 14px; color: #424245; }
  .step-body ol { padding-left: 20px; }
  .step-body li { margin-bottom: 8px; }
  .step-body code {
    background: #f5f5f7;
    padding: 2px 8px;
    border-radius: 4px;
    font-family: 'SF Mono', Monaco, monospace;
    font-size: 13px;
    color: #FF4500;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 10px 20px;
    border-radius: 8px;
    border: none;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    text-decoration: none;
    transition: all 0.2s;
  }
  .btn-primary {
    background: linear-gradient(135deg, #FF4500, #FF6B35);
    color: white;
  }
  .btn-primary:hover { opacity: 0.9; transform: translateY(-1px); }
  .btn-secondary {
    background: #e8e8ed;
    color: #1d1d1f;
  }
  .btn-secondary:hover { background: #d2d2d7; }
  .btn-group { display: flex; gap: 12px; margin-top: 16px; flex-wrap: wrap; }
  .tip {
    background: #fff3cd;
    border-left: 4px solid #ff9f0a;
    padding: 12px 16px;
    border-radius: 0 8px 8px 0;
    font-size: 13px;
    margin-top: 12px;
    color: #664d03;
  }
  .config-box {
    background: #1d1d1f;
    color: #f5f5f7;
    padding: 16px;
    border-radius: 8px;
    font-family: 'SF Mono', Monaco, monospace;
    font-size: 13px;
    margin-top: 12px;
    overflow-x: auto;
    position: relative;
  }
  .config-box .copy-btn {
    position: absolute;
    top: 8px; right: 8px;
    background: rgba(255,255,255,0.1);
    border: none;
    color: #86868b;
    padding: 4px 8px;
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
  }
  .config-box .copy-btn:hover { background: rgba(255,255,255,0.2); color: white; }
  .footer {
    text-align: center;
    margin-top: 40px;
    color: #86868b;
    font-size: 13px;
  }
  .check-result {
    margin-top: 8px;
    font-size: 13px;
    padding: 8px 12px;
    border-radius: 6px;
    display: none;
  }
  .check-result.ok { display: block; background: #d1f2d1; color: #0a5c0a; }
  .check-result.fail { display: block; background: #ffd6d6; color: #5c0a0a; }
  .success-banner {
    display: none;
    background: linear-gradient(135deg, #d1f2d1, #e8f5e9);
    border: 2px solid #30d158;
    border-radius: 12px;
    padding: 20px 24px;
    margin-bottom: 24px;
    text-align: center;
    animation: fadeIn 0.5s ease-out;
  }
  .success-banner h2 { color: #0a5c0a; font-size: 20px; margin-bottom: 8px; }
  .success-banner p { color: #1b5e20; font-size: 14px; }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-10px); } to { opacity: 1; transform: translateY(0); } }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <h1>Crab Claw by Acosmi Browser Extension</h1>
    <p>让 AI Agent 控制您的 Chrome 标签页</p>
  </div>

  <!-- 连接成功 Banner（自动检测到扩展连接后显示）-->
  <div class="success-banner" id="successBanner">
    <h2>扩展连接成功！Extension Connected!</h2>
    <p>浏览器扩展已成功连接到 Relay 服务器。现在可以通过 AI Agent 控制浏览器了。</p>
  </div>

  <!-- Relay 状态 -->
  <div class="status-card">
    <div class="status-dot" id="relayDot"></div>
    <div class="status-info">
      <div class="label">Extension Relay 服务</div>
      <div class="value" id="relayStatus">检查中...</div>
    </div>
    <button class="btn btn-secondary" onclick="checkRelay()" style="font-size:12px;padding:6px 12px;">刷新</button>
  </div>

  <!-- 步骤 1 -->
  <div class="step-card">
    <div class="step-header">
      <div class="step-num">1</div>
      <div class="step-title">下载扩展</div>
    </div>
    <div class="step-body">
      <p>下载扩展压缩包并解压到任意位置。</p>
      <div class="btn-group">
        <a href="/browser-extension/download" class="btn btn-primary">下载扩展 (.zip)</a>
      </div>
      <div class="tip">
        解压后会得到 <code>browser-extension/</code> 文件夹，包含 manifest.json、background.js 等文件。
        建议放在固定位置（如 <code>~/crabclaw-extension/</code>），避免误删。
      </div>
    </div>
  </div>

  <!-- 步骤 2 -->
  <div class="step-card">
    <div class="step-header">
      <div class="step-num">2</div>
      <div class="step-title">安装到 Chrome</div>
    </div>
    <div class="step-body">
      <ol>
        <li>打开 Chrome，地址栏输入 <code>chrome://extensions</code> 并回车</li>
        <li>打开右上角 <strong>开发者模式</strong> 开关</li>
        <li>点击 <strong>加载已解压的扩展程序</strong></li>
        <li>选择刚才解压的 <code>browser-extension/</code> 文件夹</li>
        <li>安装成功后，点击工具栏拼图图标将扩展固定</li>
      </ol>
      <div class="btn-group">
        <button class="btn btn-secondary" onclick="copyExtensionsURL()">
          <span id="copyExtBtnText">复制 chrome://extensions 地址</span>
        </button>
      </div>
      <div id="copyExtResult" class="check-result" style="display:none;"></div>
      <div class="tip">
        浏览器安全策略不允许网页直接打开 <code>chrome://extensions</code>，请复制后粘贴到地址栏回车。
        此操作只需执行一次，后续更新只需在扩展页面点击"重新加载"。
      </div>
      <div class="tip" style="margin-top:8px;background:#e8f4fd;border-left-color:#007aff;color:#003d66;">
        <strong>Chrome 134+ 注意：</strong>开发者模式需保持开启。每次启动 Chrome 会显示一条
        "禁用开发者模式扩展"的警告条，点击关闭即可，不影响正常使用。
      </div>
    </div>
  </div>

  <!-- 步骤 3 -->
  <div class="step-card">
    <div class="step-header">
      <div class="step-num">3</div>
      <div class="step-title">连接与配置</div>
    </div>
    <div class="step-body">
      <p>扩展安装后会自动尝试连接 Relay 服务器。如果连接失败，在弹窗中填入以下地址：</p>
      <div class="config-box">
        <button class="copy-btn" onclick="copyText('%s')">复制</button>
        %s
      </div>
      <p style="margin-top:12px;">Token 留空即可，扩展会自动发现。</p>
      <div id="checkResult" class="check-result"></div>
      <div class="btn-group">
        <button class="btn btn-primary" onclick="checkRelay()">检测 Relay 连接</button>
      </div>
    </div>
  </div>

  <!-- 步骤 4 -->
  <div class="step-card">
    <div class="step-header">
      <div class="step-num">4</div>
      <div class="step-title">附加标签页</div>
    </div>
    <div class="step-body">
      <ol>
        <li>打开要让 Agent 控制的网页</li>
        <li>点击工具栏的 <strong>Crab Claw by Acosmi</strong> 扩展图标</li>
        <li>在标签页列表中点击 <strong>Attach</strong></li>
        <li>看到绿色 <code>ON</code> 标记即表示就绪</li>
      </ol>
      <div class="tip">
        附加后，Agent 拥有该标签页的完整控制权（点击、输入、导航、读取内容）。
        请仅附加您信任的标签页。建议使用专用 Chrome 配置文件。
      </div>
    </div>
  </div>

  <div class="footer">
    <p>Crab Claw by Acosmi Browser Extension v1.0.0</p>
    <p style="margin-top:4px;">Relay 端口: %d | 仅监听 127.0.0.1</p>
  </div>
</div>

<script>
var _extensionConnected = false;

function checkRelay() {
  var dot = document.getElementById('relayDot');
  var status = document.getElementById('relayStatus');
  var result = document.getElementById('checkResult');
  var banner = document.getElementById('successBanner');
  dot.className = 'status-dot warn';
  status.textContent = '检查中...';

  fetch('/browser-extension/status')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      if (data.connected) {
        dot.className = 'status-dot ok';
        status.textContent = '扩展已连接 (端口 ' + data.port + ')';
        if (result) {
          result.className = 'check-result ok';
          result.textContent = '扩展已连接到 Relay 服务器，可以开始使用浏览器自动化。';
        }
        if (banner && !_extensionConnected) {
          banner.style.display = 'block';
          _extensionConnected = true;
        }
      } else if (data.port > 0) {
        dot.className = 'status-dot ok';
        status.textContent = 'Relay 运行中 (端口 ' + data.port + ')，等待扩展连接...';
        if (result) {
          result.className = 'check-result ok';
          result.textContent = 'Relay 服务正常运行。安装扩展后将自动连接。';
        }
      } else {
        dot.className = 'status-dot err';
        status.textContent = 'Relay 未启动';
        if (result) {
          result.className = 'check-result fail';
          result.textContent = 'Relay 服务未检测到。请确认 Gateway 已启动且浏览器功能已启用。';
        }
      }
    })
    .catch(function() {
      dot.className = 'status-dot err';
      status.textContent = '无法连接';
      if (result) {
        result.className = 'check-result fail';
        result.textContent = '无法连接到 Gateway 服务。请确认 Gateway 正在运行。';
      }
    });
}

function copyExtensionsURL() {
  navigator.clipboard.writeText('chrome://extensions').then(function() {
    var btnText = document.getElementById('copyExtBtnText');
    var tip = document.getElementById('copyExtResult');
    if (btnText) {
      btnText.textContent = '已复制！';
      setTimeout(function() { btnText.textContent = '复制 chrome://extensions 地址'; }, 2000);
    }
    if (tip) {
      tip.className = 'check-result ok';
      tip.textContent = '已复制！请粘贴到 Chrome 地址栏并回车。';
      tip.style.display = 'block';
    }
  });
}

function copyText(text) {
  navigator.clipboard.writeText(text).then(function() {
    var btn = event.target;
    btn.textContent = '已复制';
    setTimeout(function() { btn.textContent = '复制'; }, 1500);
  });
}

// 页面加载时检查一次，之后每 3 秒自动轮询直到扩展连接
checkRelay();
var _pollTimer = setInterval(function() {
  if (_extensionConnected) { clearInterval(_pollTimer); return; }
  checkRelay();
}, 3000);
</script>
</body>
</html>`, relayURL, relayURL, relayPort)
}
