package runner

// agent_channel.go — 三级指挥体系 Phase 4: 异步消息通道
//
// 子智能体执行中可以异步向主智能体发送求助消息，
// 主智能体处理后回传答复，子智能体不停止执行。
//
// 双向通道模式:
//   - toParent: 子智能体 → 主智能体（求助请求 + 状态更新）
//   - toChild:  主智能体 → 子智能体（指令 + 求助回复）
//
// 行业对标:
//   - LangGraph: interrupt() + human-in-the-loop
//   - CrewAI: agent delegation + callback
//   - Anthropic: Oversight Paradox — 异步监督优于阻塞等待

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"

	"github.com/google/uuid"
)

// ---------- 消息类型 ----------

// AgentMessageType 消息类型常量。
const (
	// MsgHelpRequest 求助请求（子→主）。
	MsgHelpRequest = "help_request"
	// MsgHelpResponse 求助回复（主→子）。
	MsgHelpResponse = "help_response"
	// MsgStatusUpdate 状态更新（子→主，信息性）。
	MsgStatusUpdate = "status_update"
	// MsgDirective 指令（主→子，注入到工具循环上下文）。
	MsgDirective = "directive"
)

// AgentMessage 双向异步消息。
type AgentMessage struct {
	// ID 消息唯一标识。
	ID string `json:"id"`
	// Type 消息类型: "help_request" | "help_response" | "status_update" | "directive"
	Type string `json:"type"`
	// Content 消息内容。
	Content string `json:"content"`
	// Context 上下文信息（可选，帮助接收方理解背景）。
	Context string `json:"context,omitempty"`
	// Options 选项列表（可选，help_request 时提供备选方案）。
	Options []string `json:"options,omitempty"`
	// ReplyTo 回复的消息 ID（help_response 回复 help_request 时设置）。
	ReplyTo string `json:"replyTo,omitempty"`
	// Urgency 紧急程度（help_request 时设置）: "low" | "medium" | "high"。
	Urgency string `json:"urgency,omitempty"`
	// Timestamp 消息时间戳（Unix 毫秒）。
	Timestamp int64 `json:"timestamp"`
}

// HelpRequestPayload 求助请求的结构化载荷（request_help 工具输入）。
type HelpRequestPayload struct {
	// Question 求助问题描述。
	Question string `json:"question"`
	// Context 上下文背景（当前在做什么，遇到了什么问题）。
	Context string `json:"context,omitempty"`
	// Options 建议的解决选项（子智能体提供，主智能体/用户选择）。
	Options []string `json:"options,omitempty"`
	// Urgency 紧急程度: "low" | "medium" | "high"
	Urgency string `json:"urgency,omitempty"`
}

// ---------- AgentChannel ----------

const (
	// channelBufferSize 通道缓冲区大小。
	// 10 条消息足以覆盖异步场景，防止发送方阻塞。
	channelBufferSize = 10
)

// AgentChannel 双向异步消息通道（主智能体 ⇄ 子智能体）。
// 非阻塞设计 — 发送方永远不会阻塞（满则丢弃并 warn）。
type AgentChannel struct {
	mu       sync.Mutex
	toParent chan AgentMessage // 子→主
	toChild  chan AgentMessage // 主→子
	closed   bool
}

// NewAgentChannel 创建双向异步通道。
func NewAgentChannel() *AgentChannel {
	return &AgentChannel{
		toParent: make(chan AgentMessage, channelBufferSize),
		toChild:  make(chan AgentMessage, channelBufferSize),
	}
}

// SendToParent 子智能体向主智能体发送消息（非阻塞）。
// 通道已满或已关闭时返回 error，不阻塞发送方。
// 注: 必须持锁贯穿 close-check + send，防止 TOCTOU 竞态（send-to-closed-channel panic）。
func (ch *AgentChannel) SendToParent(msg AgentMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.closed {
		return fmt.Errorf("agent channel closed")
	}

	select {
	case ch.toParent <- msg:
		return nil
	default:
		slog.Warn("agent channel toParent full, dropping message",
			"msgID", msg.ID,
			"type", msg.Type,
		)
		return fmt.Errorf("toParent channel full (cap=%d)", channelBufferSize)
	}
}

// SendToChild 主智能体向子智能体发送消息（非阻塞）。
// 通道已满或已关闭时返回 error。
// 注: 必须持锁贯穿 close-check + send，防止 TOCTOU 竞态（send-to-closed-channel panic）。
func (ch *AgentChannel) SendToChild(msg AgentMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.closed {
		return fmt.Errorf("agent channel closed")
	}

	select {
	case ch.toChild <- msg:
		return nil
	default:
		slog.Warn("agent channel toChild full, dropping message",
			"msgID", msg.ID,
			"type", msg.Type,
		)
		return fmt.Errorf("toChild channel full (cap=%d)", channelBufferSize)
	}
}

// ReceiveFromParent 子智能体非阻塞接收主智能体消息。
// 无消息时返回 nil（不阻塞）。通道已关闭且缓冲区空时也返回 nil。
func (ch *AgentChannel) ReceiveFromParent() *AgentMessage {
	select {
	case msg, ok := <-ch.toChild:
		if !ok {
			return nil // channel closed
		}
		return &msg
	default:
		return nil
	}
}

// DrainFromChild 主智能体一次性读取所有待处理的子→主消息。
// 返回 0~N 条消息（非阻塞）。通道已关闭时返回剩余缓冲消息后停止。
func (ch *AgentChannel) DrainFromChild() []AgentMessage {
	var msgs []AgentMessage
	for {
		select {
		case msg, ok := <-ch.toParent:
			if !ok {
				return msgs // channel closed
			}
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// ToParentChan 返回 toParent channel（供 select 监听）。
func (ch *AgentChannel) ToParentChan() <-chan AgentMessage {
	return ch.toParent
}

// Close 关闭通道，安全支持重复调用。
func (ch *AgentChannel) Close() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.closed {
		return
	}
	ch.closed = true
	close(ch.toParent)
	close(ch.toChild)
}

// IsClosed 返回通道是否已关闭。
func (ch *AgentChannel) IsClosed() bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.closed
}

// ---------- 便捷方法 ----------

// SendHelpRequest 子智能体发送求助请求的便捷方法。
func (ch *AgentChannel) SendHelpRequest(payload HelpRequestPayload) (string, error) {
	msgID := uuid.New().String()
	msg := AgentMessage{
		ID:        msgID,
		Type:      MsgHelpRequest,
		Content:   payload.Question,
		Context:   payload.Context,
		Options:   payload.Options,
		Urgency:   payload.Urgency,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := ch.SendToParent(msg); err != nil {
		return "", err
	}
	return msgID, nil
}

// SendHelpResponse 主智能体发送求助回复的便捷方法。
func (ch *AgentChannel) SendHelpResponse(replyTo, content string) error {
	return ch.SendToChild(AgentMessage{
		Type:    MsgHelpResponse,
		Content: content,
		ReplyTo: replyTo,
	})
}

// SendDirective 主智能体发送指令的便捷方法。
func (ch *AgentChannel) SendDirective(content string) error {
	return ch.SendToChild(AgentMessage{
		Type:    MsgDirective,
		Content: content,
	})
}

// ---------- request_help 工具定义 + 执行器 ----------

// RequestHelpToolDef 返回 request_help 的 LLM 工具定义。
// 子智能体使用此工具异步向主智能体发送求助请求。
func RequestHelpToolDef() llmclient.ToolDef {
	return llmclient.ToolDef{
		Name:        "request_help",
		Description: "Send an asynchronous help request to the parent agent. Use this when you encounter a problem that you cannot solve independently, need clarification on requirements, or need a decision from the user. The request is non-blocking — you can continue working while waiting for a response. Responses will be injected into your context automatically.",
		InputSchema: json.RawMessage(`{
	"type": "object",
	"properties": {
		"question": {
			"type": "string",
			"description": "The question or help request (≤500 chars). Be specific about what you need."
		},
		"context": {
			"type": "string",
			"description": "Background context: what you're currently doing, what you've tried, and what the problem is (≤300 chars, optional)."
		},
		"options": {
			"type": "array",
			"description": "Suggested solution options for the parent/user to choose from (optional).",
			"items": { "type": "string" }
		},
		"urgency": {
			"type": "string",
			"enum": ["low", "medium", "high"],
			"description": "Urgency level. 'low' = informational, 'medium' = need answer soon, 'high' = blocked without answer."
		}
	},
	"required": ["question"]
}`),
	}
}

// ReportProgressToolDef 返回 report_progress 的 LLM 工具定义。
// 智能体使用此工具主动汇报中间进度，更新实时事件流；最终结果仍需正常回复/投递。
func ReportProgressToolDef() llmclient.ToolDef {
	return llmclient.ToolDef{
		Name: "report_progress",
		Description: `Report intermediate progress to the user. Use this tool proactively during long-running tasks to keep the user informed:
- After completing a significant step (e.g., finished reading files, built the code, ran tests)
- When starting a new phase of work
- When encountering delays or waiting for resources
This is best for real-time progress surfaces. It is not the final answer. Remote chat channels only receive these updates when the gateway entrypoint explicitly enables throttled progress delivery; otherwise assume live UI/task surfaces only. The report is non-blocking — the event is emitted immediately and you continue working.`,
		InputSchema: json.RawMessage(`{
	"type": "object",
	"properties": {
		"summary": {
			"type": "string",
			"description": "Brief progress summary (≤300 chars). Example: 'Completed code analysis, found 3 issues. Starting fixes now.'"
		},
		"percent": {
			"type": "integer",
			"description": "Estimated completion percentage (0-100, optional). Omit if hard to estimate."
		},
		"phase": {
			"type": "string",
			"description": "Current work phase (optional). Example: 'analyzing', 'implementing', 'testing', 'reviewing'."
		}
	},
	"required": ["summary"]
}`),
	}
}

// ExecuteRequestHelp 执行 request_help 工具调用。
// 向主智能体发送求助请求（非阻塞），返回确认消息。
func ExecuteRequestHelp(inputJSON json.RawMessage, ch *AgentChannel) (string, error) {
	if ch == nil {
		return "[request_help] Help channel not available in this context.", nil
	}

	var input HelpRequestPayload
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid request_help input: %w", err)
	}

	if input.Question == "" {
		return "[request_help] 'question' is required.", nil
	}
	if len([]rune(input.Question)) > 500 {
		input.Question = string([]rune(input.Question)[:500])
	}
	if len([]rune(input.Context)) > 300 {
		input.Context = string([]rune(input.Context)[:300])
	}
	if len(input.Options) > 5 {
		input.Options = input.Options[:5]
	}
	if input.Urgency == "" {
		input.Urgency = "medium"
	}

	msgID, err := ch.SendHelpRequest(input)
	if err != nil {
		return fmt.Sprintf("[request_help] Failed to send: %s", err), nil
	}

	slog.Debug("request_help sent",
		"msgID", msgID,
		"urgency", input.Urgency,
		"question", input.Question,
	)

	return fmt.Sprintf("[request_help] Help request sent (ID: %s). "+
		"Continue working — the response will appear in your context automatically when available.", msgID), nil
}
