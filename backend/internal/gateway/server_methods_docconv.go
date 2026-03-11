package gateway

// server_methods_docconv.go — 文档转换（DocConv）RPC 方法（Phase D 新增）
// 提供 docconv.config.get / docconv.config.set / docconv.test / docconv.formats 方法
// 纯新增文件，不修改任何已有方法

import (
	"context"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// DocConvHandlers 返回文档转换 RPC 方法处理器。
func DocConvHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"docconv.config.get": handleDocConvConfigGet,
		"docconv.config.set": handleDocConvConfigSet,
		"docconv.test":       handleDocConvTest,
		"docconv.formats":    handleDocConvFormats,
	}
}

// ---------- docconv.config.get ----------

// DocConvConfigGetResult docconv.config.get 响应
type DocConvConfigGetResult struct {
	Configured    bool                  `json:"configured"`
	Hash          string                `json:"hash,omitempty"`
	Provider      string                `json:"provider,omitempty"`
	MCPServerName string                `json:"mcpServerName,omitempty"`
	MCPTransport  string                `json:"mcpTransport,omitempty"`
	MCPCommand    string                `json:"mcpCommand,omitempty"`
	MCPURL        string                `json:"mcpUrl,omitempty"`
	PandocPath    string                `json:"pandocPath,omitempty"`
	Providers     []DocConvProviderInfo `json:"providers"`
	MCPPresets    []DocConvMCPPreset    `json:"mcpPresets"`
}

// DocConvProviderInfo 可选 DocConv Provider 描述
type DocConvProviderInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

// DocConvMCPPreset MCP Server 预设
type DocConvMCPPreset struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	Transport string `json:"transport"`
	Hint      string `json:"hint,omitempty"`
}

func buildDocConvConfigGetResult(cfg *types.DocConvConfig) DocConvConfigGetResult {
	result := DocConvConfigGetResult{
		Providers: []DocConvProviderInfo{
			{ID: "mcp", Label: "MCP 工具", Hint: "标准 MCP 协议，支持多种文档转换服务器"},
			{ID: "builtin", Label: "内置", Hint: "pandoc CLI + 文本直读"},
			{ID: "", Label: "禁用", Hint: "不使用文档转换"},
		},
		MCPPresets: []DocConvMCPPreset{
			{
				Name:      "mcp-pandoc",
				Label:     "mcp-pandoc",
				Command:   "npx -y mcp-pandoc",
				Transport: "stdio",
				Hint:      "基于 Pandoc，支持 MD/HTML/PDF/DOCX/LaTeX/TXT",
			},
			{
				Name:      "mcp-document-converter",
				Label:     "mcp-document-converter",
				Command:   "npx -y @xt765/mcp-document-converter",
				Transport: "stdio",
				Hint:      "25 种格式组合，语法高亮，CSS 样式",
			},
			{
				Name:      "doc-ops-mcp",
				Label:     "doc-ops-mcp (Tele-AI)",
				Command:   "npx -y doc-ops-mcp",
				Transport: "stdio",
				Hint:      "智能转换规划，OOXML 解析，水印/二维码",
			},
		},
	}

	if cfg != nil && cfg.Provider != "" {
		result.Configured = true
		result.Provider = cfg.Provider
		result.MCPServerName = cfg.MCPServerName
		result.MCPTransport = cfg.MCPTransport
		result.MCPCommand = cfg.MCPCommand
		result.MCPURL = cfg.MCPURL
		result.PandocPath = cfg.PandocPath
	}

	return result
}

func handleDocConvConfigGet(ctx *MethodHandlerContext) {
	result := buildDocConvConfigGetResult(loadDocConvConfigFromCtx(ctx))
	if loader := ctx.Context.ConfigLoader; loader != nil {
		if snapshot, err := loader.ReadConfigFileSnapshot(); err == nil && snapshot != nil {
			result.Hash = snapshot.Hash
		}
	}

	ctx.Respond(true, result, nil)
}

// ---------- docconv.config.set ----------

func handleDocConvConfigSet(ctx *MethodHandlerContext) {
	executeConfigMutation(ctx, configMutationOptions{
		Action: "docconv.config.set",
		Mutate: func(currentCfg *types.OpenAcosmiConfig) error {
			if currentCfg.DocConv == nil {
				currentCfg.DocConv = &types.DocConvConfig{}
			}
			current := currentCfg.DocConv

			if provider, ok := readTrimmedStringParam(ctx.Params, "provider"); ok {
				current.Provider = provider
			}
			if name, ok := readTrimmedStringParam(ctx.Params, "mcpServerName"); ok {
				current.MCPServerName = name
			}
			if transport, ok := readTrimmedStringParam(ctx.Params, "mcpTransport"); ok {
				current.MCPTransport = transport
			}
			if command, ok := readTrimmedStringParam(ctx.Params, "mcpCommand"); ok {
				current.MCPCommand = command
			}
			if url, ok := readTrimmedStringParam(ctx.Params, "mcpUrl"); ok {
				current.MCPURL = url
			}
			if pandocPath, ok := readTrimmedStringParam(ctx.Params, "pandocPath"); ok {
				current.PandocPath = pandocPath
			}
			if useSandbox, ok := readOptionalBoolParam(ctx.Params, "useSandbox"); ok {
				current.UseSandbox = &useSandbox
			}
			return nil
		},
		AfterWrite: func(_ *MethodHandlerContext, cfg *types.OpenAcosmiConfig) map[string]interface{} {
			return map[string]interface{}{
				"docconv": buildDocConvConfigGetResult(cfg.DocConv),
			}
		},
	})
}

// ---------- docconv.test ----------

func handleDocConvTest(ctx *MethodHandlerContext) {
	cfg := loadDocConvConfigFromCtx(ctx)
	if cfg == nil || cfg.Provider == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "DocConv not configured"))
		return
	}

	converter, err := media.NewDocConverter(cfg)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "create converter: "+err.Error()))
		return
	}

	testCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := converter.TestConnection(testCtx); err != nil {
		ctx.Respond(true, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil)
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"success":  true,
		"provider": converter.Name(),
		"formats":  converter.SupportedFormats(),
	}, nil)
}

// ---------- docconv.formats ----------

func handleDocConvFormats(ctx *MethodHandlerContext) {
	all := media.AllSupportedExtensions()
	ctx.Respond(true, map[string]interface{}{
		"formats": all,
	}, nil)
}

// ---------- helpers ----------

func loadDocConvConfigFromCtx(ctx *MethodHandlerContext) *types.DocConvConfig {
	cfgLoader := ctx.Context.ConfigLoader
	if cfgLoader == nil {
		return nil
	}
	cfg, err := cfgLoader.LoadConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.DocConv
}
