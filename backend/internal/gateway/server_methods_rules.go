package gateway

// server_methods_rules.go — security.rules.* 方法处理器
// P3: Allow/Ask/Deny 命令规则引擎 CRUD API
//
// 方法:
//   - security.rules.list   — 列出所有规则（预设 + 用户自定义）
//   - security.rules.add    — 添加用户自定义规则
//   - security.rules.remove — 删除用户自定义规则（预设不可删除）
//   - security.rules.test   — 测试命令匹配规则

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// RulesHandlers 返回 security.rules.* 方法处理器映射。
func RulesHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"security.rules.list":   handleRulesList,
		"security.rules.add":    handleRulesAdd,
		"security.rules.remove": handleRulesRemove,
		"security.rules.test":   handleRulesTest,
	}
}

// ---------- security.rules.list ----------
// 列出所有规则（预设 + 用户自定义）。

func handleRulesList(ctx *MethodHandlerContext) {
	snapshot := infra.ReadExecApprovalsSnapshot()
	var userRules []infra.CommandRule
	if snapshot.File != nil && snapshot.File.Defaults != nil {
		userRules = snapshot.File.Defaults.Rules
	}
	allRules := runner.MergeRulesWithPresets(userRules)

	ctx.Respond(true, map[string]interface{}{
		"rules":       allRules,
		"total":       len(allRules),
		"presetCount": len(runner.PresetCommandRules),
		"userCount":   len(userRules),
	}, nil)
}

// ---------- security.rules.add ----------
// 添加用户自定义规则。

func handleRulesAdd(ctx *MethodHandlerContext) {
	pattern, _ := ctx.Params["pattern"].(string)
	if pattern == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "pattern is required"))
		return
	}

	actionStr, _ := ctx.Params["action"].(string)
	action := infra.CommandRuleAction(actionStr)
	if action != infra.RuleActionAllow && action != infra.RuleActionAsk && action != infra.RuleActionDeny {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "action must be 'allow', 'ask', or 'deny'"))
		return
	}

	description, _ := ctx.Params["description"].(string)

	priority := 50 // 用户自定义规则默认优先级
	if priorityRaw, ok := ctx.Params["priority"].(float64); ok && priorityRaw >= 0 {
		priority = int(priorityRaw)
	}

	// 生成规则 ID
	id := generateRuleID()
	now := time.Now().UnixMilli()

	newRule := infra.CommandRule{
		ID:          id,
		Pattern:     pattern,
		Action:      action,
		Description: description,
		IsPreset:    false,
		Priority:    priority,
		CreatedAt:   &now,
	}

	// 读取 → 追加 → 保存（受锁保护）
	unlock := infra.AcquireExecApprovalsLock()
	snapshot := infra.ReadExecApprovalsSnapshot()
	file := snapshot.File
	if file == nil {
		file = &infra.ExecApprovalsFile{Version: 1, Agents: make(map[string]*infra.ExecApprovalsAgent)}
	}
	infra.NormalizeExecApprovals(file)
	file.Defaults.Rules = append(file.Defaults.Rules, newRule)

	saveErr := infra.SaveExecApprovals(file)
	unlock()
	if saveErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save rule: "+saveErr.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"id":      id,
		"rule":    newRule,
		"message": "Rule added successfully",
	}, nil)
}

// ---------- security.rules.remove ----------
// 删除用户自定义规则（预设规则不可删除）。

func handleRulesRemove(ctx *MethodHandlerContext) {
	ruleID, _ := ctx.Params["id"].(string)
	if ruleID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "id is required"))
		return
	}

	// 检查是否为预设规则
	for _, preset := range runner.PresetCommandRules {
		if preset.ID == ruleID {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "cannot delete preset rule: "+ruleID))
			return
		}
	}

	// 读取 → 删除 → 保存（受锁保护）
	unlock := infra.AcquireExecApprovalsLock()
	snapshot := infra.ReadExecApprovalsSnapshot()
	file := snapshot.File
	if file == nil || file.Defaults == nil {
		unlock()
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "rule not found: "+ruleID))
		return
	}

	found := false
	remaining := make([]infra.CommandRule, 0, len(file.Defaults.Rules))
	for _, rule := range file.Defaults.Rules {
		if rule.ID == ruleID {
			found = true
			continue
		}
		remaining = append(remaining, rule)
	}

	if !found {
		unlock()
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "rule not found: "+ruleID))
		return
	}

	file.Defaults.Rules = remaining

	saveErr := infra.SaveExecApprovals(file)
	unlock()
	if saveErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save rules: "+saveErr.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"id":      ruleID,
		"message": "Rule removed successfully",
	}, nil)
}

// ---------- security.rules.test ----------
// 测试命令是否匹配规则。

func handleRulesTest(ctx *MethodHandlerContext) {
	command, _ := ctx.Params["command"].(string)
	if command == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "command is required"))
		return
	}

	// 合并所有规则
	snapshot := infra.ReadExecApprovalsSnapshot()
	var userRules []infra.CommandRule
	if snapshot.File != nil && snapshot.File.Defaults != nil {
		userRules = snapshot.File.Defaults.Rules
	}
	allRules := runner.MergeRulesWithPresets(userRules)

	result := runner.EvaluateCommand(command, allRules)

	resp := map[string]interface{}{
		"command": command,
		"matched": result.Matched,
	}
	if result.Matched {
		resp["action"] = string(result.Action)
		resp["reason"] = result.Reason
		if result.Rule != nil {
			resp["matchedRule"] = map[string]interface{}{
				"id":       result.Rule.ID,
				"pattern":  result.Rule.Pattern,
				"isPreset": result.Rule.IsPreset,
			}
		}
	}

	ctx.Respond(true, resp, nil)
}

// ---------- 辅助 ----------

func generateRuleID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "rule_" + hex.EncodeToString(buf)
}
