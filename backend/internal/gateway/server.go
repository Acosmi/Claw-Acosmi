package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anthropic/open-acosmi/internal/agents/llmclient"
	"github.com/anthropic/open-acosmi/internal/agents/models"
	"github.com/anthropic/open-acosmi/internal/agents/runner"
	"github.com/anthropic/open-acosmi/internal/agents/skills"
	"github.com/anthropic/open-acosmi/internal/argus"
	"github.com/anthropic/open-acosmi/internal/autoreply"
	"github.com/anthropic/open-acosmi/internal/autoreply/reply"
	"github.com/anthropic/open-acosmi/internal/channels"
	"github.com/anthropic/open-acosmi/internal/channels/dingtalk"
	"github.com/anthropic/open-acosmi/internal/channels/feishu"
	"github.com/anthropic/open-acosmi/internal/channels/wecom"
	"github.com/anthropic/open-acosmi/internal/cli"
	"github.com/anthropic/open-acosmi/internal/config"
	"github.com/anthropic/open-acosmi/internal/cron"
	"github.com/anthropic/open-acosmi/internal/media"
	"github.com/anthropic/open-acosmi/internal/memory/uhms"
	"github.com/anthropic/open-acosmi/internal/memory/uhms/vectoradapter"
	"github.com/anthropic/open-acosmi/internal/sandbox"
	applog "github.com/anthropic/open-acosmi/pkg/log"
	"github.com/anthropic/open-acosmi/pkg/mcpremote"
	types "github.com/anthropic/open-acosmi/pkg/types"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

// ---------- 网关启动编排 ----------
// 对齐 TS server.impl.ts: startGatewayServer()

// GatewayServerOptions 网关启动选项。
type GatewayServerOptions struct {
	ControlUIDir string
	BindMode     BindMode
	BindHost     string
}

// GatewayRuntime 网关运行时，持有 server/state 引用及关闭方法。
type GatewayRuntime struct {
	State             *GatewayState
	HTTPServer        *GatewayHTTPServer
	MaintenanceTimers *MaintenanceTimers
	mu                sync.Mutex
	closed            bool
}

// Close 优雅关闭网关。
func (rt *GatewayRuntime) Close(reason string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return nil
	}
	rt.closed = true

	slog.Info("gateway: shutting down", "reason", reason)

	// 停止维护计时器（tick 广播）
	if rt.MaintenanceTimers != nil {
		rt.MaintenanceTimers.Stop()
	}

	rt.State.SetPhase(BootPhaseStopping)

	// 广播 shutdown 事件
	rt.State.Broadcaster().Broadcast("gateway.shutdown", ShutdownEvent{
		Reason: reason,
	}, nil)

	// 停止沙箱子系统（容器池 + Worker）
	rt.State.StopSandbox()

	// 停止原生沙箱 Worker
	rt.State.StopNativeSandbox()

	// 停止 Argus 视觉子智能体
	rt.State.StopArgus()

	// 停止 Coder 编程子智能体
	rt.State.StopCoder()

	// 停止 MCP 远程工具 Bridge
	rt.State.StopRemoteMCP()

	// 停止 UHMS 记忆系统
	rt.State.StopUHMS()

	// 优雅关闭 HTTP 服务器
	if err := rt.HTTPServer.Shutdown(); err != nil {
		slog.Error("gateway: http shutdown error", "error", err)
	}

	rt.State.SetPhase(BootPhaseStopped)
	slog.Info("gateway: shutdown complete")
	return nil
}

// ---------- Argus Bridge → Agent 适配器 ----------

// argusBridgeAdapter 将 *argus.Bridge 适配为 runner.ArgusBridgeForAgent 接口。
// 转换 mcpclient 类型为 runner 本地类型，避免 runner→mcpclient 依赖。
type argusBridgeAdapter struct {
	bridge *argus.Bridge
}

func (a *argusBridgeAdapter) AgentTools() []runner.ArgusToolDef {
	tools := a.bridge.Tools()
	result := make([]runner.ArgusToolDef, len(tools))
	for i, t := range tools {
		result[i] = runner.ArgusToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return result
}

func (a *argusBridgeAdapter) AgentCallTool(ctx context.Context, name string, args json.RawMessage, timeout time.Duration) (string, error) {
	result, err := a.bridge.CallTool(ctx, name, args, timeout)
	if err != nil {
		return "", err
	}
	// 提取 MCP content → 纯文本（image 标注大小）
	var sb strings.Builder
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(c.Text)
		case "image":
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("[image: %s, %d bytes base64]", c.MIME, len(c.Data)))
		}
	}
	if result.IsError {
		return fmt.Sprintf("[Argus error] %s", sb.String()), nil
	}
	return sb.String(), nil
}

// ---------- Coder Bridge → Agent 适配器 ----------

// coderBridgeAdapter 将 *argus.Bridge（oa-coder MCP 子进程）适配为 runner.CoderBridgeForAgent 接口。
// 复用 argus.Bridge 的 MCP stdio 管理，仅更改错误前缀标识。
type coderBridgeAdapter struct {
	bridge *argus.Bridge
}

func (a *coderBridgeAdapter) AgentTools() []runner.ArgusToolDef {
	tools := a.bridge.Tools()
	result := make([]runner.ArgusToolDef, len(tools))
	for i, t := range tools {
		result[i] = runner.ArgusToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return result
}

func (a *coderBridgeAdapter) AgentCallTool(ctx context.Context, name string, args json.RawMessage, timeout time.Duration) (string, error) {
	result, err := a.bridge.CallTool(ctx, name, args, timeout)
	if err != nil {
		return "", err
	}
	// 提取 MCP content → 纯文本
	var sb strings.Builder
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(c.Text)
		}
	}
	if result.IsError {
		return fmt.Sprintf("[Coder error] %s", sb.String()), nil
	}
	return sb.String(), nil
}

// ---------- Native Sandbox Bridge → Agent 适配器 ----------

// nativeSandboxBridgeAdapter 将 *sandbox.NativeSandboxBridge 适配为 runner.NativeSandboxForAgent 接口。
// 转换 sandbox 包类型为 runner 包接口，避免 runner→sandbox 直接依赖。
type nativeSandboxBridgeAdapter struct {
	bridge *sandbox.NativeSandboxBridge
}

func (a *nativeSandboxBridgeAdapter) ExecuteSandboxed(ctx context.Context, cmd string, args []string, env map[string]string, timeoutMs int64) (stdout, stderr string, exitCode int, err error) {
	return a.bridge.Execute(ctx, cmd, args, env, timeoutMs)
}

// ---------- Remote MCP Bridge → Agent 适配器 ----------

// remoteMCPBridgeAdapter 将 *mcpremote.RemoteBridge 适配为 runner.RemoteMCPBridgeForAgent 接口。
type remoteMCPBridgeAdapter struct {
	bridge *mcpremote.RemoteBridge
}

func (a *remoteMCPBridgeAdapter) AgentRemoteTools() []runner.RemoteToolDef {
	tools := a.bridge.Tools()
	result := make([]runner.RemoteToolDef, len(tools))
	for i, t := range tools {
		result[i] = runner.RemoteToolDef{
			Name:        t.Name,
			Title:       t.Title,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return result
}

func (a *remoteMCPBridgeAdapter) AgentCallRemoteTool(ctx context.Context, name string, args json.RawMessage, timeout time.Duration) (string, error) {
	result, err := a.bridge.CallTool(ctx, name, args, timeout)
	if err != nil {
		return "", err
	}
	return mcpremote.ToolCallResultToText(result), nil
}

// ---------- UHMS Bridge → Agent 适配器 ----------

// uhmsBridgeAdapter 将 *uhms.DefaultManager 适配为 runner.UHMSBridgeForAgent 接口。
// 转换 llmclient.ChatMessage ↔ uhms.Message，避免 runner→uhms 直接依赖。
type uhmsBridgeAdapter struct {
	mgr         *uhms.DefaultManager
	broadcaster *Broadcaster // 可选, 用于 WS 事件广播
}

func (a *uhmsBridgeAdapter) CompressChatMessages(ctx context.Context, messages []llmclient.ChatMessage, tokenBudget int) ([]llmclient.ChatMessage, error) {
	// 快速路径: 粗估 token 量，低于阈值跳过双向转换 (参考 gRPC-Go PreparedMsg / Letta 门控)
	// 估算方式: 累加字节数 / 3.5 (英文+代码混合场景 ~90-96% 准确)
	threshold := a.mgr.CompressThreshold()
	if threshold > 0 {
		var totalBytes int
		for i := range messages {
			totalBytes += len(messages[i].Content)
		}
		estimatedTokens := int(float64(totalBytes) / 3.5)
		if estimatedTokens < threshold {
			return messages, nil // 直通: 避免 chatMessagesToUHMS + uhmsToChatMessages 开销
		}
	}

	beforeCount := len(messages)

	// 1. llmclient.ChatMessage → uhms.Message
	uhmsMessages := chatMessagesToUHMS(messages)

	// 2. 调用 UHMS 压缩
	compressed, err := a.mgr.CompressIfNeeded(ctx, uhmsMessages, tokenBudget)
	if err != nil {
		return nil, err
	}

	// 3. 广播压缩事件 (仅在实际压缩时)
	afterCount := len(compressed)
	if a.broadcaster != nil && afterCount < beforeCount {
		a.broadcaster.Broadcast("memory.compressed", map[string]interface{}{
			"before_messages": beforeCount,
			"after_messages":  afterCount,
			"ts":              time.Now().UnixMilli(),
		}, nil)
	}

	// 4. uhms.Message → llmclient.ChatMessage
	return uhmsToChatMessages(compressed), nil
}

func (a *uhmsBridgeAdapter) CommitChatSession(ctx context.Context, userID, sessionKey string, messages []llmclient.ChatMessage) error {
	uhmsMessages := chatMessagesToUHMS(messages)
	result, err := a.mgr.CommitSession(ctx, userID, sessionKey, uhmsMessages)
	if err != nil {
		return err
	}

	// 广播提交事件
	if a.broadcaster != nil && result != nil {
		a.broadcaster.Broadcast("memory.committed", map[string]interface{}{
			"session_key":      result.SessionKey,
			"memories_created": result.MemoriesCreated,
			"tokens_saved":     result.TokensSaved,
			"ts":               time.Now().UnixMilli(),
		}, nil)
	}
	return nil
}

func (a *uhmsBridgeAdapter) BuildContextBrief(ctx context.Context) string {
	// "default" matches the single-user desktop app convention used by
	// CompressChatMessages/Status/etc. Multi-user would require per-session userID.
	return a.mgr.BuildContextBrief(ctx, "default")
}

// chatMessagesToUHMS converts llmclient.ChatMessage slice to uhms.Message slice.
// Extracts text content from ContentBlock arrays.
func chatMessagesToUHMS(messages []llmclient.ChatMessage) []uhms.Message {
	result := make([]uhms.Message, 0, len(messages))
	for _, msg := range messages {
		var sb strings.Builder
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(block.Text)
			case "tool_use":
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("[tool_use: %s]", block.Name))
			case "tool_result":
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				text := block.ResultText
				if runes := []rune(text); len(runes) > 2000 {
					text = string(runes[:2000]) + "... (truncated)"
				}
				sb.WriteString(text)
			}
		}
		if content := sb.String(); content != "" {
			result = append(result, uhms.Message{
				Role:    msg.Role,
				Content: content,
			})
		}
	}
	return result
}

// uhmsToChatMessages converts uhms.Message slice back to llmclient.ChatMessage slice.
func uhmsToChatMessages(messages []uhms.Message) []llmclient.ChatMessage {
	result := make([]llmclient.ChatMessage, len(messages))
	for i, msg := range messages {
		result[i] = llmclient.TextMessage(msg.Role, msg.Content)
	}
	return result
}

// configToUHMSConfig converts types.MemoryUHMSConfig → uhms.UHMSConfig.
func configToUHMSConfig(c *types.MemoryUHMSConfig) uhms.UHMSConfig {
	cfg := uhms.DefaultUHMSConfig()
	cfg.Enabled = c.Enabled
	if c.DBPath != "" {
		cfg.DBPath = c.DBPath
	}
	if c.VFSPath != "" {
		cfg.VFSPath = c.VFSPath
	}
	if c.VectorMode != "" {
		cfg.VectorMode = uhms.VectorMode(c.VectorMode)
	}
	if c.CompressionThreshold > 0 {
		cfg.CompressionThreshold = c.CompressionThreshold
	}
	if c.DecayEnabled != nil {
		cfg.DecayEnabled = c.DecayEnabled
	}
	if c.DecayIntervalHours > 0 {
		cfg.DecayIntervalHours = c.DecayIntervalHours
	}
	if c.MaxMemories > 0 {
		cfg.MaxMemories = c.MaxMemories
	}
	if c.TieredLoadingEnabled != nil {
		cfg.TieredLoadingEnabled = c.TieredLoadingEnabled
	}
	if c.EmbeddingProvider != "" {
		cfg.EmbeddingProvider = c.EmbeddingProvider
	}
	if c.EmbeddingModel != "" {
		cfg.EmbeddingModel = c.EmbeddingModel
	}
	if c.EmbeddingBaseURL != "" {
		cfg.EmbeddingBaseURL = c.EmbeddingBaseURL
	}
	if c.CompressionTriggerPercent > 0 {
		cfg.CompressionTriggerPercent = c.CompressionTriggerPercent
	}
	if c.ObservationMaskTurns > 0 {
		cfg.ObservationMaskTurns = c.ObservationMaskTurns
	}
	if c.KeepRecentMessages > 0 {
		cfg.KeepRecentMessages = c.KeepRecentMessages
	}
	return cfg
}

// initUHMSVectorBackend initializes and injects the vector search backend into the UHMS manager.
// Called only when VectorMode != off. Failures are non-fatal (graceful degradation to FTS5-only).
func initUHMSVectorBackend(mgr *uhms.DefaultManager, uhmsCfg uhms.UHMSConfig, fullCfg *types.OpenAcosmiConfig) {
	// 1. Build embedding provider.
	embProvider := uhmsCfg.EmbeddingProvider
	if embProvider == "" || embProvider == "auto" {
		embProvider = "openai" // default
	}

	// Resolve API key from provider config.
	apiKey := ""
	if fullCfg != nil && fullCfg.Models != nil && fullCfg.Models.Providers != nil {
		if pc := fullCfg.Models.Providers[embProvider]; pc != nil {
			apiKey = pc.APIKey
		}
		// Fallback: try OpenAI key if embedding provider key not found.
		if apiKey == "" && embProvider != "openai" {
			if pc := fullCfg.Models.Providers["openai"]; pc != nil {
				apiKey = pc.APIKey
			}
		}
	}

	embedder, err := vectoradapter.NewHTTPEmbeddingProvider(embProvider, uhmsCfg.EmbeddingModel, uhmsCfg.EmbeddingBaseURL, apiKey)
	if err != nil {
		slog.Warn("gateway: UHMS embedding provider init failed (non-fatal)", "error", err)
		return
	}

	// 2. Build vector index.
	vectorDataDir := filepath.Join(uhmsCfg.ResolvedVFSPath(), "segment-vectors")
	vecIdx, err := vectoradapter.NewSegmentVectorIndex(vectorDataDir, embedder.Dimension())
	if err != nil {
		slog.Warn("gateway: UHMS vector index init failed (non-fatal)", "error", err)
		return
	}

	// 3. Inject into manager.
	mgr.SetVectorBackend(vecIdx, embedder)
	slog.Info("gateway: UHMS vector backend activated",
		"mode", uhmsCfg.VectorMode,
		"dimension", embedder.Dimension(),
		"embeddingProvider", embProvider,
	)
}

// buildUHMSLLMAdapter constructs an LLM adapter for UHMS from explicit config.
// Returns nil if no provider is configured — UHMS LLM features degrade gracefully.
// Used at boot and by memory.uhms.llm.set RPC for hot-swap.
func buildUHMSLLMAdapter(uhmsCfg *types.MemoryUHMSConfig, fullCfg *types.OpenAcosmiConfig) uhms.LLMProvider {
	if uhmsCfg == nil || uhmsCfg.LLMProvider == "" {
		return nil
	}

	provider := uhmsCfg.LLMProvider
	model := uhmsCfg.LLMModel
	if model == "" {
		model = defaultModelForProvider(provider)
	}
	baseURL := uhmsCfg.LLMBaseURL

	// UHMS 独立 API key 优先, 否则从 agent providers 查找
	apiKey := uhmsCfg.LLMApiKey
	if apiKey == "" && fullCfg != nil && fullCfg.Models != nil && fullCfg.Models.Providers != nil {
		if pc := fullCfg.Models.Providers[provider]; pc != nil {
			apiKey = pc.APIKey
		}
	}

	return &uhms.LLMClientAdapter{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}
}

// defaultModelForProvider returns a sensible default model for common LLM providers.
func defaultModelForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return "claude-sonnet-4-5-20250514"
	case "openai":
		return "gpt-4o-mini"
	case "ollama":
		return "llama3.2"
	case "deepseek":
		return "deepseek-chat"
	case "google":
		return "gemini-2.0-flash"
	case "groq":
		return "llama-3.3-70b-versatile"
	case "mistral":
		return "mistral-small-latest"
	case "together":
		return "meta-llama/Llama-3.3-70B-Instruct-Turbo"
	case "openrouter":
		return "anthropic/claude-sonnet-4"
	default:
		return "default"
	}
}

// StartGatewayServer 启动网关服务。
// 这是核心启动函数，将所有 Phase 0-9 实现的子系统组装起来。
func StartGatewayServer(port int, opts GatewayServerOptions) (*GatewayRuntime, error) {
	slog.Info("gateway: starting", "port", port)

	// ---------- 1. 创建运行时状态 ----------
	state := NewGatewayState()
	state.SetPhase(BootPhaseStarting)

	// ---------- 1b. 启用文件日志 ----------
	applog.EnableFileLogging("")
	logFilePath := applog.DefaultRollingPath()

	// ---------- 2. 解析认证配置 ----------
	auth := ResolveGatewayAuth(nil, "")

	// 校验配置
	bootCfg := BootConfig{
		Server: ServerConfig{
			Host: ResolveGatewayBindHost(opts.BindMode, opts.BindHost),
			Port: port,
		},
		Auth:   auth,
		Reload: DefaultReloadSettings,
	}
	if err := ValidateBootConfig(bootCfg); err != nil {
		// 认证未配置时，允许本地连接（开发模式）
		slog.Warn("gateway: auth config issue (local access still allowed)", "error", err)
	}

	// ---------- 2b. DI 注入: channel dock → autoreply ----------
	// 将 channels 包函数注入到 autoreply/reply DI 变量，
	// 避免 autoreply → channels 循环依赖。
	autoreply.NativeCommandSurfaceProvider = func() []string {
		ids := channels.ListNativeCommandChannels()
		result := make([]string, len(ids))
		for i, id := range ids {
			result[i] = strings.ToLower(string(id))
		}
		return result
	}
	reply.PluginDebounceProvider = func(channelKey string) *int {
		return channels.GetPluginDebounce(channels.ChannelID(channelKey))
	}
	reply.BlockStreamingCoalesceDefaultsProvider = func(channelKey string) (int, int) {
		return channels.GetBlockStreamingCoalesceDefaults(channels.ChannelID(channelKey))
	}

	// ---------- 3. 创建方法注册表 ----------
	registry := NewMethodRegistry()
	storePath := resolveDefaultStorePath()
	sessionStore := NewSessionStore(storePath)

	// 注册会话方法 (对齐 TS server-methods-list.ts)
	registry.RegisterAll(map[string]GatewayMethodHandler{
		"sessions.list":    handleSessionsList,
		"sessions.preview": handleSessionsPreview,
		"sessions.resolve": handleSessionsResolve,
		"sessions.patch":   handleSessionsPatch,
		"sessions.reset":   handleSessionsReset,
		"sessions.delete":  handleSessionsDelete,
		"sessions.compact": handleSessionsCompact,
	})

	// 注册 Batch A 方法 (config/models/agents/agent)
	registry.RegisterAll(ConfigHandlers())
	registry.RegisterAll(ModelsHandlers())
	registry.RegisterAll(AgentsHandlers())
	registry.RegisterAll(AgentHandlers())

	// 注册 Batch C 方法 (channels/logs/system)
	registry.RegisterAll(ChannelsHandlers())
	registry.RegisterAll(LogsHandlers())
	registry.RegisterAll(SystemHandlers())

	// 注册 Batch D-W1 方法 (cron/tts/skills/node/device/voicewake/update/browser/talk/web)
	registry.RegisterAll(CronHandlers())
	registry.RegisterAll(TtsHandlers())
	registry.RegisterAll(SkillsHandlers())
	registry.RegisterAll(NodeHandlers())
	registry.RegisterAll(DeviceHandlers())
	registry.RegisterAll(VoiceWakeHandlers())
	registry.RegisterAll(UpdateHandlers())
	registry.RegisterAll(BrowserHandlers())
	registry.RegisterAll(TalkHandlers())
	registry.RegisterAll(WebHandlers())

	// 注册 Batch FE-C 方法 (agents.files/sessions.usage/exec.approvals)
	registry.RegisterAll(AgentFilesHandlers())
	registry.RegisterAll(UsageHandlers())
	registry.RegisterAll(ExecApprovalsHandlers())
	registry.RegisterAll(SecurityHandlers())
	registry.RegisterAll(EscalationHandlers())
	registry.RegisterAll(RulesHandlers())          // P3: Allow/Ask/Deny 命令规则 CRUD
	registry.RegisterAll(RemoteApprovalHandlers()) // P4: 远程审批
	registry.RegisterAll(TaskPresetHandlers())     // P5: 任务预设权限
	RegisterSandboxMethods(registry)               // 沙箱配置 + 状态 + 测试
	registry.RegisterAll(ArgusHandlers())          // Argus 视觉子智能体静态方法
	registry.RegisterAll(MCPRemoteHandlers())      // P2: MCP 远程工具方法
	registry.RegisterAll(UHMSHandlers())           // P3: UHMS 记忆系统方法
	registry.RegisterAll(MemoryHandlers())         // memory.* 直接操作方法
	registry.RegisterAll(STTHandlers())            // Phase C: STT 配置方法
	registry.RegisterAll(DocConvHandlers())        // Phase D: 文档转换方法
	if state.ArgusBridge() != nil {
		RegisterArgusDynamicMethods(registry, state.ArgusBridge()) // Argus 动态工具方法
	}

	// Coder 确认流 RPC
	registry.Register("coder.confirm.resolve", func(ctx *MethodHandlerContext) {
		id, _ := ctx.Params["id"].(string)
		decision, _ := ctx.Params["decision"].(string)
		if id == "" || decision == "" {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing id or decision"))
			return
		}
		if ctx.Context.CoderConfirmMgr == nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "coder confirmation not enabled"))
			return
		}
		if err := ctx.Context.CoderConfirmMgr.ResolveConfirmation(id, decision); err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, err.Error()))
			return
		}
		ctx.Respond(true, map[string]interface{}{"ok": true}, nil)
	})

	// 注册 Batch B 方法 (chat/send/agent)
	registry.RegisterAll(ChatHandlers())
	registry.RegisterAll(SendHandlers())
	registry.RegisterAll(AgentRPCHandlers())

	// 注册 health 方法
	registry.Register("health", func(ctx *MethodHandlerContext) {
		ctx.Respond(true, GetHealthStatus(state, cli.Version), nil)
	})

	// 注册 status 方法 (精简版)
	registry.Register("status", func(ctx *MethodHandlerContext) {
		ctx.Respond(true, map[string]interface{}{
			"phase":   string(state.Phase()),
			"version": cli.Version,
			"clients": state.Broadcaster().ClientCount(),
		}, nil)
	})

	// ---------- 4. 创建配置加载器和模型目录 ----------
	cfgLoader := config.NewConfigLoader()
	modelCatalog := models.NewModelCatalog()

	// 尝试加载配置以填充模型目录
	var loadedCfg *types.OpenAcosmiConfig
	if cfg, err := cfgLoader.LoadConfig(); err == nil {
		loadedCfg = cfg
	}

	// ---------- 4a-wizard. 注册 Wizard 方法（替代 stub） ----------
	wizardTracker := NewWizardSessionTracker()
	registry.RegisterAll(WizardHandlers(WizardHandlerDeps{
		Tracker:      wizardTracker,
		ConfigLoader: cfgLoader,
		ModelCatalog: modelCatalog,
		State:        state,
	}))

	// ---------- 4b. Batch C 基础设施 ----------
	presenceStore := NewSystemPresenceStore()
	heartbeatState := NewHeartbeatState()
	eventQueue := NewSystemEventQueue()

	// ---------- 4d. 创建 AttemptRunner ----------
	// 注意: attemptRunner 的 Config 字段在 dispatcher 中动态刷新
	// 构建 Argus Bridge 适配器（nil-safe: bridge 不可用时 adapter 也为 nil）
	var argusBridgeForAgent runner.ArgusBridgeForAgent
	if ab := state.ArgusBridge(); ab != nil {
		argusBridgeForAgent = &argusBridgeAdapter{bridge: ab}
	}

	// 构建 Coder Bridge 适配器（nil-safe: bridge 不可用时 adapter 也为 nil）
	var coderBridgeForAgent runner.CoderBridgeForAgent
	if cb := state.CoderBridge(); cb != nil {
		coderBridgeForAgent = &coderBridgeAdapter{bridge: cb}
	}

	// 构建 NativeSandbox 适配器（nil-safe: bridge 不可用时 adapter 也为 nil）
	var nativeSandboxForAgent runner.NativeSandboxForAgent
	if nsb := state.NativeSandboxBridge(); nsb != nil {
		nativeSandboxForAgent = &nativeSandboxBridgeAdapter{bridge: nsb}
	}

	// 构建 UHMS Bridge 适配器（nil-safe: manager 不可用时 adapter 也为 nil）
	// 如果 UHMS 配置启用但 boot 时未传 LLM，在此处注入 LLM adapter
	var uhmsBridgeForAgent runner.UHMSBridgeForAgent
	if mgr := state.UHMSManager(); mgr != nil {
		uhmsBridgeForAgent = &uhmsBridgeAdapter{mgr: mgr, broadcaster: state.Broadcaster()}
	} else if loadedCfg != nil && loadedCfg.Memory != nil && loadedCfg.Memory.UHMS != nil && loadedCfg.Memory.UHMS.Enabled {
		// boot.go 使用 DefaultUHMSConfig 初始化（默认 disabled），
		// 这里从真实配置读取并重新初始化
		uhmsCfg := configToUHMSConfig(loadedCfg.Memory.UHMS)

		// 构建 LLM adapter: 优先使用 UHMS 独立配置，fallback 到 agent provider
		llmProvider := buildUHMSLLMAdapter(loadedCfg.Memory.UHMS, loadedCfg)

		mgr, err := uhms.NewManager(uhmsCfg, llmProvider)
		if err != nil {
			slog.Warn("gateway: UHMS init from config failed (non-fatal)", "error", err)
		} else {
			// 如果 LLM provider 是 Anthropic，注入 Compaction API client
			if llmProvider != nil {
				if adapter, ok := llmProvider.(*uhms.LLMClientAdapter); ok && strings.ToLower(adapter.Provider) == "anthropic" {
					mgr.SetCompactionClient(&uhms.AnthropicCompactionClient{
						APIKey: adapter.APIKey,
					})
					slog.Info("gateway: UHMS Anthropic Compaction API enabled")
				}
			}
			state.SetUHMSManager(mgr)
			uhmsBridgeForAgent = &uhmsBridgeAdapter{mgr: mgr, broadcaster: state.Broadcaster()}

			// Inject vector backend when VectorMode != off.
			if uhmsCfg.VectorMode != uhms.VectorOff {
				initUHMSVectorBackend(mgr, uhmsCfg, loadedCfg)
			}

			slog.Info("gateway: UHMS memory system initialized from config",
				"vectorMode", uhmsCfg.VectorMode,
				"vfsPath", uhmsCfg.ResolvedVFSPath(),
			)
		}
	}

	attemptRunner := &runner.EmbeddedAttemptRunner{
		Config:            loadedCfg,               // 初始值，dispatch 时会被热更新
		AuthStore:         nil,                     // Phase 10 暂不集成，回退到环境变量
		ArgusBridge:       argusBridgeForAgent,     // Argus 视觉工具注入
		CoderBridge:       coderBridgeForAgent,     // Coder 编程工具注入
		NativeSandbox:     nativeSandboxForAgent,   // 原生沙箱 Worker 注入
		UHMSBridge:        uhmsBridgeForAgent,      // UHMS 记忆系统注入
		CoderConfirmation: state.CoderConfirmMgr(), // Coder 确认流注入
		// RemoteMCPBridge 在 4h 节之后注入
	}

	// ---------- 4e. 创建真实 PipelineDispatcher（内联实现，避免循环导入） ----------
	// 这是 DI 回调函数，`chat.send` → `DispatchInboundMessage` → `GetReplyFromConfig`
	// 每次调用时从 cfgLoader 热加载最新配置，确保向导保存的配置立即生效。
	pipelineDispatcher := func(ctx context.Context, msgCtx *autoreply.MsgContext, opts *autoreply.GetReplyOptions) ([]autoreply.ReplyPayload, error) {
		// 0. 热加载最新配置（向导保存后立即生效）
		currentCfg := loadedCfg
		if freshCfg, err := cfgLoader.LoadConfig(); err == nil {
			currentCfg = freshCfg
			// 同步更新 attemptRunner 的配置引用
			attemptRunner.Config = currentCfg
		} else {
			slog.Warn("pipeline: config reload failed, using cached", "error", err)
		}

		// 1. 创建 AgentExecutor: ModelFallbackExecutor 需要 RunnerDeps + Config
		executor := &reply.ModelFallbackExecutor{
			RunnerDeps: runner.EmbeddedRunDeps{
				AttemptRunner: attemptRunner,
				ModelResolver: &runner.EnvModelResolver{Catalog: modelCatalog},
				// AuthStore, CompactionRunner 暂留 nil — fallback 到默认行为
			},
			Config: currentCfg,
			OnPermissionDenied: func(tool, detail string) {
				bc := state.Broadcaster()
				if bc != nil {
					// 读取安全等级（与前端 PermissionDeniedEvent 契约一致）
					level := "deny"
					if currentCfg != nil && currentCfg.Tools != nil && currentCfg.Tools.Exec != nil && currentCfg.Tools.Exec.Security != "" {
						level = currentCfg.Tools.Exec.Security
					}
					bc.Broadcast("permission_denied", map[string]interface{}{
						"tool":   tool,
						"detail": detail,
						"level":  level,
						"ts":     time.Now().UnixMilli(),
					}, nil)
				}

				// 自动触发提权请求（幂等，重复调用安全）
				escMgr := state.EscalationMgr()
				if escMgr != nil {
					escLevel := "allowlist"
					if tool == "bash" || tool == "write_file" {
						escLevel = "full"
					}
					reason := fmt.Sprintf("工具 %s 需要权限: %s", tool, truncateStr(detail, 200))
					escId := fmt.Sprintf("auto_esc_%d", time.Now().UnixNano())
					if err := escMgr.RequestEscalation(escId, escLevel, reason, "", "", msgCtx.ChannelID, msgCtx.SenderID, 30); err != nil {
						slog.Debug("auto-escalation skipped (expected if already pending)", "error", err)
					}
				}
			},
			// WaitForApproval 阻塞等待提权审批。
			// 轮询 EscalationManager 状态，直到出现 active grant（审批通过）、
			// pending 清除（被拒绝/超时），或 ctx 取消。
			WaitForApproval: func(ctx context.Context) bool {
				escMgr := state.EscalationMgr()
				if escMgr == nil {
					return false
				}
				const pollInterval = 2 * time.Second
				const maxWait = 10 * time.Minute
				deadline := time.After(maxWait)
				ticker := time.NewTicker(pollInterval)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return false
					case <-deadline:
						slog.Warn("WaitForApproval: max wait exceeded")
						return false
					case <-ticker.C:
						status := escMgr.GetStatus()
						if status.HasActive {
							// 审批已通过
							return true
						}
						if !status.HasPending && !status.HasActive {
							// 既无 pending 也无 active — 被拒绝或超时
							return false
						}
						// 仍有 pending — 继续等待
					}
				}
			},
			// SecurityLevelFunc 动态返回有效安全级别（含临时提权）
			SecurityLevelFunc: func() string {
				escMgr := state.EscalationMgr()
				if escMgr != nil {
					return escMgr.GetEffectiveLevel()
				}
				// fallback 到静态配置
				if currentCfg != nil && currentCfg.Tools != nil && currentCfg.Tools.Exec != nil {
					return currentCfg.Tools.Exec.Security
				}
				return "deny"
			},
		}
		// 2. 构建 reply 层选项（AgentID/SessionKey 等由 MsgContext 在 GetReplyFromConfig 内部推导）
		replyOpts := &reply.GetReplyOptions{
			AgentExecutor: executor,
		}
		return reply.GetReplyFromConfig(ctx, msgCtx, opts, replyOpts)
	}

	// ---------- 4c. 初始化中国频道插件 ----------
	// Phase 5: 从 config 读取频道配置，初始化并启动已配置的频道插件
	channelMgr := state.ChannelMgr()
	if !config.SkipChannels && loadedCfg != nil && loadedCfg.Channels != nil {
		// 注册插件
		if loadedCfg.Channels.Feishu != nil {
			feishuPlugin := feishu.NewFeishuPlugin(loadedCfg)

			// 公共飞书消息分发逻辑（DispatchFunc 和 DispatchMultimodalFunc 共用）
			feishuDispatch := func(channel, accountID, chatID, userID, text string) string {
				sessionKey := fmt.Sprintf("feishu:%s", chatID)

				// ===== 步骤 1: 确保 session 注册到 SessionStore =====
				var resolvedSessionId string
				if sessionStore != nil {
					entry := sessionStore.LoadSessionEntry(sessionKey)
					if entry == nil {
						newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
						entry = &SessionEntry{
							SessionKey: sessionKey,
							SessionId:  newId,
							Label:      fmt.Sprintf("飞书:%s", chatID),
							Channel:    "feishu",
						}
						sessionStore.Save(entry)
						slog.Info("feishu: auto-created session", "sessionKey", sessionKey, "sessionId", newId)
					}
					resolvedSessionId = entry.SessionId
					sessionStore.RecordSessionMeta(sessionKey, InboundMeta{
						Channel:     "feishu",
						DisplayName: userID,
					})
				}

				// ===== 步骤 2: 持久化用户消息到 transcript =====
				if resolvedSessionId != "" {
					AppendUserTranscriptMessage(AppendTranscriptParams{
						Message:         text,
						SessionID:       resolvedSessionId,
						StorePath:       storePath,
						CreateIfMissing: true,
					})
				}

				msgCtx := &autoreply.MsgContext{
					Body:        text,
					ChannelType: channel,
					ChannelID:   chatID,
					SenderID:    userID,
					AccountID:   accountID,
					SessionKey:  sessionKey,
				}

				// 广播用户消息到前端（让聊天页面能看到飞书会话）
				bc := state.Broadcaster()
				if bc != nil {
					ts := time.Now().UnixMilli()
					bc.Broadcast("chat.message", map[string]interface{}{
						"sessionKey": sessionKey,
						"channel":    "feishu",
						"role":       "user",
						"text":       text,
						"from":       userID,
						"chatId":     chatID,
						"ts":         ts,
					}, nil)

					// 跨会话通知：让所有前端客户端感知到飞书消息
					bc.Broadcast("channel.message.incoming", map[string]interface{}{
						"sessionKey": sessionKey,
						"channel":    "feishu",
						"text":       truncateStr(text, 100),
						"from":       userID,
						"label":      fmt.Sprintf("飞书:%s", chatID),
						"ts":         ts,
					}, nil)
				}

				result := DispatchInboundMessage(context.Background(), DispatchInboundParams{
					MsgCtx:     msgCtx,
					SessionKey: sessionKey,
					Dispatcher: pipelineDispatcher,
				})

				var replyText string
				if result.Error != nil {
					slog.Error("feishu dispatch error", "error", result.Error, "chatID", chatID)
					replyText = fmt.Sprintf("⚠️ 处理失败: %s", result.Error.Error())
				} else {
					replyText = CombineReplyPayloads(result.Replies)
				}

				// ===== 步骤 4: 持久化 AI 回复到 transcript =====
				if resolvedSessionId != "" && replyText != "" {
					AppendAssistantTranscriptMessage(AppendTranscriptParams{
						Message:         replyText,
						SessionID:       resolvedSessionId,
						StorePath:       storePath,
						CreateIfMissing: true,
					})
				}

				// 广播 AI 回复到前端
				if bc != nil && replyText != "" {
					bc.Broadcast("chat.message", map[string]interface{}{
						"sessionKey": sessionKey,
						"channel":    "feishu",
						"role":       "assistant",
						"text":       replyText,
						"chatId":     chatID,
						"ts":         time.Now().UnixMilli(),
					}, nil)
				}

				return replyText
			}

			feishuPlugin.DispatchFunc = func(ctx context.Context, channel, accountID, chatID, userID, text string) string {
				return feishuDispatch(channel, accountID, chatID, userID, text)
			}

			// 多模态分发：预处理附件（STT 转录 + 文档转换 + 图片下载），然后走公共分发
			feishuPreprocessor := &MultimodalPreprocessor{}
			if loadedCfg.STT != nil && loadedCfg.STT.Provider != "" {
				if prov, err := media.NewSTTProvider(loadedCfg.STT); err == nil {
					feishuPreprocessor.STTProvider = prov
					slog.Info("multimodal: STT provider loaded", "provider", prov.Name())
				} else {
					slog.Warn("multimodal: STT provider init failed (non-fatal)", "error", err)
				}
			}
			if loadedCfg.DocConv != nil && loadedCfg.DocConv.Provider != "" {
				if conv, err := media.NewDocConverter(loadedCfg.DocConv); err == nil {
					feishuPreprocessor.DocConverter = conv
					slog.Info("multimodal: DocConv provider loaded", "provider", conv.Name())
				} else {
					slog.Warn("multimodal: DocConv provider init failed (non-fatal)", "error", err)
				}
			}

			feishuPlugin.DispatchMultimodalFunc = func(channel, accountID, chatID, userID string, msg *channels.ChannelMessage) string {
				// M-01: 添加超时，防止 STT/DocConv 无限挂起
				preprocessCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()
				client := feishuPlugin.GetClient(accountID)
				result := feishuPreprocessor.ProcessFeishuMessage(preprocessCtx, client, msg)
				return feishuDispatch(channel, accountID, chatID, userID, result.Text)
			}

			// 注入卡片回传交互回调（审批按钮点击，走 WebSocket 长连接）
			feishuPlugin.CardActionFunc = buildFeishuCardActionHandler(state)

			channelMgr.RegisterPlugin(feishuPlugin)
			slog.Info("channel: feishu plugin registered")

			// Bug A fix: 自动注入飞书频道凭据到审批系统
			if rn := state.RemoteApprovalNotifier(); rn != nil {
				feishuCfg := loadedCfg.Channels.Feishu
				if feishuCfg.AppID != "" && feishuCfg.AppSecret != "" {
					rn.InjectChannelFeishuConfig(feishuCfg.AppID, feishuCfg.AppSecret)
				}
			}
		}
		if loadedCfg.Channels.DingTalk != nil {
			dingtalkPlugin := dingtalk.NewDingTalkPlugin(loadedCfg)
			dingtalkPlugin.DispatchFunc = func(ctx context.Context, channel, accountID, chatID, userID, text string) string {
				sessionKey := fmt.Sprintf("dingtalk:%s", chatID)

				// 步骤 1: session 注册
				var resolvedSessionId string
				if sessionStore != nil {
					entry := sessionStore.LoadSessionEntry(sessionKey)
					if entry == nil {
						newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
						entry = &SessionEntry{
							SessionKey: sessionKey,
							SessionId:  newId,
							Label:      fmt.Sprintf("钉钉:%s", chatID),
							Channel:    "dingtalk",
						}
						sessionStore.Save(entry)
						slog.Info("dingtalk: auto-created session", "sessionKey", sessionKey, "sessionId", newId)
					}
					resolvedSessionId = entry.SessionId
					sessionStore.RecordSessionMeta(sessionKey, InboundMeta{
						Channel:     "dingtalk",
						DisplayName: userID,
					})
				}

				// 步骤 2: 持久化用户消息
				if resolvedSessionId != "" {
					AppendUserTranscriptMessage(AppendTranscriptParams{
						Message:         text,
						SessionID:       resolvedSessionId,
						StorePath:       storePath,
						CreateIfMissing: true,
					})
				}

				msgCtx := &autoreply.MsgContext{
					Body:        text,
					ChannelType: channel,
					ChannelID:   chatID,
					SenderID:    userID,
					AccountID:   accountID,
					SessionKey:  sessionKey,
				}

				// 广播用户消息到前端
				bc := state.Broadcaster()
				if bc != nil {
					bc.Broadcast("chat.message", map[string]interface{}{
						"sessionKey": sessionKey,
						"channel":    "dingtalk",
						"role":       "user",
						"text":       text,
						"from":       userID,
						"chatId":     chatID,
						"ts":         time.Now().UnixMilli(),
					}, nil)
				}

				result := DispatchInboundMessage(ctx, DispatchInboundParams{
					MsgCtx:     msgCtx,
					SessionKey: sessionKey,
					Dispatcher: pipelineDispatcher,
				})

				var reply string
				if result.Error != nil {
					slog.Error("dingtalk dispatch error", "error", result.Error, "chatID", chatID)
					reply = fmt.Sprintf("⚠️ 处理失败: %s", result.Error.Error())
				} else {
					reply = CombineReplyPayloads(result.Replies)
				}

				// 步骤 4: 持久化 AI 回复
				if resolvedSessionId != "" && reply != "" {
					AppendAssistantTranscriptMessage(AppendTranscriptParams{
						Message:         reply,
						SessionID:       resolvedSessionId,
						StorePath:       storePath,
						CreateIfMissing: true,
					})
				}

				// 广播 AI 回复到前端
				if bc != nil && reply != "" {
					bc.Broadcast("chat.message", map[string]interface{}{
						"sessionKey": sessionKey,
						"channel":    "dingtalk",
						"role":       "assistant",
						"text":       reply,
						"chatId":     chatID,
						"ts":         time.Now().UnixMilli(),
					}, nil)
				}

				return reply
			}
			channelMgr.RegisterPlugin(dingtalkPlugin)
			slog.Info("channel: dingtalk plugin registered")
		}
		if loadedCfg.Channels.WeCom != nil {
			wecomPlugin := wecom.NewWeComPlugin(loadedCfg)
			wecomPlugin.DispatchFunc = func(ctx context.Context, channel, accountID, chatID, userID, text string) string {
				sessionKey := fmt.Sprintf("wecom:%s", chatID)

				// 步骤 1: session 注册
				var resolvedSessionId string
				if sessionStore != nil {
					entry := sessionStore.LoadSessionEntry(sessionKey)
					if entry == nil {
						newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
						entry = &SessionEntry{
							SessionKey: sessionKey,
							SessionId:  newId,
							Label:      fmt.Sprintf("企微:%s", chatID),
							Channel:    "wecom",
						}
						sessionStore.Save(entry)
						slog.Info("wecom: auto-created session", "sessionKey", sessionKey, "sessionId", newId)
					}
					resolvedSessionId = entry.SessionId
					sessionStore.RecordSessionMeta(sessionKey, InboundMeta{
						Channel:     "wecom",
						DisplayName: userID,
					})
				}

				// 步骤 2: 持久化用户消息
				if resolvedSessionId != "" {
					AppendUserTranscriptMessage(AppendTranscriptParams{
						Message:         text,
						SessionID:       resolvedSessionId,
						StorePath:       storePath,
						CreateIfMissing: true,
					})
				}

				msgCtx := &autoreply.MsgContext{
					Body:        text,
					ChannelType: channel,
					ChannelID:   chatID,
					SenderID:    userID,
					AccountID:   accountID,
					SessionKey:  sessionKey,
				}

				// 广播用户消息到前端
				bc := state.Broadcaster()
				if bc != nil {
					bc.Broadcast("chat.message", map[string]interface{}{
						"sessionKey": sessionKey,
						"channel":    "wecom",
						"role":       "user",
						"text":       text,
						"from":       userID,
						"chatId":     chatID,
						"ts":         time.Now().UnixMilli(),
					}, nil)
				}

				result := DispatchInboundMessage(ctx, DispatchInboundParams{
					MsgCtx:     msgCtx,
					SessionKey: sessionKey,
					Dispatcher: pipelineDispatcher,
				})

				var reply string
				if result.Error != nil {
					slog.Error("wecom dispatch error", "error", result.Error, "chatID", chatID)
					reply = fmt.Sprintf("⚠️ 处理失败: %s", result.Error.Error())
				} else {
					reply = CombineReplyPayloads(result.Replies)
				}

				// 步骤 4: 持久化 AI 回复
				if resolvedSessionId != "" && reply != "" {
					AppendAssistantTranscriptMessage(AppendTranscriptParams{
						Message:         reply,
						SessionID:       resolvedSessionId,
						StorePath:       storePath,
						CreateIfMissing: true,
					})
				}

				// 广播 AI 回复到前端
				if bc != nil && reply != "" {
					bc.Broadcast("chat.message", map[string]interface{}{
						"sessionKey": sessionKey,
						"channel":    "wecom",
						"role":       "assistant",
						"text":       reply,
						"chatId":     chatID,
						"ts":         time.Now().UnixMilli(),
					}, nil)
				}

				return reply
			}
			channelMgr.RegisterPlugin(wecomPlugin)
			slog.Info("channel: wecom plugin registered")
		}

		// 启动已配置的频道
		for _, chID := range []channels.ChannelID{channels.ChannelFeishu, channels.ChannelDingTalk, channels.ChannelWeCom} {
			if err := channelMgr.StartChannel(chID, channels.DefaultAccountID); err != nil {
				slog.Warn("channel: start failed (non-fatal)", "channel", chID, "error", err)
			}
		}

		// ---------- 4c-2. 启动 Monitor 模式渠道 (Discord/Telegram/Slack) ----------
		monitorDepsCtx := &ChannelDepsContext{
			StorePath:    storePath,
			State:        state,
			EventQueue:   eventQueue,
			Dispatcher:   pipelineDispatcher,
			SessionStore: sessionStore,
		}
		monitorCtx, monitorCancel := context.WithCancel(context.Background())
		_ = monitorCancel // 优雅关闭时调用
		startMonitorChannels(monitorCtx, monitorDepsCtx, loadedCfg, nil)
	}

	// ---------- 4f. 创建 CronService ----------
	cronStorePath := filepath.Join(storePath, "cron", "jobs.json")
	cronSvc := cron.NewCronService(cron.CronServiceDeps{
		StorePath: cronStorePath,
		Logger:    &slogCronLogger{},
		OnEvent: func(event cron.CronEvent) {
			bc := state.Broadcaster()
			if bc != nil {
				bc.Broadcast("cron.event", event, nil)
			}
		},
		EnqueueSystemEvent: func(text string) error {
			eventQueue.Enqueue(text, "main", "cron")
			return nil
		},
		RequestHeartbeatNow: func() {
			// 心跳唤醒 — 目前仅标记为可用
			heartbeatState.SetEnabled(true)
		},
	})
	if !config.SkipCron {
		if err := cronSvc.Start(); err != nil {
			slog.Warn("gateway: cron service start failed", "error", err)
		} else {
			slog.Info("gateway: cron service started")
		}
	}

	// ---------- 4g. 创建技能商店客户端 ----------
	var skillStoreClient *skills.SkillStoreClient
	if loadedCfg != nil && loadedCfg.Skills != nil && loadedCfg.Skills.Store != nil {
		store := loadedCfg.Skills.Store
		if store.URL != "" && store.Token != "" {
			skillStoreClient = skills.NewSkillStoreClient(store.URL, store.Token)
			slog.Info("gateway: skill store client configured", "url", store.URL)
		}
	}

	// ---------- 4h. 创建 MCP 远程工具 Bridge (P2) ----------
	var remoteMCPBridge *mcpremote.RemoteBridge
	if loadedCfg != nil && loadedCfg.Skills != nil && loadedCfg.Skills.Store != nil {
		store := loadedCfg.Skills.Store
		if store.MCP != nil && store.MCP.Enabled && store.URL != "" {
			// 计算 MCP 端点
			mcpEndpoint := store.MCP.Endpoint
			if mcpEndpoint == "" {
				mcpEndpoint = strings.TrimRight(store.URL, "/") + "/api/v4/mcp"
			}

			// 创建 Token Manager（P1 JWT 兼容模式）
			tokenMgr := mcpremote.NewOAuthTokenManager(mcpremote.OAuthConfig{
				StaticToken:  store.Token,
				OAuthEnabled: store.MCP.OAuthEnabled,
				IssuerURL:    store.URL,
			}, nil)

			remoteMCPBridge = mcpremote.NewRemoteBridge(mcpremote.RemoteBridgeConfig{
				Endpoint:     mcpEndpoint,
				TokenManager: tokenMgr,
			})

			// 异步启动连接（不阻塞网关启动）
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := remoteMCPBridge.Start(bgCtx); err != nil {
					slog.Warn("gateway: MCP remote bridge start failed (non-fatal)", "error", err, "endpoint", mcpEndpoint)
				}
			}()
			state.SetRemoteMCPBridge(remoteMCPBridge)
			slog.Info("gateway: MCP remote bridge initializing", "endpoint", mcpEndpoint)
		}
	}

	// 4h-b. 注入 Remote MCP Bridge 到 AttemptRunner（需在 4h 后）
	if remoteMCPBridge != nil {
		attemptRunner.RemoteMCPBridge = &remoteMCPBridgeAdapter{bridge: remoteMCPBridge}
	}

	// ---------- 5. 创建 HTTP 路由 ----------
	wsConfig := WsServerConfig{
		Auth:               auth,
		TrustedProxies:     bootCfg.TrustedProxies,
		State:              state,
		Registry:           registry,
		SessionStore:       sessionStore,
		StorePath:          storePath,
		Version:            cli.Version,
		LogFilePath:        logFilePath,
		ConfigLoader:       cfgLoader,
		ModelCatalog:       modelCatalog,
		PresenceStore:      presenceStore,
		HeartbeatState:     heartbeatState,
		EventQueue:         eventQueue,
		PipelineDispatcher: pipelineDispatcher,
		CronService:        cronSvc,
		CronStorePath:      filepath.Dir(cronStorePath),
		ChannelMgr:         channelMgr,
		SkillStoreClient:   skillStoreClient,
		RemoteMCPBridge:    remoteMCPBridge, // P2: MCP 远程工具
		BootedAt:           time.Now(),
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		SendJSON(w, http.StatusOK, GetHealthStatus(state, cli.Version))
	})

	// WebSocket 端点
	mux.HandleFunc("/ws", HandleWebSocketUpgrade(wsConfig))

	// Issue 4 fix: Register hooks/openai/tools routes directly on top-level mux
	// (Previously these were nested under /hooks/ via CreateGatewayHTTPHandler, causing
	// double-prefix like /hooks/hooks/ and nil callback panics.)
	httpCfg := GatewayHTTPHandlerConfig{
		ControlUIDir:   opts.ControlUIDir,
		TrustedProxies: bootCfg.TrustedProxies,
		GetHooksConfig: func() *HooksConfig {
			// Hooks require explicit enabled+token in config; return nil if not configured.
			// The hooksHandler already returns 404 when config is nil.
			if loadedCfg != nil && loadedCfg.Hooks != nil && loadedCfg.Hooks.Enabled != nil && *loadedCfg.Hooks.Enabled {
				raw := &HooksRawConfig{
					Enabled:  loadedCfg.Hooks.Enabled,
					Token:    loadedCfg.Hooks.Token,
					Path:     loadedCfg.Hooks.Path,
					Presets:  loadedCfg.Hooks.Presets,
					Mappings: convertHookMappings(loadedCfg.Hooks.Mappings),
				}
				if loadedCfg.Hooks.MaxBodyBytes != nil {
					raw.MaxBodyBytes = int64(*loadedCfg.Hooks.MaxBodyBytes)
				}
				cfg, err := ResolveHooksConfig(raw)
				if err != nil {
					slog.Warn("gateway: hooks config error", "error", err)
					return nil
				}
				return cfg
			}
			return nil
		},
		GetAuth: func() ResolvedGatewayAuth {
			return auth
		},
		PipelineDispatcher: pipelineDispatcher,
	}

	// Hooks handler (直接注册到顶层)
	hooksHandler := createHooksHTTPHandler(httpCfg)
	mux.HandleFunc("/hooks/", hooksHandler)

	// OpenAI Chat Completions API (直接注册到顶层)
	openaiCfg := OpenAIChatHandlerConfig{
		GetAuth:        httpCfg.GetAuth,
		Dispatcher:     httpCfg.PipelineDispatcher,
		TrustedProxies: httpCfg.TrustedProxies,
	}
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		HandleOpenAIChatCompletions(w, r, openaiCfg)
	})
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		HandleOpenAIResponses(w, r, openaiCfg)
	})

	// Tools invoke (直接注册到顶层)
	toolsCfg := ToolsInvokeHandlerConfig{
		GetAuth:   httpCfg.GetAuth,
		Invoker:   httpCfg.ToolInvoker,
		ToolNames: httpCfg.ToolNames,
	}
	mux.HandleFunc("/tools/invoke/", func(w http.ResponseWriter, r *http.Request) {
		HandleToolsInvoke(w, r, toolsCfg)
	})

	// Control UI 静态文件
	if opts.ControlUIDir != "" {
		fs := http.FileServer(http.Dir(opts.ControlUIDir))
		mux.Handle("/ui/", http.StripPrefix("/ui/", fs))
	}

	// Phase 5: 频道 webhook HTTP 路由
	mux.HandleFunc("/channels/feishu/webhook", ChannelWebhookFeishu(channelMgr))
	mux.HandleFunc("/channels/wecom/callback", ChannelWebhookWeCom(channelMgr))

	// Issue 6: Root path handler — prevents 404 for "/"
	hasControlUI := opts.ControlUIDir != ""
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only handle exact "/" path; net/http routes unmatched paths to "/"
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if hasControlUI {
			http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
			return
		}
		SendJSON(w, http.StatusOK, map[string]interface{}{
			"name":    "OpenAcosmi Gateway",
			"version": cli.Version,
			"status":  "ok",
		})
	})

	// ---------- 5. 创建并启动 HTTP 服务器 ----------
	serverCfg := ServerConfig{
		Host:           bootCfg.Server.Host,
		Port:           port,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   0, // SSE + WS 需要无限写超时
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	httpServer := NewGatewayHTTPServer(serverCfg, mux)

	// ---------- 5a. 功能开关（SKIP_* 系列）----------
	// 对应 TS server.impl.ts 启动顺序第 4-7 步的 OPENACOSMI_SKIP_* 控制逻辑。
	// 各子系统在未来接入时须先检查对应 flag 再执行 Start()。
	if config.SkipCron {
		slog.Info("gateway: OPENACOSMI_SKIP_CRON set — cron scheduler will not start")
	}
	if config.SkipChannels {
		slog.Info("gateway: OPENACOSMI_SKIP_CHANNELS set — channel subsystem will not start")
	}
	if config.SkipBrowserControl {
		slog.Info("gateway: OPENACOSMI_SKIP_BROWSER_CONTROL_SERVER set — browser control server will not start")
	}
	if config.SkipCanvasHost {
		slog.Info("gateway: OPENACOSMI_SKIP_CANVAS_HOST set — canvas host will not start")
	}
	if config.SkipProviders {
		slog.Info("gateway: OPENACOSMI_SKIP_PROVIDERS set — provider initialization skipped")
	}

	// ---------- 5b. 启动维护计时器（gateway.tick 广播） ----------
	// 对齐 TS server-maintenance.ts: 每 30s 广播 tick 事件
	maintenanceTimers := StartMaintenanceTick(state.Broadcaster())

	runtime := &GatewayRuntime{
		State:             state,
		HTTPServer:        httpServer,
		MaintenanceTimers: maintenanceTimers,
	}

	// 启动 HTTP 监听
	go func() {
		listenAddr := fmt.Sprintf("%s:%d", serverCfg.Host, serverCfg.Port)
		slog.Info("🦜 Gateway listening", "addr", listenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway: http server error", "error", err)
			os.Exit(1)
		}
	}()

	// 等待服务器就绪
	time.Sleep(100 * time.Millisecond)
	state.SetPhase(BootPhaseReady)
	slog.Info("gateway: ready", "port", port)

	// 打印 Dashboard URL（Jupyter 模式：终端可点击直接打开）
	if auth.Token != "" {
		dashURL := fmt.Sprintf("http://localhost:%d/?token=%s", port, auth.Token)
		slog.Info("🔑 Dashboard URL (copy to browser)", "url", dashURL)
	}

	return runtime, nil
}

// RunGatewayBlocking 启动网关并阻塞直到收到终止信号。
func RunGatewayBlocking(port int, opts GatewayServerOptions) error {
	runtime, err := StartGatewayServer(port, opts)
	if err != nil {
		return err
	}

	// 等待终止信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	slog.Info("gateway: received signal", "signal", sig)

	return runtime.Close(fmt.Sprintf("signal: %s", sig))
}

// resolveDefaultStorePath 解析默认存储路径。
// 顺序: $OPENACOSMI_STORE_PATH → ~/.openacosmi/store
func resolveDefaultStorePath() string {
	if v := os.Getenv("OPENACOSMI_STORE_PATH"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("gateway: cannot resolve home dir, using ./store", "error", err)
		return "./store"
	}
	return filepath.Join(home, ".openacosmi", "store")
}

// convertHookMappings 将 types.HookMappingConfig 列表转换为 gateway.HookMappingConfig 列表。
func convertHookMappings(mappings []types.HookMappingConfig) []HookMappingConfig {
	if len(mappings) == 0 {
		return nil
	}
	result := make([]HookMappingConfig, len(mappings))
	for i, m := range mappings {
		r := HookMappingConfig{
			ID:                         m.ID,
			Action:                     m.Action,
			WakeMode:                   m.WakeMode,
			MessageTemplate:            m.MessageTemplate,
			TextTemplate:               m.TextTemplate,
			Name:                       m.Name,
			SessionKey:                 m.SessionKey,
			Channel:                    m.Channel,
			To:                         m.To,
			Model:                      m.Model,
			Deliver:                    m.Deliver,
			Thinking:                   m.Thinking,
			AllowUnsafeExternalContent: m.AllowUnsafeExternalContent,
		}
		if m.Match != nil {
			r.Match = &HookMatchFieldConfig{
				Path:   m.Match.Path,
				Source: m.Match.Source,
			}
		}
		if m.TimeoutSeconds != nil {
			r.TimeoutSeconds = *m.TimeoutSeconds
		}
		if m.Transform != nil {
			r.TransformModule = m.Transform.Module
			r.TransformExport = m.Transform.Export
		}
		result[i] = r
	}
	return result
}

// slogCronLogger 将 cron.CronLogger 接口适配到标准 slog。
type slogCronLogger struct{}

func (l *slogCronLogger) Info(msg string, fields ...interface{}) { slog.Info("cron: "+msg, fields...) }
func (l *slogCronLogger) Warn(msg string, fields ...interface{}) { slog.Warn("cron: "+msg, fields...) }
func (l *slogCronLogger) Error(msg string, fields ...interface{}) {
	slog.Error("cron: "+msg, fields...)
}
func (l *slogCronLogger) Debug(msg string, fields ...interface{}) {
	slog.Debug("cron: "+msg, fields...)
}

// buildFeishuCardActionHandler 构建飞书卡片回传交互处理器。
// 处理审批卡片按钮点击（批准/拒绝），通过 WebSocket 长连接接收，无需公网地址。
func buildFeishuCardActionHandler(state *GatewayState) feishu.CardActionHandler {
	return func(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
		if event == nil || event.Event == nil || event.Event.Action == nil {
			slog.Warn("feishu card action: missing event data")
			return &callback.CardActionTriggerResponse{
				Toast: &callback.Toast{Type: "error", Content: "回调数据异常"},
			}, nil
		}

		value := event.Event.Action.Value
		actionStr, _ := value["action"].(string)
		escalationID, _ := value["id"].(string)

		if escalationID == "" {
			slog.Warn("feishu card action: missing escalation ID")
			return &callback.CardActionTriggerResponse{
				Toast: &callback.Toast{Type: "error", Content: "审批 ID 缺失"},
			}, nil
		}

		escMgr := state.EscalationMgr()
		if escMgr == nil {
			slog.Warn("feishu card action: escalation manager not available")
			return &callback.CardActionTriggerResponse{
				Toast: &callback.Toast{Type: "error", Content: "审批系统未初始化"},
			}, nil
		}

		switch actionStr {
		case "approve":
			ttl := 30
			if ttlFloat, ok := value["ttl"].(float64); ok && ttlFloat > 0 {
				ttl = int(ttlFloat)
			}
			if err := escMgr.ResolveEscalation(true, ttl); err != nil {
				slog.Warn("feishu card action: approve failed", "id", escalationID, "error", err)
				return &callback.CardActionTriggerResponse{
					Toast: &callback.Toast{Type: "warning", Content: "审批失败: " + err.Error()},
				}, nil
			}
			slog.Info("feishu card action: approved", "id", escalationID, "ttl", ttl)
			return &callback.CardActionTriggerResponse{
				Toast: &callback.Toast{Type: "success", Content: "✅ 审批通过，权限已生效"},
			}, nil

		case "deny":
			if err := escMgr.ResolveEscalation(false, 0); err != nil {
				slog.Warn("feishu card action: deny failed", "id", escalationID, "error", err)
				return &callback.CardActionTriggerResponse{
					Toast: &callback.Toast{Type: "warning", Content: "拒绝失败: " + err.Error()},
				}, nil
			}
			slog.Info("feishu card action: denied", "id", escalationID)
			return &callback.CardActionTriggerResponse{
				Toast: &callback.Toast{Type: "info", Content: "❌ 已拒绝权限提升请求"},
			}, nil

		default:
			slog.Warn("feishu card action: unknown action", "action", actionStr)
			return &callback.CardActionTriggerResponse{
				Toast: &callback.Toast{Type: "error", Content: "未知操作: " + actionStr},
			}, nil
		}
	}
}
