package gateway

import (
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// ---------- HTTP 路由分发 ----------

// GatewayHTTPHandlerConfig HTTP 路由处理器配置。
type GatewayHTTPHandlerConfig struct {
	// GetHooksConfig 获取当前 hooks 配置（可热更新）。
	GetHooksConfig func() *HooksConfig
	// GetAuth 获取当前认证配置。
	GetAuth func() ResolvedGatewayAuth
	// HookDispatchers 回调
	OnWakeHook  func(payload *HookWakePayload)
	OnAgentHook func(payload *HookAgentPayload) string // 返回 runId
	// Logger
	Logger *log.Logger
	// CORS origin
	CORSOrigin string
	// ControlUI 静态文件目录 (为空则不提供)
	ControlUIDir string
	// TrustedProxies
	TrustedProxies []string
	// PipelineDispatcher 管线分发器（OpenAI API 需要）
	PipelineDispatcher PipelineDispatcher
	// ToolNames 已注册工具名称列表（tools invoke 需要）
	ToolNames []string
	// ToolInvoker 工具调用回调（DI 注入）
	ToolInvoker ToolInvoker
	// GetPublicMethodContext 公开统计 API 依赖（可选；nil = 不注册 /public/stats/*）。
	GetPublicMethodContext func() *GatewayMethodContext
}

// CreateGatewayHTTPHandler 创建主 HTTP 路由分发处理器。
// 根据路径前缀路由到各子系统: hooks → openai → control-ui → 404。
func CreateGatewayHTTPHandler(cfg GatewayHTTPHandlerConfig) http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		SendJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Hooks handler
	hooksHandler := createHooksHTTPHandler(cfg)
	mux.HandleFunc("/hooks/", hooksHandler)

	// OpenAI Chat Completions API
	openaiCfg := OpenAIChatHandlerConfig{
		GetAuth:        cfg.GetAuth,
		Dispatcher:     cfg.PipelineDispatcher,
		TrustedProxies: cfg.TrustedProxies,
	}
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		HandleOpenAIChatCompletions(w, r, openaiCfg)
	})

	// OpenAI Responses API (proxy to chat completions)
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		HandleOpenAIResponses(w, r, openaiCfg)
	})

	// Tools Invoke
	toolsCfg := ToolsInvokeHandlerConfig{
		GetAuth:   cfg.GetAuth,
		Invoker:   cfg.ToolInvoker,
		ToolNames: cfg.ToolNames,
	}
	mux.HandleFunc("/tools/invoke/", func(w http.ResponseWriter, r *http.Request) {
		HandleToolsInvoke(w, r, toolsCfg)
	})

	RegisterPublicUsageStatsRoutes(mux, PublicUsageStatsHandlerConfig{
		GetMethodContext: cfg.GetPublicMethodContext,
		CORSOrigin:       "*",
		CacheControl:     "public, max-age=60",
	})

	// Control UI (静态文件)
	if cfg.ControlUIDir != "" {
		fs := http.FileServer(http.Dir(cfg.ControlUIDir))
		mux.Handle("/ui/", http.StripPrefix("/ui/", fs))
	}

	// GW-07: Canvas/A2UI/Slack HTTP 路由桩
	// TS 对照: server-http.ts Canvas/a2ui/Slack 路由
	// 当前返回 501 Not Implemented，后续 Phase 对接具体实现。
	mux.HandleFunc("/canvas/", func(w http.ResponseWriter, r *http.Request) {
		SendJSON(w, http.StatusNotImplemented, map[string]interface{}{
			"error":   ErrCodeNotImplemented,
			"message": "canvas HTTP endpoints are not yet implemented",
		})
	})
	mux.HandleFunc("/a2ui/", func(w http.ResponseWriter, r *http.Request) {
		SendJSON(w, http.StatusNotImplemented, map[string]interface{}{
			"error":   ErrCodeNotImplemented,
			"message": "a2ui HTTP endpoints are not yet implemented",
		})
	})
	mux.HandleFunc("/integrations/slack/", func(w http.ResponseWriter, r *http.Request) {
		SendJSON(w, http.StatusNotImplemented, map[string]interface{}{
			"error":   ErrCodeNotImplemented,
			"message": "slack integration HTTP endpoints are not yet implemented",
		})
	})

	// 404 fallback — ServeMux 自带

	return mux
}

// ---------- Hooks HTTP 处理器 ----------

// createHooksHTTPHandler 创建 hooks 路由处理器。
func createHooksHTTPHandler(cfg GatewayHTTPHandlerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// S-11: 全局 500 恢复
		defer func() {
			if rec := recover(); rec != nil {
				logf(cfg.Logger, "hooks handler panic: %v", rec)
				SendJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"error":   ErrCodeInternalError,
					"message": "internal server error",
				})
			}
		}()

		hooksConfig := cfg.GetHooksConfig()
		if hooksConfig == nil {
			SendNotFound(w)
			return
		}

		// 只允许 POST
		if r.Method != http.MethodPost {
			SendMethodNotAllowed(w, "POST")
			return
		}

		// S-1: 拒绝 URL 中的 token (防止泄露到日志)
		if r.URL.Query().Get("token") != "" {
			SendJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":   "invalid_request",
				"message": "token must not be passed as a query parameter; use Authorization header, X-CrabClaw-Token header, or legacy X-OpenAcosmi-Token header",
			})
			return
		}

		// Token 验证
		token := ExtractHookToken(r)
		if token == "" || !SafeEqual(token, hooksConfig.Token) {
			SendUnauthorized(w)
			return
		}

		// 读取 body
		body, err := ReadJSONBody(r, hooksConfig.MaxBodyBytes)
		if err != nil {
			// S-6: 区分 413 vs 400
			if strings.Contains(err.Error(), "too large") {
				SendJSON(w, http.StatusRequestEntityTooLarge, map[string]interface{}{
					"error":   "payload_too_large",
					"message": err.Error(),
				})
			} else {
				SendInvalidRequest(w, err.Error())
			}
			return
		}

		// 解析 subPath
		subPath := strings.TrimPrefix(r.URL.Path, "/hooks")
		subPath = strings.TrimPrefix(subPath, "/")

		// S-3: 空 subPath 返回 404
		if subPath == "" {
			SendNotFound(w)
			return
		}

		// S-2: 直接路由 wake 和 agent 子路径
		switch subPath {
		case "wake":
			dispatchDirectWake(w, body, cfg)
			return
		case "agent":
			dispatchDirectAgent(w, body, cfg)
			return
		}

		// 构造映射上下文，从 payload.source 获取 source (M-12 修复)
		payloadSource := ""
		if bodyMap, ok := body.(map[string]interface{}); ok {
			if src, ok := bodyMap["source"].(string); ok {
				payloadSource = src
			}
		}
		// 如果 payload 无 source 字段, 则 fallback 到 header 检测
		if payloadSource == "" {
			payloadSource = detectSource(r)
		}

		ctx := &HookMappingContext{
			Path:    subPath,
			Source:  payloadSource,
			Method:  r.Method,
			Headers: NormalizeHookHeaders(r),
			Body:    body,
			Query:   r.URL.Query(),
		}

		// 匹配映射规则
		result, err := ApplyHookMappings(hooksConfig.Mappings, ctx)
		if err != nil {
			logf(cfg.Logger, "hooks mapping error: %v", err)
			SendInvalidRequest(w, err.Error())
			return
		}
		if result == nil {
			SendJSON(w, http.StatusOK, map[string]interface{}{
				"matched": false,
			})
			return
		}

		// 分发 action
		switch result.Action {
		case "wake":
			wakePayload, werr := NormalizeWakePayload(result.Payload)
			if werr != nil {
				SendInvalidRequest(w, "wake payload: "+werr.Error())
				return
			}
			if cfg.OnWakeHook != nil {
				cfg.OnWakeHook(wakePayload)
			}
			SendJSON(w, http.StatusOK, map[string]interface{}{
				"matched": true,
				"action":  "wake",
			})

		case "agent":
			agentPayload, aerr := NormalizeAgentPayload(result.Payload)
			if aerr != nil {
				SendInvalidRequest(w, "agent payload: "+aerr.Error())
				return
			}
			// S-5: 返回 202 + runId
			runId := ""
			if cfg.OnAgentHook != nil {
				runId = cfg.OnAgentHook(agentPayload)
			}
			if runId == "" {
				runId = uuid.New().String()
			}
			SendJSON(w, http.StatusAccepted, map[string]interface{}{
				"matched": true,
				"action":  "agent",
				"runId":   runId,
			})

		default:
			SendInvalidRequest(w, "unknown action: "+result.Action)
		}
	}
}

// dispatchDirectWake 处理 /hooks/wake 直接路由。
func dispatchDirectWake(w http.ResponseWriter, body interface{}, cfg GatewayHTTPHandlerConfig) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		bodyMap = make(map[string]interface{})
	}
	wakePayload, err := NormalizeWakePayload(bodyMap)
	if err != nil {
		SendInvalidRequest(w, "wake payload: "+err.Error())
		return
	}
	if cfg.OnWakeHook != nil {
		cfg.OnWakeHook(wakePayload)
	}
	SendJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"mode": wakePayload.Mode,
	})
}

// dispatchDirectAgent 处理 /hooks/agent 直接路由。
func dispatchDirectAgent(w http.ResponseWriter, body interface{}, cfg GatewayHTTPHandlerConfig) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		bodyMap = make(map[string]interface{})
	}
	agentPayload, err := NormalizeAgentPayload(bodyMap)
	if err != nil {
		SendInvalidRequest(w, "agent payload: "+err.Error())
		return
	}
	// S-5: 返回 202 + runId
	runId := ""
	if cfg.OnAgentHook != nil {
		runId = cfg.OnAgentHook(agentPayload)
	}
	if runId == "" {
		runId = uuid.New().String()
	}
	SendJSON(w, http.StatusAccepted, map[string]interface{}{
		"ok":    true,
		"runId": runId,
	})
}

// detectSource 检测 webhook 来源。
func detectSource(r *http.Request) string {
	if r.Header.Get("X-GitHub-Event") != "" || r.Header.Get("X-GitHub-Delivery") != "" {
		return "github"
	}
	if r.Header.Get("X-GitLab-Event") != "" || r.Header.Get("X-GitLab-Token") != "" {
		return "gitlab"
	}
	if r.Header.Get("X-Slack-Signature") != "" || r.Header.Get("X-Slack-Request-Timestamp") != "" {
		return "slack"
	}
	return strings.ToLower(r.Header.Get("X-Webhook-Source"))
}

func logf(logger *log.Logger, format string, args ...interface{}) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}
