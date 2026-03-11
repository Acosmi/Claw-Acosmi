package configtools

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

type ActionKind string

const (
	ActionRead      ActionKind = "read"
	ActionWrite     ActionKind = "write"
	ActionVerify    ActionKind = "verify"
	ActionEnumerate ActionKind = "enumerate"
)

type HashScope string

const (
	HashScopeNone                 HashScope = "none"
	HashScopeMainConfig           HashScope = "main_config"
	HashScopeRemoteApprovalConfig HashScope = "remote_approval_config"
)

type SecretPolicy string

const (
	SecretPolicyOpaque            SecretPolicy = "opaque"
	SecretPolicyPreserveIfOmitted SecretPolicy = "preserve_if_omitted"
)

type DomainActionSpec struct {
	Name           string
	Method         string
	Kind           ActionKind
	Status         string
	AllowedParams  []string
	RequiredParams []string
}

type DomainToolSpec struct {
	ToolName     string
	Emoji        string
	Title        string
	Summary      string
	Description  string
	HashScope    HashScope
	SecretPolicy SecretPolicy
	Actions      []DomainActionSpec
}

var specializedToolSpecs = []DomainToolSpec{
	{
		ToolName:     "browser_config",
		Emoji:        "🌐",
		Title:        "Browser Config",
		Summary:      "Read or update browser configuration via dedicated gateway actions",
		Description:  "Manage browser configuration via dedicated gateway actions. Call action=get before action=set. action=set requires baseHash from the latest browser_config get result.",
		HashScope:    HashScopeMainConfig,
		SecretPolicy: SecretPolicyOpaque,
		Actions: []DomainActionSpec{
			{Name: "get", Method: "tools.browser.get", Kind: ActionRead},
			{Name: "set", Method: "tools.browser.set", Kind: ActionWrite, Status: "saved", AllowedParams: []string{"baseHash", "enabled", "cdpUrl", "evaluateEnabled", "headless"}, RequiredParams: []string{"baseHash"}},
		},
	},
	{
		ToolName:     "remote_approval_config",
		Emoji:        "🛡️",
		Title:        "Remote Approval Config",
		Summary:      "Read, update, and verify remote approval configuration",
		Description:  "Manage remote approval configuration via dedicated gateway actions. Call action=get before action=set. action=set requires baseHash from the latest remote_approval_config get result. Use action=test after updates when verification is needed.",
		HashScope:    HashScopeRemoteApprovalConfig,
		SecretPolicy: SecretPolicyPreserveIfOmitted,
		Actions: []DomainActionSpec{
			{Name: "get", Method: "security.remoteApproval.config.get", Kind: ActionRead},
			{Name: "set", Method: "security.remoteApproval.config.set", Kind: ActionWrite, Status: "saved", AllowedParams: []string{"baseHash", "enabled", "callbackUrl", "feishu", "dingtalk", "wecom"}, RequiredParams: []string{"baseHash"}},
			{Name: "test", Method: "security.remoteApproval.test", Kind: ActionVerify, Status: "verified", AllowedParams: []string{"provider"}, RequiredParams: []string{"provider"}},
		},
	},
	{
		ToolName:     "image_config",
		Emoji:        "🖼️",
		Title:        "Image Config",
		Summary:      "Read, update, verify, and enumerate image-understanding configuration",
		Description:  "Manage image-understanding configuration via dedicated gateway actions. Call action=get before action=set. action=set requires baseHash from the latest image_config get result. Use models or ollama_models to inspect options and test to verify connectivity.",
		HashScope:    HashScopeMainConfig,
		SecretPolicy: SecretPolicyPreserveIfOmitted,
		Actions: []DomainActionSpec{
			{Name: "get", Method: "image.config.get", Kind: ActionRead},
			{Name: "set", Method: "image.config.set", Kind: ActionWrite, Status: "saved", AllowedParams: []string{"baseHash", "provider", "apiKey", "model", "baseUrl", "prompt", "maxTokens"}, RequiredParams: []string{"baseHash"}},
			{Name: "test", Method: "image.test", Kind: ActionVerify, Status: "verified"},
			{Name: "models", Method: "image.models", Kind: ActionEnumerate, AllowedParams: []string{"provider"}, RequiredParams: []string{"provider"}},
			{Name: "ollama_models", Method: "image.ollama.models", Kind: ActionEnumerate},
		},
	},
	{
		ToolName:     "stt_config",
		Emoji:        "🎙️",
		Title:        "STT Config",
		Summary:      "Read, update, verify, and enumerate speech-to-text configuration",
		Description:  "Manage speech-to-text configuration via dedicated gateway actions. Call action=get before action=set. action=set requires baseHash from the latest stt_config get result. Use models to inspect options and test to verify connectivity.",
		HashScope:    HashScopeMainConfig,
		SecretPolicy: SecretPolicyPreserveIfOmitted,
		Actions: []DomainActionSpec{
			{Name: "get", Method: "stt.config.get", Kind: ActionRead},
			{Name: "set", Method: "stt.config.set", Kind: ActionWrite, Status: "saved", AllowedParams: []string{"baseHash", "provider", "apiKey", "model", "baseUrl", "binaryPath", "modelPath", "language"}, RequiredParams: []string{"baseHash"}},
			{Name: "test", Method: "stt.test", Kind: ActionVerify, Status: "verified"},
			{Name: "models", Method: "stt.models", Kind: ActionEnumerate, AllowedParams: []string{"provider"}, RequiredParams: []string{"provider"}},
		},
	},
	{
		ToolName:     "docconv_config",
		Emoji:        "📄",
		Title:        "DocConv Config",
		Summary:      "Read, update, verify, and enumerate document-conversion configuration",
		Description:  "Manage document-conversion configuration via dedicated gateway actions. Call action=get before action=set. action=set requires baseHash from the latest docconv_config get result. Use formats to inspect supported formats and test to verify connectivity.",
		HashScope:    HashScopeMainConfig,
		SecretPolicy: SecretPolicyOpaque,
		Actions: []DomainActionSpec{
			{Name: "get", Method: "docconv.config.get", Kind: ActionRead},
			{Name: "set", Method: "docconv.config.set", Kind: ActionWrite, Status: "saved", AllowedParams: []string{"baseHash", "provider", "mcpServerName", "mcpTransport", "mcpCommand", "mcpUrl", "pandocPath", "useSandbox"}, RequiredParams: []string{"baseHash"}},
			{Name: "test", Method: "docconv.test", Kind: ActionVerify, Status: "verified"},
			{Name: "formats", Method: "docconv.formats", Kind: ActionEnumerate},
		},
	},
	{
		ToolName:     "media_config",
		Emoji:        "🎬",
		Title:        "Media Config",
		Summary:      "Read or update media-agent configuration via dedicated gateway actions",
		Description:  "Manage media-agent configuration via dedicated gateway actions. Call action=get before action=update. action=update requires baseHash from the latest media_config get result.",
		HashScope:    HashScopeMainConfig,
		SecretPolicy: SecretPolicyPreserveIfOmitted,
		Actions: []DomainActionSpec{
			{Name: "get", Method: "media.config.get", Kind: ActionRead},
			{Name: "update", Method: "media.config.update", Kind: ActionWrite, Status: "saved", AllowedParams: []string{"baseHash", "provider", "model", "apiKey", "baseUrl", "autoSpawnEnabled", "maxAutoSpawnsPerDay", "hotKeywords", "monitorIntervalMin", "trendingThreshold", "contentCategories", "autoDraftEnabled", "trendingBocha", "trendingCustomOpenAI", "wechat", "xiaohongshu", "website"}, RequiredParams: []string{"baseHash"}},
		},
	},
}

var toolSpecByName = func() map[string]DomainToolSpec {
	index := make(map[string]DomainToolSpec, len(specializedToolSpecs))
	for _, spec := range specializedToolSpecs {
		index[spec.ToolName] = spec
	}
	return index
}()

func ToolSpecs() []DomainToolSpec {
	cloned := make([]DomainToolSpec, 0, len(specializedToolSpecs))
	for _, spec := range specializedToolSpecs {
		spec.Actions = slices.Clone(spec.Actions)
		cloned = append(cloned, spec)
	}
	return cloned
}

func ToolSpecByName(name string) (DomainToolSpec, bool) {
	spec, ok := toolSpecByName[strings.TrimSpace(name)]
	return spec, ok
}

func ToolNames() []string {
	names := make([]string, 0, len(specializedToolSpecs))
	for _, spec := range specializedToolSpecs {
		names = append(names, spec.ToolName)
	}
	return names
}

func ToolSummaries() map[string]string {
	summaries := make(map[string]string, len(specializedToolSpecs))
	for _, spec := range specializedToolSpecs {
		summaries[spec.ToolName] = spec.Summary
	}
	return summaries
}

func FindActionSpec(spec DomainToolSpec, action string) (DomainActionSpec, bool) {
	for _, candidate := range spec.Actions {
		if candidate.Name == strings.TrimSpace(action) {
			return candidate, true
		}
	}
	return DomainActionSpec{}, false
}

func ActionNames(spec DomainToolSpec) []string {
	names := make([]string, 0, len(spec.Actions))
	for _, action := range spec.Actions {
		names = append(names, action.Name)
	}
	sort.Strings(names)
	return names
}

func ToolDescription(spec DomainToolSpec) string {
	if strings.TrimSpace(spec.Description) != "" {
		return spec.Description
	}
	return fmt.Sprintf("%s. Supported actions: %s.", spec.Summary, strings.Join(ActionNames(spec), ", "))
}
