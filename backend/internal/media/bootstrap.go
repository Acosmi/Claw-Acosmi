package media

// ============================================================================
// media/bootstrap.go — oa-media 子系统引导模块
// 将所有工具实例化 + 依赖注入封装为 NewMediaSubsystem()，
// 集成时只需一行调用即可接入主系统。
//
// Design doc: docs/xinshenji/impl-tracking-media-subagent.md §P2-6
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

// ---------- 子系统配置 ----------

// MediaSubsystemConfig 媒体子系统初始化配置。
type MediaSubsystemConfig struct {
	// Workspace 工作目录（用于 DraftStore 和错误截图等）。
	Workspace string

	// EnablePublish 启用发布工具（Phase 2+）。
	EnablePublish bool

	// EnableInteract 启用互动工具（Phase 3+）。
	EnableInteract bool

	// EnabledSources 启用的热点源名称列表（空=全部启用）。
	EnabledSources []string

	// Publishers 各平台发布器（按 Platform 注册）。
	// 由外部通过 RegisterPublisher() 注入。
	Publishers map[Platform]MediaPublisher
}

// ---------- 子系统 ----------

// MediaSubsystem 聚合 oa-media 的全部运行时组件。
type MediaSubsystem struct {
	mu             sync.RWMutex
	DraftStore     DraftStore
	Aggregator     *TrendingAggregator
	Publishers     map[Platform]MediaPublisher
	PublishHistory PublishHistoryStore
	StateStore     MediaStateStore
	Tools          []*MediaTool
	ToolsByName    map[string]*MediaTool
}

// NewMediaSubsystem 创建完整的媒体子系统。
// 初始化 DraftStore、TrendingAggregator、各工具实例。
func NewMediaSubsystem(cfg MediaSubsystemConfig) (*MediaSubsystem, error) {
	draftsDir := filepath.Join(cfg.Workspace, "_media", "drafts")
	store, err := NewFileDraftStore(draftsDir)
	if err != nil {
		return nil, err
	}

	var historyStore PublishHistoryStore
	if cfg.EnablePublish {
		historyDir := filepath.Join(cfg.Workspace, "_media", "publish_history")
		hs, err := NewFilePublishHistoryStore(historyDir)
		if err != nil {
			return nil, err
		}
		historyStore = hs
	}

	// 持久状态存储（跨会话记忆）
	stateDir := filepath.Join(cfg.Workspace, "_media")
	stateStore, err := NewFileMediaStateStore(stateDir)
	if err != nil {
		return nil, err
	}

	aggregator := NewTrendingAggregator()
	allSources := map[string]TrendingSource{
		"weibo": NewWeiboTrendingSource(),
		"baidu": NewBaiduTrendingSource(),
		"zhihu": NewZhihuTrendingSource(),
	}
	if cfg.EnabledSources != nil {
		// 显式配置了源列表（含空列表=全部禁用）
		for _, name := range cfg.EnabledSources {
			if src, ok := allSources[name]; ok {
				aggregator.AddSource(src)
			} else {
				slog.Warn("media: unknown trending source in EnabledSources, skipped", "source", name)
			}
		}
	} else {
		// 未配置（nil）= 全部启用
		for _, src := range allSources {
			aggregator.AddSource(src)
		}
	}

	publishers := cfg.Publishers
	if publishers == nil {
		publishers = make(map[Platform]MediaPublisher)
	}

	tools := buildMediaTools(cfg, store, aggregator, publishers, historyStore, stateStore)
	toolsByName := make(map[string]*MediaTool, len(tools))
	for _, t := range tools {
		toolsByName[t.ToolName] = t
	}

	slog.Info("media subsystem initialized",
		"tool_count", len(tools),
		"publish_enabled", cfg.EnablePublish,
		"interact_enabled", cfg.EnableInteract,
	)

	return &MediaSubsystem{
		DraftStore:     store,
		Aggregator:     aggregator,
		Publishers:     publishers,
		PublishHistory: historyStore,
		StateStore:     stateStore,
		Tools:          tools,
		ToolsByName:    toolsByName,
	}, nil
}

// buildMediaTools 根据配置构建工具实例列表。
func buildMediaTools(
	cfg MediaSubsystemConfig,
	store DraftStore,
	agg *TrendingAggregator,
	publishers map[Platform]MediaPublisher,
	history PublishHistoryStore,
	stateStore MediaStateStore,
) []*MediaTool {
	tools := []*MediaTool{
		CreateTrendingTool(agg, stateStore),
		CreateContentComposeTool(store),
	}
	if cfg.EnablePublish {
		tools = append(tools, CreateMediaPublishTool(store, publishers, history))
	}
	if cfg.EnableInteract {
		tools = append(tools, CreateSocialInteractTool(nil))
	}
	return tools
}

// RegisterPublisher 注册平台发布器。
func (s *MediaSubsystem) RegisterPublisher(platform Platform, pub MediaPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Publishers[platform] = pub
	slog.Info("media publisher registered", "platform", platform)
}

// RegisterInteractor 注册社交互动器。
// 替换初始化时以 nil 创建的 social_interact 工具实例。
// 若工具列表中不存在 social_interact（EnableInteract=false 时），则新增。
func (s *MediaSubsystem) RegisterInteractor(interactor SocialInteractor) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tool := CreateSocialInteractTool(interactor)
	for i, t := range s.Tools {
		if t.ToolName == ToolSocialInteract {
			s.Tools[i] = tool
			s.ToolsByName[ToolSocialInteract] = tool
			slog.Info("media social interactor registered (replaced)")
			return
		}
	}
	// 工具不存在，追加
	s.Tools = append(s.Tools, tool)
	s.ToolsByName[ToolSocialInteract] = tool
	slog.Info("media social interactor registered (added)")
}

// GetTool 按名字获取工具。
func (s *MediaSubsystem) GetTool(name string) *MediaTool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ToolsByName[name]
}

// ToolNames 返回所有已注册工具名列表。
func (s *MediaSubsystem) ToolNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.Tools))
	for _, t := range s.Tools {
		names = append(names, t.ToolName)
	}
	return names
}

// GetToolDef 返回 LLM 工具定义（inputSchema JSON + description）。
// 实现 runner.MediaSubsystemForAgent 接口。
func (s *MediaSubsystem) GetToolDef(name string) (json.RawMessage, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tool, ok := s.ToolsByName[name]
	if !ok {
		return nil, "", false
	}
	// ToolParams 是 map[string]any (JSON Schema)，需要序列化为 json.RawMessage
	// 在 RLock 保护范围内序列化，避免并发修改 map 导致竞态
	schema, err := json.Marshal(tool.ToolParams)
	if err != nil {
		slog.Warn("media tool schema marshal failed", "tool", name, "error", err)
		return nil, "", false
	}
	return json.RawMessage(schema), tool.ToolDesc, true
}

// ExecuteTool 执行媒体工具调用，返回文本结果。
// 实现 runner.MediaSubsystemForAgent 接口。
func (s *MediaSubsystem) ExecuteTool(ctx context.Context, name string, inputJSON json.RawMessage) (string, error) {
	s.mu.RLock()
	tool, ok := s.ToolsByName[name]
	s.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("[Media tool %q not found]", name), nil
	}
	if tool.ToolExecute == nil {
		return fmt.Sprintf("[Media tool %q has no executor]", name), nil
	}

	// 解析 inputJSON 为 map[string]any
	var args map[string]any
	if len(inputJSON) > 0 {
		if err := json.Unmarshal(inputJSON, &args); err != nil {
			return fmt.Sprintf("[Media tool %q: invalid input: %s]", name, err), nil
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	result, err := tool.ToolExecute(ctx, "", args)
	if err != nil {
		return fmt.Sprintf("[Media tool %q error: %s]", name, err), nil
	}
	if result == nil {
		return "[Media tool returned no result]", nil
	}

	// 提取文本内容
	var texts []string
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	if len(texts) == 0 {
		return "[Media tool returned empty result]", nil
	}
	return strings.Join(texts, "\n"), nil
}

// BuildSystemPrompt 构建媒体子智能体系统提示词。
// contractPrompt 为合约格式化文本（由 DelegationContract.FormatForSystemPrompt() 生成）。
// 实现 runner.MediaSubsystemForAgent 接口。
func (s *MediaSubsystem) BuildSystemPrompt(task, contractPrompt, sessionKey string) string {
	// 使用 contractPromptAdapter 将纯文本合约段适配为 ContractFormatter 接口
	var contract ContractFormatter
	if contractPrompt != "" {
		contract = &contractPromptAdapter{text: contractPrompt}
	}
	// 加载跨会话状态
	var state *MediaState
	if s.StateStore != nil {
		if st, err := s.StateStore.Load(); err == nil {
			state = st
		}
	}
	return BuildMediaSystemPrompt(MediaPromptParams{
		Task:                task,
		Contract:            contract,
		RequesterSessionKey: sessionKey,
		State:               state,
	})
}

// contractPromptAdapter 将纯文本合约段适配为 ContractFormatter 接口。
type contractPromptAdapter struct {
	text string
}

func (a *contractPromptAdapter) FormatForSystemPrompt() string {
	return a.text
}
