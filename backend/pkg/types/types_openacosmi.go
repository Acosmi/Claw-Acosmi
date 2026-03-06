package types

// OpenAcosmi 根配置类型 — 继承自 src/config/types.openacosmi.ts (124 行)
// 这是整个系统配置的顶层入口，聚合所有子模块配置

// OpenAcosmiMeta 配置元数据
type OpenAcosmiMeta struct {
	LastTouchedVersion string `json:"lastTouchedVersion,omitempty"`
	LastTouchedAt      string `json:"lastTouchedAt,omitempty"`
}

// OpenAcosmiShellEnvConfig Shell 环境变量导入配置
type OpenAcosmiShellEnvConfig struct {
	Enabled   *bool `json:"enabled,omitempty"`
	TimeoutMs *int  `json:"timeoutMs,omitempty"`
}

// OpenAcosmiEnvConfig 环境变量配置
type OpenAcosmiEnvConfig struct {
	ShellEnv *OpenAcosmiShellEnvConfig `json:"shellEnv,omitempty"`
	Vars     map[string]string         `json:"vars,omitempty"`
}

// OpenAcosmiWizardConfig 向导配置
type OpenAcosmiWizardConfig struct {
	LastRunAt      string `json:"lastRunAt,omitempty"`
	LastRunVersion string `json:"lastRunVersion,omitempty"`
	LastRunCommit  string `json:"lastRunCommit,omitempty"`
	LastRunCommand string `json:"lastRunCommand,omitempty"`
	LastRunMode    string `json:"lastRunMode,omitempty"` // "local"|"remote"
}

// OpenAcosmiUpdateConfig 更新配置
type OpenAcosmiUpdateConfig struct {
	Channel      string `json:"channel,omitempty"` // "stable"|"beta"|"dev"
	CheckOnStart *bool  `json:"checkOnStart,omitempty"`
}

// OpenAcosmiUIAssistantConfig UI 助手显示配置
type OpenAcosmiUIAssistantConfig struct {
	Name   string `json:"name,omitempty"`
	Avatar string `json:"avatar,omitempty"`
}

// OpenAcosmiUIConfig UI 配置
type OpenAcosmiUIConfig struct {
	SeamColor string                       `json:"seamColor,omitempty"`
	Assistant *OpenAcosmiUIAssistantConfig `json:"assistant,omitempty"`
}

// OpenAcosmiConfig 系统总配置 — 顶层入口
type OpenAcosmiConfig struct {
	Meta               *OpenAcosmiMeta           `json:"meta,omitempty" label:"Metadata"`
	Auth               *AuthConfig               `json:"auth,omitempty" label:"Authentication"`
	Env                *OpenAcosmiEnvConfig      `json:"env,omitempty" label:"Environment"`
	Wizard             *OpenAcosmiWizardConfig   `json:"wizard,omitempty" label:"Setup Wizard"`
	Diagnostics        *DiagnosticsConfig        `json:"diagnostics,omitempty" label:"Diagnostics"`
	Logging            *LoggingConfig            `json:"logging,omitempty" label:"Logging"`
	Update             *OpenAcosmiUpdateConfig   `json:"update,omitempty" label:"Update Settings"`
	Browser            *BrowserConfig            `json:"browser,omitempty" label:"Browser"`
	UI                 *OpenAcosmiUIConfig       `json:"ui,omitempty" label:"UI Appearance"`
	Skills             *SkillsConfig             `json:"skills,omitempty" label:"Skills"`
	Plugins            *PluginsConfig            `json:"plugins,omitempty" label:"Plugins"`
	Models             *ModelsConfig             `json:"models,omitempty" label:"Model Providers"`
	NodeHost           *NodeHostConfig           `json:"nodeHost,omitempty" label:"Node Host"`
	Agents             *AgentsConfig             `json:"agents,omitempty" label:"Agents"`
	Tools              *ToolsConfig              `json:"tools,omitempty" label:"Tools"`
	Markdown           *MarkdownConfig           `json:"markdown,omitempty" label:"Markdown"`
	Bindings           []AgentBinding            `json:"bindings,omitempty" label:"Bindings"`
	Broadcast          *BroadcastConfig          `json:"broadcast,omitempty" label:"Broadcast"`
	Audio              *AudioConfig              `json:"audio,omitempty" label:"Audio / TTS"`
	Messages           *MessagesConfig           `json:"messages,omitempty" label:"Messages"`
	Commands           *CommandsConfig           `json:"commands,omitempty" label:"Commands"`
	Approvals          *ApprovalsConfig          `json:"approvals,omitempty" label:"Execution Approvals"`
	Session            *SessionConfig            `json:"session,omitempty" label:"Session"`
	Web                *WebConfig                `json:"web,omitempty" label:"Web Server"`
	Channels           *ChannelsConfig           `json:"channels,omitempty" label:"Channels"`
	Cron               *CronConfig               `json:"cron,omitempty" label:"Cron Jobs"`
	Hooks              *HooksConfig              `json:"hooks,omitempty" label:"Hooks"`
	Discovery          *DiscoveryConfig          `json:"discovery,omitempty" label:"Discovery"`
	CanvasHost         *CanvasHostConfig         `json:"canvasHost,omitempty" label:"Canvas Host"`
	Talk               *TalkConfig               `json:"talk,omitempty" label:"Talk / Voice"`
	Gateway            *GatewayConfig            `json:"gateway,omitempty" label:"Gateway"`
	Memory             *MemoryConfig             `json:"memory,omitempty" label:"Memory"`
	STT                *STTConfig                `json:"stt,omitempty" label:"Speech to Text"`
	DocConv            *DocConvConfig            `json:"docConv,omitempty" label:"Document Conversion"`
	ImageUnderstanding *ImageUnderstandingConfig `json:"imageUnderstanding,omitempty" label:"Image Understanding"`
	SubAgents          *SubAgentConfig           `json:"subAgents,omitempty" label:"Sub-Agents"`
}

// SubAgentConfig 子智能体配置。
type SubAgentConfig struct {
	ScreenObserver *ScreenObserverSettings `json:"screenObserver,omitempty"`
	OpenCoder      *OpenCoderSettings      `json:"openCoder,omitempty"`
	MediaAgent     *MediaAgentSettings     `json:"mediaAgent,omitempty"`
}

// MediaAgentSettings 媒体子智能体配置。
type MediaAgentSettings struct {
	AutoSpawnEnabled    bool     `json:"autoSpawnEnabled,omitempty"`    // 自动 spawn 开关（默认 false）
	MaxAutoSpawnsPerDay int      `json:"maxAutoSpawnsPerDay,omitempty"` // 每日最大自动 spawn 次数（默认 5）
	Provider            string   `json:"provider,omitempty"`            // LLM provider: "deepseek"/"anthropic" 等
	Model               string   `json:"model,omitempty"`               // LLM model: "deepseek-chat" 等
	APIKey              string   `json:"apiKey,omitempty"`              // 独立 API key（自动脱敏）
	BaseURL             string   `json:"baseUrl,omitempty"`             // OpenAI 兼容端点
	EnabledSources      []string `json:"enabledSources,omitempty"`      // 启用的热点源（空=全部启用）
	EnablePublish       *bool    `json:"enablePublish,omitempty"`       // 是否启用发布工具（nil=默认true）
	EnableInteract      *bool    `json:"enableInteract,omitempty"`      // 是否启用互动工具（nil=默认false）
	WizardCompleted     bool     `json:"wizardCompleted,omitempty"`     // 向导是否已完成
}

// OpenCoderSettings open-coder 编程子智能体独立配置。
type OpenCoderSettings struct {
	Provider string `json:"provider,omitempty"` // "anthropic"/"deepseek"/"openai" 等
	APIKey   string `json:"apiKey,omitempty"`   // 独立 API key（自动脱敏 — 匹配 redact sensitiveKeyPatterns）
	Model    string `json:"model,omitempty"`    // "deepseek-chat"/"claude-sonnet-4" 等
	BaseURL  string `json:"baseUrl,omitempty"`  // OpenAI 兼容端点（vLLM/ollama 等）
}

// ScreenObserverSettings 视觉观测器配置。
type ScreenObserverSettings struct {
	BinaryPath      string  `json:"binaryPath,omitempty"`      // argus-sensory 二进制绝对路径（ARGUS-002: 显式配置，优先于自动发现）
	Enabled         *bool   `json:"enabled,omitempty"`         // 是否启用
	IntervalMs      int     `json:"intervalMs,omitempty"`      // 截图间隔 ms（默认 1000）
	ChangeThreshold float32 `json:"changeThreshold,omitempty"` // 变化阈值（默认 0.02）
	BufferSize      int     `json:"bufferSize,omitempty"`      // ring buffer 帧数（默认 500）
	VLAModel        string  `json:"vlaModel,omitempty"`        // "showui-2b"/"opencua-7b"/"anthropic"/"none"
	VLAEndpoint     string  `json:"vlaEndpoint,omitempty"`     // VLA 服务端点
	ApprovalMode    string  `json:"approvalMode,omitempty"`    // "none"/"medium_and_above"/"all"
	// Phase 5: 灵瞳子智能体 LLM 独立配置（与 OpenCoderSettings 对称）
	Provider string `json:"provider,omitempty"` // LLM provider: "anthropic"/"openai" 等
	APIKey   string `json:"apiKey,omitempty"`   // 独立 API key（自动脱敏）
	Model    string `json:"model,omitempty"`    // LLM model: "claude-sonnet-4" 等
	BaseURL  string `json:"baseUrl,omitempty"`  // OpenAI 兼容端点
}

// ConfigValidationIssue 配置验证问题
type ConfigValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// LegacyConfigIssue 旧版配置问题
type LegacyConfigIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ConfigFileSnapshot 配置文件快照
type ConfigFileSnapshot struct {
	Path         string                  `json:"path"`
	Exists       bool                    `json:"exists"`
	Raw          *string                 `json:"raw"`
	Parsed       interface{}             `json:"parsed"`
	Valid        bool                    `json:"valid"`
	Config       OpenAcosmiConfig        `json:"config"`
	Hash         string                  `json:"hash,omitempty"`
	Issues       []ConfigValidationIssue `json:"issues"`
	Warnings     []ConfigValidationIssue `json:"warnings"`
	LegacyIssues []LegacyConfigIssue     `json:"legacyIssues"`
}
