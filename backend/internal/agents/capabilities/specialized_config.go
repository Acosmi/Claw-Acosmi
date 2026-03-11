package capabilities

import (
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/configtools"
)

type specializedConfigCapabilityMeta struct {
	ID             string
	SortOrder      int
	DetailKeys     string
	IntentKeywords IntentKeywords
}

var specializedConfigMetaByTool = map[string]specializedConfigCapabilityMeta{
	"browser_config": {
		ID:         "system/browser_config",
		SortOrder:  13,
		DetailKeys: "action",
		IntentKeywords: IntentKeywords{
			ZH: []string{"浏览器配置", "浏览器 cdp", "cdp 配置", "浏览器设置"},
			EN: []string{"browser config", "browser cdp", "cdp config"},
		},
	},
	"remote_approval_config": {
		ID:         "system/remote_approval_config",
		SortOrder:  14,
		DetailKeys: "action",
		IntentKeywords: IntentKeywords{
			ZH: []string{"远程审批配置", "审批配置", "飞书审批", "钉钉审批", "企微审批"},
			EN: []string{"remote approval", "approval config", "feishu approval", "dingtalk approval", "wecom approval"},
		},
	},
	"image_config": {
		ID:         "system/image_config",
		SortOrder:  15,
		DetailKeys: "action",
		IntentKeywords: IntentKeywords{
			ZH: []string{"图片理解配置", "图像理解配置", "vision 配置", "image 配置"},
			EN: []string{"image config", "vision config", "image model config"},
		},
	},
	"stt_config": {
		ID:         "system/stt_config",
		SortOrder:  16,
		DetailKeys: "action",
		IntentKeywords: IntentKeywords{
			ZH: []string{"语音识别配置", "语音转文字配置", "stt 配置", "whisper 配置"},
			EN: []string{"stt config", "speech to text config", "whisper config"},
		},
	},
	"docconv_config": {
		ID:         "system/docconv_config",
		SortOrder:  17,
		DetailKeys: "action",
		IntentKeywords: IntentKeywords{
			ZH: []string{"文档转换配置", "docconv 配置", "pandoc 配置", "文档解析配置"},
			EN: []string{"docconv config", "document conversion config", "pandoc config"},
		},
	},
	"media_config": {
		ID:         "system/media_config",
		SortOrder:  18,
		DetailKeys: "action",
		IntentKeywords: IntentKeywords{
			ZH: []string{"媒体配置", "媒体代理配置", "热点监控配置", "自动发文配置"},
			EN: []string{"media config", "media agent config", "content automation config"},
		},
	},
}

func specializedConfigRegistrySpecs() []CapabilitySpec {
	specs := make([]CapabilitySpec, 0, len(configtools.ToolSpecs()))
	for _, tool := range configtools.ToolSpecs() {
		specs = append(specs, CapabilitySpec{
			ID:            tool.ToolName,
			Kind:          KindTool,
			ToolName:      tool.ToolName,
			RuntimeOwner:  "attempt_runner",
			EnabledWhen:   "GatewayOpts.Enabled()",
			PromptSummary: tool.Summary,
			ToolGroups:    []string{"group:system"},
			SkillBindable: true,
		})
	}
	return specs
}

func specializedConfigToolNodes() []*CapabilityNode {
	nodes := make([]*CapabilityNode, 0, len(configtools.ToolSpecs()))
	for _, tool := range configtools.ToolSpecs() {
		meta, ok := specializedConfigMetaByTool[tool.ToolName]
		if !ok {
			panic(fmt.Sprintf("missing specialized config meta for %s", tool.ToolName))
		}
		nodes = append(nodes, &CapabilityNode{
			ID:     meta.ID,
			Name:   tool.ToolName,
			Kind:   NodeKindTool,
			Parent: "system",
			Runtime: &NodeRuntime{
				Owner:       "attempt_runner",
				EnabledWhen: "GatewayOpts.Enabled()",
			},
			Prompt: &NodePrompt{
				Summary:    tool.Summary,
				SortOrder:  meta.SortOrder,
				UsageGuide: specializedConfigUsageGuide(tool),
			},
			Routing: &NodeRouting{
				MinTier:        "task_write",
				ExcludeFrom:    []string{"task_delete"},
				IntentKeywords: meta.IntentKeywords,
				IntentPriority: 10,
			},
			Perms: &NodePermissions{
				MinSecurityLevel: "full",
				FileAccess:       "none",
				ApprovalType:     "exec_escalation",
				ScopeCheck:       "none",
				EscalationHints: &EscalationHints{
					DefaultRequestedLevel: "full",
					DefaultTTLMinutes:     0,
					NeedsRunSession:       true,
				},
			},
			Skills: &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{
				Icon:       tool.Emoji,
				Title:      tool.Title,
				Label:      tool.Title,
				Verb:       "Configure",
				DetailKeys: meta.DetailKeys,
			},
			Policy: &NodePolicy{
				PolicyGroups: []string{"group:system"},
				Profiles:     []string{"full"},
				WizardGroup:  "system",
			},
		})
	}
	return nodes
}

func specializedConfigUsageGuide(tool configtools.DomainToolSpec) string {
	title := strings.TrimSpace(tool.Title)
	if title == "" {
		title = tool.ToolName
	}
	return fmt.Sprintf("Use when only %s needs to be inspected, validated, or updated without editing the whole system config document", strings.ToLower(title))
}
