// tree_migrate.go builds the initial CapabilityTree from four source-of-truth
// locations (P0-3 through P0-14):
//
//  1. Static Registry   (registry.go)         — registered tools and subagent entries
//  2. Runtime Definitions (tool_email.go)      — send_email (not in Registry)
//  3. Skills Frontmatter  (SKILL.md files)     — skill binding data
//  4. Bridge Contracts    (Argus/MCP bridges)  — dynamic tool groups
//
// Plus data extraction from 5 additional sources:
//   - intent_router.go   → Routing.MinTier + ExcludeFrom (P0-8)
//   - prompt_sections.go → Prompt.Summary + SortOrder    (P0-9)
//   - tool_policy.go     → Policy.PolicyGroups/Profiles  (P0-10)
//   - display.go         → Display fields                (P0-11)
//   - permission_escalation.go → EscalationHints         (P0-12)
//
// Design doc: docs/codex/2026-03-09-能力树与自治能力管理系统架构设计-v2.md §5
package capabilities

import "fmt"

// GenerateTreeFromRegistry builds the complete capability tree from all known sources.
// This is the Phase 0 migration entry point.
func GenerateTreeFromRegistry() *CapabilityTree {
	tree := NewCapabilityTree()

	// Step 1: Create group nodes
	for _, g := range groupDefs() {
		if err := tree.AddNode(g); err != nil {
			panic(fmt.Sprintf("capability tree: failed to add group %q: %v", g.ID, err))
		}
	}

	// Step 2: Create static tool/subagent nodes from Registry + runtime extras
	for _, n := range staticToolNodes() {
		if err := tree.AddNode(n); err != nil {
			panic(fmt.Sprintf("capability tree: failed to add tool %q: %v", n.ID, err))
		}
	}

	// Step 3: Create dynamic group nodes (argus_, remote_mcp_, local_mcp_)
	for _, n := range dynamicGroupNodes() {
		if err := tree.AddNode(n); err != nil {
			panic(fmt.Sprintf("capability tree: failed to add dynamic group %q: %v", n.ID, err))
		}
	}

	return tree
}

// ---------------------------------------------------------------------------
// Group definitions
// ---------------------------------------------------------------------------

func groupDefs() []*CapabilityNode {
	return []*CapabilityNode{
		{
			ID: "runtime", Name: "runtime", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Core runtime execution tools"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:runtime"}},
		},
		{
			ID: "fs", Name: "fs", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "File system operations"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:fs"}},
		},
		{
			ID: "web", Name: "web", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Web search and fetch tools"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:web"}},
		},
		{
			ID: "ui", Name: "ui", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Browser and canvas UI tools"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:ui"}},
		},
		{
			ID: "memory", Name: "memory", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "UHMS memory search and retrieval"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:memory"}},
		},
		{
			ID: "system", Name: "system", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "System management tools"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:system"}},
		},
		{
			ID: "messaging", Name: "messaging", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Channel messaging tools"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:messaging"}},
		},
		{
			ID: "sessions", Name: "sessions", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Session and sub-agent management"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:sessions"}},
		},
		{
			ID: "ai", Name: "ai", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "AI model tools"},
			Policy: &NodePolicy{PolicyGroups: []string{"group:ai"}},
		},
		{
			ID: "media", Name: "media", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Media sending and email tools"},
		},
		{
			ID: "skills", Name: "skills", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Skill index search and lookup"},
		},
		{
			ID: "subagents", Name: "subagents", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Sub-agent delegation entries"},
		},
		{
			ID: "internal", Name: "internal", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Internal system tools"},
		},
		{
			ID: "dynamic", Name: "dynamic", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Dynamic tool groups discovered at runtime"},
		},
		// P4-9: meta group for self-inspection tools
		{
			ID: "meta", Name: "meta", Kind: NodeKindGroup,
			Prompt: &NodePrompt{GroupIntro: "Self-inspection and governance meta-tools"},
		},
	}
}

// ---------------------------------------------------------------------------
// Static tool/subagent nodes — Registry (P0-3) + send_email (P0-4)
// with routing (P0-8), prompt (P0-9), policy (P0-10), display (P0-11),
// escalation hints (P0-12), skills (P0-13), wizard (P0-14)
// ---------------------------------------------------------------------------

func staticToolNodes() []*CapabilityNode {
	nodes := []*CapabilityNode{
		// ── runtime/ ──
		{
			ID: "runtime/bash", Name: "bash", Kind: NodeKindTool, Parent: "runtime",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Execute bash commands in the workspace", SortOrder: 1},
			Routing: &NodeRouting{
				MinTier: "task_light",
				// P3-6: delete keywords → IntentPriority 30 classifies as task_delete
				IntentKeywords: IntentKeywords{
					ZH: []string{"删除", "删掉", "删了", "删", "移除", "清理", "清除", "清掉"},
					EN: []string{"remove", "delete", "rm "},
				},
				IntentPriority: 30,
			},
			Perms: &NodePermissions{
				MinSecurityLevel: "sandboxed", FileAccess: "none",
				ApprovalType: "exec_escalation", ScopeCheck: "workspace",
				EscalationHints: &EscalationHints{
					DefaultRequestedLevel: "sandboxed", DefaultTTLMinutes: 30,
					NeedsRunSession: true,
				},
			},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "🛠️", Title: "Bash", Verb: "Execute", DetailKeys: "command"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:runtime"}, Profiles: []string{"coding", "full"}, WizardGroup: "runtime"},
		},

		// ── fs/ ──
		{
			ID: "fs/read_file", Name: "read_file", Kind: NodeKindTool, Parent: "fs",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Read file contents", SortOrder: 2},
			Routing: &NodeRouting{MinTier: "task_light"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "global_read", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "📖", Title: "Read File", Verb: "Read", DetailKeys: "path"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:fs"}, Profiles: []string{"coding", "full"}, WizardGroup: "fs"},
		},
		{
			ID: "fs/write_file", Name: "write_file", Kind: NodeKindTool, Parent: "fs",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Create or overwrite files", SortOrder: 3},
			Routing: &NodeRouting{
				MinTier:     "task_write",
				ExcludeFrom: []string{"task_delete"},
				// P3-6: write/create keywords → IntentPriority 10 classifies as task_write
				IntentKeywords: IntentKeywords{
					ZH: []string{"写", "编写", "创建", "修改", "改一", "改个", "修", "添加", "新增"},
					EN: []string{"write", "create", "add", "modify", "fix"},
				},
				IntentPriority: 10,
			},
			Perms: &NodePermissions{
				MinSecurityLevel: "sandboxed", FileAccess: "scoped_write",
				ApprovalType: "plan_confirm", ScopeCheck: "workspace",
			},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "✍️", Title: "Write File", Verb: "Write", DetailKeys: "path"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:fs"}, Profiles: []string{"coding", "full"}, WizardGroup: "fs"},
		},
		{
			ID: "fs/list_dir", Name: "list_dir", Kind: NodeKindTool, Parent: "fs",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "List directory contents", SortOrder: 4},
			Routing: &NodeRouting{MinTier: "task_light"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "global_read", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "📂", Title: "List Dir", Verb: "List", DetailKeys: "path"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:fs"}, Profiles: []string{"coding", "full"}, WizardGroup: "fs"},
		},
		{
			ID: "fs/apply_patch", Name: "apply_patch", Kind: NodeKindTool, Parent: "fs",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "tools.exec.applyPatch.enabled"},
			Prompt:  &NodePrompt{Summary: "Apply multi-file patches with structured patch format", SortOrder: 5},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms: &NodePermissions{
				MinSecurityLevel: "sandboxed", FileAccess: "scoped_write",
				ApprovalType: "plan_confirm", ScopeCheck: "workspace",
			},
			Skills: &NodeSkillBinding{Bindable: true},
			Policy: &NodePolicy{PolicyGroups: []string{"group:fs"}, Profiles: []string{"coding", "full"}, WizardGroup: "fs"},
		},

		// ── web/ ──
		{
			ID: "web/web_search", Name: "web_search", Kind: NodeKindTool, Parent: "web",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "WebSearchProvider != nil"},
			Prompt:  &NodePrompt{Summary: "Search the web for real-time information", SortOrder: 6},
			Routing: &NodeRouting{MinTier: "task_light", ExcludeFrom: []string{"task_delete"}},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "🔎", Title: "Web Search", Verb: "Search", DetailKeys: "query,count"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:web"}, Profiles: []string{"full"}, WizardGroup: "web"},
		},
		{
			ID: "web/web_fetch", Name: "web_fetch", Kind: NodeKindTool, Parent: "web",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Fetch and extract readable content from a URL", SortOrder: 7},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "📄", Title: "Web Fetch", Verb: "Fetch", DetailKeys: "url,extractMode,maxChars"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:web"}, Profiles: []string{"full"}, WizardGroup: "web"},
		},

		// ── ui/ ──
		{
			ID: "ui/browser", Name: "browser", Kind: NodeKindTool, Parent: "ui",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "BrowserController != nil"},
			Prompt:  &NodePrompt{Summary: "Control web browser via CDP (navigate, click, type, screenshot, ARIA refs)", SortOrder: 8},
			Routing: &NodeRouting{
				MinTier:     "task_light",
				ExcludeFrom: []string{"task_delete"},
				// P3-6: browser keywords → IntentPriority 20 classifies as task_multimodal
				IntentKeywords: IntentKeywords{
					ZH: []string{"浏览器自动化", "浏览器打开", "网页", "打开网站", "打开官网", "打开页面", "访问网站", "访问官网", "页面点击", "填写表单"},
					EN: []string{"browser automation", "browser open", "open website", "open page", "visit site", "fill form", "click button"},
				},
				IntentPriority: 20,
			},
			Perms:   &NodePermissions{MinSecurityLevel: "sandboxed", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "🌐", Title: "Browser", Verb: "Browse"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:ui"}, Profiles: []string{"full"}, WizardGroup: "web"},
		},
		{
			ID: "ui/canvas", Name: "canvas", Kind: NodeKindTool, Parent: "ui",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Display, evaluate and snapshot Canvas artifacts", SortOrder: 9},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:ui"}, Profiles: []string{"full"}, WizardGroup: "ui"},
		},

		// ── memory/ ──
		{
			ID: "memory/memory_search", Name: "memory_search", Kind: NodeKindTool, Parent: "memory",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "UHMSBridge != nil"},
			Prompt:  &NodePrompt{Summary: "Search UHMS memory by keyword", SortOrder: 26},
			Routing: &NodeRouting{MinTier: "question", ExcludeFrom: []string{"task_delete"}},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "🧠", Title: "Memory Search", Verb: "Search", DetailKeys: "query"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:memory"}, Profiles: []string{"coding", "full"}, WizardGroup: "memory"},
		},
		{
			ID: "memory/memory_get", Name: "memory_get", Kind: NodeKindTool, Parent: "memory",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "UHMSBridge != nil"},
			Prompt:  &NodePrompt{Summary: "Get specific memory entry by ID", SortOrder: 27},
			Routing: &NodeRouting{MinTier: "question", ExcludeFrom: []string{"task_delete"}},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "📓", Title: "Memory Get", Verb: "Get", DetailKeys: "path,from,lines"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:memory"}, Profiles: []string{"coding", "full"}, WizardGroup: "memory"},
		},

		// ── system/ ──
		{
			ID: "system/nodes", Name: "nodes", Kind: NodeKindTool, Parent: "system",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "List/describe/notify/control paired node devices", SortOrder: 10},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:system"}, Profiles: []string{"full"}, WizardGroup: "system"},
		},
		{
			ID: "system/cron", Name: "cron", Kind: NodeKindTool, Parent: "system",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Manage scheduled tasks and wake events", SortOrder: 11},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "sandboxed", FileAccess: "none", ApprovalType: "plan_confirm", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:system"}, Profiles: []string{"full"}, WizardGroup: "system"},
		},
		{
			ID: "system/gateway", Name: "gateway", Kind: NodeKindTool, Parent: "system",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Inspect schema, patch/apply config, restart, or run updates", SortOrder: 12},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms: &NodePermissions{
				MinSecurityLevel: "full", FileAccess: "none",
				ApprovalType: "exec_escalation", ScopeCheck: "none",
				EscalationHints: &EscalationHints{
					DefaultRequestedLevel: "full", DefaultTTLMinutes: 0,
					NeedsRunSession: true,
				},
			},
			Skills: &NodeSkillBinding{Bindable: true},
			Policy: &NodePolicy{PolicyGroups: []string{"group:system"}, Profiles: []string{"full"}, WizardGroup: "system"},
		},

		// ── messaging/ ──
		{
			ID: "messaging/message", Name: "message", Kind: NodeKindTool, Parent: "messaging",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Send messages and channel operations", SortOrder: 13},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:messaging"}, Profiles: []string{"messaging", "full"}, WizardGroup: "messaging"},
		},

		// ── sessions/ ──
		{
			ID: "sessions/agents_list", Name: "agents_list", Kind: NodeKindTool, Parent: "sessions",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "List available agent IDs for sessions_spawn", SortOrder: 17},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:sessions"}, Profiles: []string{"coding", "full"}, WizardGroup: "sessions"},
		},
		{
			ID: "sessions/sessions_list", Name: "sessions_list", Kind: NodeKindTool, Parent: "sessions",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "List other sessions with filters and pagination", SortOrder: 18},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:sessions"}, Profiles: []string{"coding", "messaging", "full"}, WizardGroup: "sessions"},
		},
		{
			ID: "sessions/sessions_history", Name: "sessions_history", Kind: NodeKindTool, Parent: "sessions",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Fetch history for another session or sub-agent", SortOrder: 19},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:sessions"}, Profiles: []string{"coding", "messaging", "full"}, WizardGroup: "sessions"},
		},
		{
			ID: "sessions/sessions_send", Name: "sessions_send", Kind: NodeKindTool, Parent: "sessions",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Send a message to another session or sub-agent", SortOrder: 20},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:sessions"}, Profiles: []string{"coding", "messaging", "full"}, WizardGroup: "sessions"},
		},
		{
			ID: "sessions/sessions_spawn", Name: "sessions_spawn", Kind: NodeKindTool, Parent: "sessions",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Spawn a sub-agent session", SortOrder: 21},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "sandboxed", FileAccess: "none", ApprovalType: "plan_confirm", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:sessions"}, Profiles: []string{"coding", "full"}, WizardGroup: "sessions"},
		},
		{
			ID: "sessions/session_status", Name: "session_status", Kind: NodeKindTool, Parent: "sessions",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Show session status card (usage, time, mode)", SortOrder: 22},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:sessions"}, Profiles: []string{"minimal", "coding", "messaging", "full"}, WizardGroup: "sessions"},
		},

		// ── ai/ ──
		{
			ID: "ai/image", Name: "image", Kind: NodeKindTool, Parent: "ai",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Analyze images using configured image model", SortOrder: 23},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: true},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:ai"}, Profiles: []string{"coding", "full"}, WizardGroup: "ai"},
		},

		// ── media/ (P0-4: includes send_email which is NOT in Registry) ──
		{
			ID: "media/send_media", Name: "send_media", Kind: NodeKindTool, Parent: "media",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "MediaSender != nil"},
			Prompt: &NodePrompt{
				Summary:    "Send file/media to channel (feishu/discord/telegram/whatsapp)",
				SortOrder:  16,
				UsageGuide: "Use when user asks to send/share/forward a file or media to a channel",
			},
			Routing: &NodeRouting{
				// P3-4: 降级为 task_light — "发给我文件" 是轻量读+发送操作，不需要 task_write 级别
				MinTier:     "task_light",
				ExcludeFrom: []string{"task_delete"},
				IntentKeywords: IntentKeywords{
					ZH: []string{"发送", "发给我", "传给我", "分享", "转发", "文件", "发给", "发消息", "通知"},
					EN: []string{"send", "share", "forward", "file", "media"},
				},
				// IntentPriority=0: 不参与 tier 分类（send_media 在 task_light 层即可用，无需关键词路由）
			},
			Perms: &NodePermissions{
				MinSecurityLevel: "allowlist", FileAccess: "scoped_read",
				ApprovalType: "data_export", ScopeCheck: "mount_required",
				EscalationHints: &EscalationHints{
					DefaultRequestedLevel: "allowlist",
					DefaultTTLMinutes:     30,
					DefaultMountMode:      "ro",
					NeedsOriginator:       true,
				},
			},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "📎", Title: "Send Media", Verb: "Send", DetailKeys: "path,channel"},
			Policy:  &NodePolicy{Profiles: []string{"messaging", "full"}},
		},
		// P0-4: send_email — NOT in Registry, only in runtime buildToolDefinitions
		{
			ID: "media/send_email", Name: "send_email", Kind: NodeKindTool, Parent: "media",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "EmailSender != nil"},
			Prompt: &NodePrompt{
				Summary:    "Send an email message with new/reply support",
				SortOrder:  16,
				UsageGuide: "Use when user asks to send email, compose email, or reply to email thread",
			},
			Routing: &NodeRouting{
				MinTier:     "task_write",
				ExcludeFrom: []string{"task_delete"},
				IntentKeywords: IntentKeywords{
					ZH: []string{"邮件", "发邮件", "写邮件", "回复邮件"},
					EN: []string{"email", "send email", "compose email", "reply email"},
				},
				// F-01 修复: IntentPriority=10 确保 "发送邮件" 路由到 task_write（"邮件" 命中）
				IntentPriority: 10,
			},
			Perms: &NodePermissions{
				MinSecurityLevel: "allowlist", FileAccess: "none",
				ApprovalType: "plan_confirm", ScopeCheck: "none",
			},
			Skills:  &NodeSkillBinding{Bindable: true},
			Display: &NodeDisplay{Icon: "📧", Title: "Send Email", Verb: "Send", DetailKeys: "to,subject"},
		},

		// ── skills/ ──
		{
			ID: "skills/search_skills", Name: "search_skills", Kind: NodeKindTool, Parent: "skills",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "UHMSBridge != nil && IsSkillsIndexed"},
			Prompt:  &NodePrompt{Summary: "Search skills index by keyword", SortOrder: 24},
			Routing: &NodeRouting{MinTier: "question"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
		},
		{
			ID: "skills/lookup_skill", Name: "lookup_skill", Kind: NodeKindTool, Parent: "skills",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "skills available"},
			Prompt:  &NodePrompt{Summary: "Look up full content of a skill by name", SortOrder: 25},
			Routing: &NodeRouting{MinTier: "question"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
		},

		// ── subagents/ ──
		{
			ID: "subagents/spawn_coder_agent", Name: "spawn_coder_agent", Kind: NodeKindSubagent, Parent: "subagents",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt: &NodePrompt{
				Summary:    "Delegate coding tasks to Open Coder sub-agent (delegation contract)",
				SortOrder:  14,
				Delegation: "Delegate file-heavy coding tasks; coder has sandboxed fs access",
			},
			Routing: &NodeRouting{
				MinTier:     "task_write",
				ExcludeFrom: []string{"task_delete"},
				// P3-6: develop/coding keywords → IntentPriority 10 classifies as task_write
				IntentKeywords: IntentKeywords{
					ZH: []string{"开发", "生成", "实现", "重构", "优化"},
					EN: []string{"code", "develop", "build", "implement", "generate", "refactor"},
				},
				IntentPriority: 10,
			},
			Perms:  &NodePermissions{MinSecurityLevel: "sandboxed", FileAccess: "scoped_write", ApprovalType: "plan_confirm", ScopeCheck: "workspace"},
			Skills: &NodeSkillBinding{Bindable: false},
		},
		{
			ID: "subagents/spawn_argus_agent", Name: "spawn_argus_agent", Kind: NodeKindSubagent, Parent: "subagents",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "ArgusBridge != nil"},
			Prompt: &NodePrompt{
				Summary:    "Delegate desktop/visual tasks to Argus sub-agent (screen + visual perception)",
				SortOrder:  15,
				Delegation: "Delegate desktop UI tasks, screen capture, visual reasoning to Argus",
			},
			// P3-5: 降级为 task_write — 允许桌面视觉任务在 task_write 层级触发
			Routing: &NodeRouting{
				MinTier: "task_write",
				// P3-6: visual/argus keywords → IntentPriority 20 classifies as task_multimodal
				IntentKeywords: IntentKeywords{
					ZH: []string{"截屏", "截图", "截个", "截一", "屏幕截", "拍照", "拍个照", "拍一张", "灵瞳"},
					EN: []string{"screenshot", "capture screen", "capture ", "argus"},
				},
				IntentPriority: 20,
			},
			Perms:  &NodePermissions{MinSecurityLevel: "full", FileAccess: "none", ApprovalType: "exec_escalation", ScopeCheck: "none"},
			Skills: &NodeSkillBinding{Bindable: false},
		},
		{
			ID: "subagents/spawn_media_agent", Name: "spawn_media_agent", Kind: NodeKindSubagent, Parent: "subagents",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "MediaSubsystem != nil"},
			Prompt: &NodePrompt{
				Summary:    "Delegate media operations to media sub-agent",
				SortOrder:  16,
				Delegation: "Delegate media processing tasks to the media subsystem agent",
			},
			Routing: &NodeRouting{MinTier: "task_write", ExcludeFrom: []string{"task_delete"}},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "plan_confirm", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
		},

		// ── internal/ ──
		{
			ID: "internal/report_progress", Name: "report_progress", Kind: NodeKindTool, Parent: "internal",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Report intermediate progress to user", SortOrder: 28},
			Routing: &NodeRouting{MinTier: "task_light"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
		},
		{
			ID: "internal/request_help", Name: "request_help", Kind: NodeKindTool, Parent: "internal",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "AgentChannel != nil"},
			Prompt:  &NodePrompt{Summary: "Request help from parent agent (sub-agent only)", SortOrder: 29},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
		},

		// ── meta/ (P4-9) ──
		{
			ID: "meta/capability_manage", Name: "capability_manage", Kind: NodeKindTool, Parent: "meta",
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Inspect, diagnose, and manage the capability tree", SortOrder: 30},
			Routing: &NodeRouting{MinTier: "task_light"}, // S2: 从 question 提升到 task_light，避免误分流时暴露元工具
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
			Display: &NodeDisplay{Icon: "🌳", Title: "Capability Manage", Verb: "Inspect"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:system"}, Profiles: []string{"full"}},
		},
	}
	nodes = append(nodes, specializedConfigToolNodes()...)
	return nodes
}

// ---------------------------------------------------------------------------
// Dynamic group nodes — P0-5, P0-6, P0-7
// ---------------------------------------------------------------------------

func dynamicGroupNodes() []*CapabilityNode {
	return []*CapabilityNode{
		// P0-5: Argus dynamic tools (argus_click, argus_describe_scene, etc.)
		{
			ID: "dynamic/argus", Name: "argus", Kind: NodeKindGroup, Parent: "dynamic",
			Runtime: &NodeRuntime{
				Owner: "argus_bridge", EnabledWhen: "ArgusBridge != nil",
				Dynamic:         true,
				NamePrefix:      "argus_",
				DiscoverySource: "ArgusBridge.AgentTools()",
				ProviderID:      "argus",
				ListMethod:      "AgentTools",
			},
			Prompt:  &NodePrompt{GroupIntro: "Argus desktop automation tools (screen capture, click, type, etc.)"},
			Routing: &NodeRouting{MinTier: "task_multimodal"},
			Perms: &NodePermissions{
				MinSecurityLevel: "full", FileAccess: "none",
				ApprovalType: "exec_escalation", ScopeCheck: "none",
			},
		},
		// P0-6: Remote MCP tools (remote_*)
		{
			ID: "dynamic/remote_mcp", Name: "remote_mcp", Kind: NodeKindGroup, Parent: "dynamic",
			Runtime: &NodeRuntime{
				Owner: "attempt_runner", EnabledWhen: "RemoteMCPBridge != nil",
				Dynamic:         true,
				NamePrefix:      "remote_",
				DiscoverySource: "RemoteMCPBridge.AgentRemoteTools()",
				ProviderID:      "remote_mcp",
				ListMethod:      "AgentRemoteTools",
			},
			Prompt:  &NodePrompt{GroupIntro: "Remote MCP server tools"},
			Routing: &NodeRouting{MinTier: "task_write"},
		},
		// P0-7: Local MCP tools (mcp_*)
		{
			ID: "dynamic/local_mcp", Name: "local_mcp", Kind: NodeKindGroup, Parent: "dynamic",
			Runtime: &NodeRuntime{
				Owner: "attempt_runner", EnabledWhen: "LocalMCPBridge != nil",
				Dynamic:         true,
				NamePrefix:      "mcp_",
				DiscoverySource: "LocalMCPBridge.AgentTools()",
				ProviderID:      "local_mcp",
				ListMethod:      "AgentTools",
			},
			Prompt:  &NodePrompt{GroupIntro: "Local MCP server tools"},
			Routing: &NodeRouting{MinTier: "task_light"},
		},
	}
}
