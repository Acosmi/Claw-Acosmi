package gateway

// server_methods_browser.go — browser.* 方法处理器
// 对应 TS: src/gateway/server-methods/browser.ts (278L)
//
// 方法列表 (1): browser.request
//
// TS 实现包含两条分支:
// 1. Node proxy: 转发给远程节点的 browser service (browser.proxy)
// 2. Local control: 本地 browser control service（dispatcher 模式）
// Go 完整实现两条分支。

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// BrowserHandlers 返回 browser.* 方法映射。
func BrowserHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"browser.request": handleBrowserRequest,
	}
}

// ---------- browser.request ----------
// TS: browser.ts L148-276
// 参数: { method, path, query, body, timeoutMs }
// 逻辑:
// 1. 验证 method (GET/POST/DELETE) 和 path
// 2. 尝试 node proxy (config-driven)
// 3. 降级到 local browser control service

func handleBrowserRequest(ctx *MethodHandlerContext) {
	// FIND-12: 参数协议对齐 TS {method, path, query, body, timeoutMs}
	methodRaw, _ := ctx.Params["method"].(string)
	methodRaw = strings.TrimSpace(strings.ToUpper(methodRaw))
	path, _ := ctx.Params["path"].(string)
	path = strings.TrimSpace(path)

	var query map[string]interface{}
	if q, ok := ctx.Params["query"].(map[string]interface{}); ok {
		query = q
	}
	body := ctx.Params["body"]
	var timeoutMs int
	if tm, ok := ctx.Params["timeoutMs"].(float64); ok && !math.IsNaN(tm) && !math.IsInf(tm, 0) {
		timeoutMs = int(math.Max(1, math.Floor(tm)))
	}

	// FIND-12: 验证 method 和 path
	if methodRaw == "" || path == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "method and path are required"))
		return
	}

	// FIND-13: method 验证 GET/POST/DELETE
	if methodRaw != "GET" && methodRaw != "POST" && methodRaw != "DELETE" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "method must be GET, POST, or DELETE"))
		return
	}

	// FIND-14: node proxy 逻辑 — config-driven
	// 尝试解析 node target
	nodeTarget := resolveBrowserNodeTarget(ctx)
	if nodeTarget != nil {
		// 命令策略检查
		policyInput := NodeCommandPolicyInput{
			Platform:     nodeTarget.Platform,
			DeviceFamily: nodeTarget.DeviceFamily,
		}
		allowlist := ResolveNodeCommandAllowlist(policyInput)
		allowed := IsNodeCommandAllowed("browser.proxy", nodeTarget.Commands, allowlist)
		if !allowed.OK {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "node command not allowed: "+allowed.Reason))
			return
		}

		// 构建 proxy 参数
		proxyParams := map[string]interface{}{
			"method": methodRaw,
			"path":   path,
		}
		if query != nil {
			proxyParams["query"] = query
		}
		if body != nil {
			proxyParams["body"] = body
		}
		if timeoutMs > 0 {
			proxyParams["timeoutMs"] = timeoutMs
		}
		if query != nil {
			if profile, ok := query["profile"].(string); ok {
				proxyParams["profile"] = profile
			}
		}

		result, err := ctx.Context.NodeRegistryGW.Invoke(nodeTarget.NodeID, "browser.proxy", proxyParams)
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "node browser.proxy failed: "+err.Error()))
			return
		}

		// FIND-16: 解析 proxy 结果，持久化文件
		proxyResult := parseBrowserProxyResult(result)
		if proxyResult == nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "browser proxy failed"))
			return
		}

		// 持久化文件并替换路径
		mapping := persistBrowserProxyFiles(proxyResult.Files, ctx)
		applyBrowserProxyPaths(proxyResult.Result, mapping)

		ctx.Respond(true, proxyResult.Result, nil)
		return
	}

	// FIND-15: Local browser control service
	// TS: startBrowserControlServiceFromConfig() → createBrowserRouteDispatcher()
	// Go: 使用 HTTP 请求到本地 browser control service
	port := resolveBrowserControlPort()
	localURL := fmt.Sprintf("http://localhost:%d%s", port, path)

	// 构建 query string
	if len(query) > 0 {
		qs := make([]string, 0, len(query))
		for k, v := range query {
			qs = append(qs, fmt.Sprintf("%s=%v", k, v))
		}
		localURL += "?" + strings.Join(qs, "&")
	}

	// 构建请求体
	var bodyReader io.Reader
	if body != nil && methodRaw != "GET" {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to marshal browser request body"))
			return
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	timeout := 30 * time.Second
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	httpReq, err := http.NewRequest(methodRaw, localURL, bodyReader)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to create HTTP request: "+err.Error()))
		return
	}
	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "browser control service unavailable: "+err.Error()))
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read browser response: "+err.Error()))
		return
	}

	if resp.StatusCode >= 400 {
		// TS: 提取 error 字段或生成默认消息
		var errMsg string
		var parsed map[string]interface{}
		if json.Unmarshal(respBody, &parsed) == nil {
			if e, ok := parsed["error"]; ok {
				errMsg = fmt.Sprintf("%v", e)
			}
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("browser request failed (%d)", resp.StatusCode)
		}
		code := ErrCodeBadRequest
		if resp.StatusCode >= 500 {
			code = ErrCodeServiceUnavailable
		}
		ctx.Respond(false, nil, NewErrorShape(code, errMsg))
		return
	}

	// 解析 JSON 响应
	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// 非 JSON 响应，作为文本返回
		ctx.Respond(true, map[string]interface{}{
			"raw": string(respBody),
		}, nil)
		return
	}

	ctx.Respond(true, result, nil)
}

// ---------- 辅助函数 ----------

func resolveBrowserControlPort() int {
	// 从 Gateway 端口推导 browser control 端口。
	// 公式: gateway_port + 2（与 config/portdefaults.go DeriveDefaultBrowserControlPort 一致）。
	gatewayPort := config.ResolveGatewayPort(nil)
	return config.DeriveDefaultBrowserControlPort(gatewayPort)
}

// resolveBrowserNodeTarget 解析 browser node target（TS: resolveBrowserNodeTarget）
// 从已连接节点中查找具有 browser cap 的节点。
func resolveBrowserNodeTarget(ctx *MethodHandlerContext) *ConnectedNodeInfo {
	if ctx.Context.NodeRegistryGW == nil {
		return nil
	}

	// TODO: 从 config 读取 gateway.nodes.browser.mode (auto/manual/off)
	// 当前实现: auto 模式（自动选择唯一 browser-capable 节点）
	mode := "auto"

	if mode == "off" {
		return nil
	}

	connected := ctx.Context.NodeRegistryGW.ConnectedNodes()
	browserNodes := make([]ConnectedNodeInfo, 0)
	for _, cn := range connected {
		if isBrowserNode(&cn) {
			browserNodes = append(browserNodes, cn)
		}
	}

	if len(browserNodes) == 0 {
		return nil
	}

	// auto 模式: 仅当恰好一个 browser node 时自动选择
	if mode == "auto" && len(browserNodes) == 1 {
		return &browserNodes[0]
	}

	// manual 模式或多个 browser node: 不自动选择
	return nil
}

// isBrowserNode 检查节点是否具有 browser 能力。
func isBrowserNode(node *ConnectedNodeInfo) bool {
	for _, cap := range node.Caps {
		if cap == "browser" {
			return true
		}
	}
	for _, cmd := range node.Commands {
		if cmd == "browser.proxy" {
			return true
		}
	}
	return false
}

// normalizeNodeKey 标准化节点键名用于匹配
var nonAlphaNumRe = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeNodeKey(value string) string {
	return nonAlphaNumRe.ReplaceAllString(strings.TrimSpace(strings.ToLower(value)), "")
}

// ---------- Proxy 文件持久化 (FIND-16) ----------

type browserProxyFile struct {
	Path     string `json:"path"`
	Base64   string `json:"base64"`
	MimeType string `json:"mimeType,omitempty"`
}

type browserProxyResult struct {
	Result interface{}         `json:"result"`
	Files  []*browserProxyFile `json:"files,omitempty"`
}

func parseBrowserProxyResult(raw interface{}) *browserProxyResult {
	if raw == nil {
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	result, hasResult := m["result"]
	if !hasResult {
		return nil
	}
	proxy := &browserProxyResult{Result: result}

	if filesRaw, ok := m["files"].([]interface{}); ok {
		for _, f := range filesRaw {
			if fm, ok := f.(map[string]interface{}); ok {
				pf := &browserProxyFile{}
				if p, ok := fm["path"].(string); ok {
					pf.Path = p
				}
				if b, ok := fm["base64"].(string); ok {
					pf.Base64 = b
				}
				if mt, ok := fm["mimeType"].(string); ok {
					pf.MimeType = mt
				}
				if pf.Path != "" && pf.Base64 != "" {
					proxy.Files = append(proxy.Files, pf)
				}
			}
		}
	}
	return proxy
}

// persistBrowserProxyFiles 持久化代理返回的文件。
// TS: saveMediaBuffer → media store
// HEALTH-D2: base64 解码 → 写入 ~/.openacosmi/media/{hash}.{ext}
func persistBrowserProxyFiles(files []*browserProxyFile, ctx *MethodHandlerContext) map[string]string {
	mapping := make(map[string]string)
	if len(files) == 0 {
		return mapping
	}

	mediaDir := resolveMediaDir()
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return mapping
	}

	for _, f := range files {
		if f.Base64 == "" || f.Path == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(f.Base64)
		if err != nil {
			continue
		}

		// 内容哈希去重
		hash := sha256.Sum256(data)
		hashHex := hex.EncodeToString(hash[:8]) // 前 8 字节 = 16 hex

		// 提取扩展名
		ext := filepath.Ext(f.Path)
		if ext == "" {
			ext = mimeToExt(f.MimeType)
		}

		savedName := hashHex + ext
		savedPath := filepath.Join(mediaDir, savedName)

		// 如果文件已存在且大小一致，跳过写入
		if info, err := os.Stat(savedPath); err == nil && info.Size() == int64(len(data)) {
			mapping[f.Path] = savedPath
			continue
		}

		if err := os.WriteFile(savedPath, data, 0o644); err != nil {
			continue
		}
		mapping[f.Path] = savedPath
	}
	return mapping
}

// resolveMediaDir 返回媒体文件存储目录。
func resolveMediaDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".openacosmi", "media")
}

// mimeToExt 从 MIME 类型推断文件扩展名。
func mimeToExt(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/png"):
		return ".png"
	case strings.HasPrefix(mimeType, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mimeType, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mimeType, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mimeType, "application/pdf"):
		return ".pdf"
	default:
		return ".bin"
	}
}

// applyBrowserProxyPaths 替换 proxy 结果中的文件路径。
func applyBrowserProxyPaths(result interface{}, mapping map[string]string) {
	if len(mapping) == 0 {
		return
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		return
	}
	// 替换 path
	if p, ok := m["path"].(string); ok {
		if newPath, found := mapping[p]; found {
			m["path"] = newPath
		}
	}
	// 替换 imagePath
	if p, ok := m["imagePath"].(string); ok {
		if newPath, found := mapping[p]; found {
			m["imagePath"] = newPath
		}
	}
	// 替换 download.path
	if dl, ok := m["download"].(map[string]interface{}); ok {
		if p, ok := dl["path"].(string); ok {
			if newPath, found := mapping[p]; found {
				dl["path"] = newPath
			}
		}
	}
}
