package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
)

type feishuRewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t *feishuRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = t.target.Scheme
	cloned.URL.Host = t.target.Host
	return t.base.RoundTrip(cloned)
}

func TestFeishuBroadcastCard_FallbackToOpenIDWhenChatTargetsFail(t *testing.T) {
	var chatCalls atomic.Int32
	var userCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		targetType := r.URL.Query().Get("receive_id_type")
		switch targetType {
		case "chat_id":
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":999,"msg":"chat failed"}`))
		case "open_id":
			userCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		default:
			_, _ = w.Write([]byte(`{"code":998,"msg":"unknown target"}`))
		}
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	provider := &feishuProvider{
		config: &FeishuProviderConfig{
			ChatID: "oc_chat_1",
			UserID: "ou_user_1",
		},
		client: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &feishuRewriteTransport{
				target: targetURL,
				base:   http.DefaultTransport,
			},
		},
	}

	card := map[string]interface{}{
		"header": map[string]interface{}{"title": "test"},
	}
	if err := provider.broadcastCard(context.Background(), "dummy-token", card); err != nil {
		t.Fatalf("broadcast card should succeed via open_id fallback, got: %v", err)
	}

	if chatCalls.Load() != 1 {
		t.Fatalf("expected 1 chat send attempt, got %d", chatCalls.Load())
	}
	if userCalls.Load() != 1 {
		t.Fatalf("expected 1 open_id fallback send attempt, got %d", userCalls.Load())
	}
}

func TestFeishuBroadcastCard_NoOpenIDWhenChatHasSuccess(t *testing.T) {
	var chatCalls atomic.Int32
	var userCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		targetType := r.URL.Query().Get("receive_id_type")
		switch targetType {
		case "chat_id":
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		case "open_id":
			userCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		default:
			_, _ = w.Write([]byte(`{"code":998,"msg":"unknown target"}`))
		}
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	provider := &feishuProvider{
		config: &FeishuProviderConfig{
			ChatID: "oc_chat_1",
			UserID: "ou_user_1",
		},
		client: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &feishuRewriteTransport{
				target: targetURL,
				base:   http.DefaultTransport,
			},
		},
	}

	card := map[string]interface{}{
		"header": map[string]interface{}{"title": "test"},
	}
	if err := provider.broadcastCard(context.Background(), "dummy-token", card); err != nil {
		t.Fatalf("broadcast card should succeed on chat target, got: %v", err)
	}

	if chatCalls.Load() != 1 {
		t.Fatalf("expected 1 chat send attempt, got %d", chatCalls.Load())
	}
	if userCalls.Load() != 0 {
		t.Fatalf("expected no open_id fallback when chat succeeds, got %d", userCalls.Load())
	}
}

func TestFeishuBroadcastCard_DeduplicatesSameTargetAcrossStaticAndOriginatorSources(t *testing.T) {
	var chatCalls atomic.Int32
	var userCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		targetType := r.URL.Query().Get("receive_id_type")
		switch targetType {
		case "chat_id":
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		case "open_id":
			userCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		default:
			_, _ = w.Write([]byte(`{"code":998,"msg":"unknown target"}`))
		}
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	provider := &feishuProvider{
		config: &FeishuProviderConfig{
			ChatID:          "oc_chat_dup",
			ApprovalChatID:  "oc_chat_dup",
			LastKnownChatID: "oc_chat_dup",
			UserID:          "ou_user_dup",
			LastKnownUserID: "ou_user_dup",
		},
		client: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &feishuRewriteTransport{
				target: targetURL,
				base:   http.DefaultTransport,
			},
		},
	}

	card := map[string]interface{}{
		"header": map[string]interface{}{"title": "test"},
	}
	if err := provider.broadcastCard(
		context.Background(),
		"dummy-token",
		card,
		feishuTarget{"chat_id", "oc_chat_dup"},
		feishuTarget{"open_id", "ou_user_dup"},
	); err != nil {
		t.Fatalf("broadcast card should succeed, got: %v", err)
	}

	if chatCalls.Load() != 1 {
		t.Fatalf("expected duplicated chat targets to be collapsed to 1 send, got %d", chatCalls.Load())
	}
	if userCalls.Load() != 0 {
		t.Fatalf("expected no open_id fallback when deduplicated chat succeeds, got %d", userCalls.Load())
	}
}

func TestParseFeishuConfig_ParsesFallbackFields(t *testing.T) {
	raw := map[string]interface{}{
		"enabled":         true,
		"appId":           "cli_test",
		"appSecret":       "secret",
		"chatId":          "oc_chat",
		"userId":          "ou_user",
		"approvalChatId":  "oc_approval",
		"lastKnownChatId": "oc_last",
		"lastKnownUserId": "ou_last",
	}

	cfg := parseFeishuConfig(raw)
	if cfg == nil {
		t.Fatal("expected parsed feishu config")
	}
	if !cfg.Enabled || cfg.AppID != "cli_test" || cfg.AppSecret != "secret" {
		t.Fatalf("basic fields parse mismatch: %+v", cfg)
	}
	if cfg.ApprovalChatID != "oc_approval" {
		t.Fatalf("approvalChatId parse mismatch: %q", cfg.ApprovalChatID)
	}
	if cfg.LastKnownChatID != "oc_last" {
		t.Fatalf("lastKnownChatId parse mismatch: %q", cfg.LastKnownChatID)
	}
	if cfg.LastKnownUserID != "ou_last" {
		t.Fatalf("lastKnownUserId parse mismatch: %q", cfg.LastKnownUserID)
	}
}

func TestFeishuApprovalCard_FullUsesPermanentMode(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.buildApprovalCard(ApprovalCardRequest{
		EscalationID:   "esc_perm_001",
		RequestedLevel: "full",
		Reason:         "deploy gateway",
		TTLMinutes:     0,
		RequestedAt:    time.Now(),
	})

	elements := card["elements"].([]interface{})
	fields := elements[0].(map[string]interface{})["fields"].([]interface{})
	modeText := fields[1].(map[string]interface{})["text"].(map[string]interface{})["content"].(string)
	if modeText != "**授权模式**\n永久授权 / Permanent" {
		t.Fatalf("unexpected mode text: %q", modeText)
	}

	actions := elements[3].(map[string]interface{})["actions"].([]interface{})
	approveValue := actions[0].(map[string]interface{})["value"].(map[string]interface{})
	if _, ok := approveValue["ttl"]; ok {
		t.Fatal("full approval card should not carry ttl in approve payload")
	}
}

func TestFeishuResultCard_FullDoesNotMentionExpiry(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.buildResultCard(ApprovalResultNotification{
		EscalationID:   "esc_perm_001",
		Approved:       true,
		RequestedLevel: "full",
	})

	elements := card["elements"].([]interface{})
	bodyText := elements[0].(map[string]interface{})["text"].(map[string]interface{})["content"].(string)
	if !containsAll(bodyText, "永久授权", "手动改回") {
		t.Fatalf("expected permanent wording in result body, got %q", bodyText)
	}
	if containsAll(bodyText, "自动降级") {
		t.Fatalf("full approval result should not mention automatic downgrade: %q", bodyText)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

// ---------- P6-10: 四种类型化审批卡片渲染测试 ----------

func TestFeishuTypedCard_PlanConfirm(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedCard(TypedApprovalRequest{
		Type:       ApprovalTypePlanConfirm,
		ID:         "test-plan-1",
		TaskBrief:  "创建一个新文件",
		PlanSteps:  []string{"读取模板", "生成代码", "写入文件"},
		IntentTier: "task_write",
		TTLMinutes: 30,
	})

	header := card["header"].(map[string]interface{})
	if header["template"] != "blue" {
		t.Errorf("plan_confirm card template = %q, want blue", header["template"])
	}

	if _, err := json.Marshal(card); err != nil {
		t.Fatalf("plan_confirm card not JSON serializable: %v", err)
	}
}

func TestFeishuPlanConfirmCard_IncludesApprovalSummary(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.buildPlanConfirmCard(PlanConfirmCardRequest{
		ConfirmID:       "plan-1",
		TaskBrief:       "把 report.pdf 发到飞书",
		PlanSteps:       []string{"发送文件 /Users/test/Desktop/report.pdf"},
		ApprovalSummary: []string{"主审批: data_export（对外发送 /Users/test/Desktop/report.pdf）", "附加审批: mount_access（如超出当前作用域，ro 挂载 /Users/test/Desktop）"},
		IntentTier:      "task_light",
		TTLMinutes:      5,
	})

	elements := card["elements"].([]interface{})
	bodyText := elements[0].(map[string]interface{})["text"].(map[string]interface{})["content"].(string)
	if !containsAll(bodyText, "审批摘要", "data_export", "mount_access") {
		t.Fatalf("expected approval summary in plan confirm card, got %q", bodyText)
	}
}

func TestFeishuTypedCard_ExecEscalation(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedCard(TypedApprovalRequest{
		Type:           ApprovalTypeExecEscalation,
		ID:             "test-exec-1",
		RequestedLevel: "full",
		Command:        "rm -rf /tmp/old_data",
		RiskLevel:      "high",
		Reason:         "清理旧数据",
		TTLMinutes:     60,
		RequestedAt:    time.Now(),
	})

	header := card["header"].(map[string]interface{})
	if header["template"] != "red" {
		t.Errorf("exec_escalation card template = %q, want red", header["template"])
	}

	if _, err := json.Marshal(card); err != nil {
		t.Fatalf("exec_escalation card not JSON serializable: %v", err)
	}
}

func TestFeishuTypedCard_MountAccess(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedCard(TypedApprovalRequest{
		Type:       ApprovalTypeMountAccess,
		ID:         "test-mount-1",
		MountPath:  "/Users/admin/Documents",
		MountMode:  "rw",
		Reason:     "需要读写用户文档目录",
		TTLMinutes: 30,
	})

	header := card["header"].(map[string]interface{})
	if header["template"] != "orange" {
		t.Errorf("mount_access card template = %q, want orange", header["template"])
	}

	if _, err := json.Marshal(card); err != nil {
		t.Fatalf("mount_access card not JSON serializable: %v", err)
	}
}

func TestFeishuTypedCard_MountAccessTaskScopedOmitsTTL(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedCard(TypedApprovalRequest{
		Type:       ApprovalTypeMountAccess,
		ID:         "test-mount-task-1",
		MountPath:  "/Users/admin/Documents",
		MountMode:  "ro",
		Reason:     "需要读取用户文档目录",
		TaskScoped: true,
	})

	elements := card["elements"].([]interface{})
	bodyText := elements[0].(map[string]interface{})["text"].(map[string]interface{})["content"].(string)
	if !containsAll(bodyText, "当前任务", "自动回收") {
		t.Fatalf("expected task-scoped wording in mount_access card, got %q", bodyText)
	}

	actions := elements[2].(map[string]interface{})["actions"].([]interface{})
	approveValue := actions[0].(map[string]interface{})["value"].(map[string]interface{})
	if _, ok := approveValue["ttl"]; ok {
		t.Fatalf("task-scoped mount approval should omit ttl payload, got %+v", approveValue)
	}

	noteText := elements[3].(map[string]interface{})["elements"].([]interface{})[0].(map[string]interface{})["content"].(string)
	if !containsAll(noteText, "范围: 当前任务") {
		t.Fatalf("expected task-scoped note in mount_access card, got %q", noteText)
	}
}

func TestFeishuTypedCard_DataExport(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedCard(TypedApprovalRequest{
		Type:          ApprovalTypeDataExport,
		ID:            "test-export-1",
		TargetChannel: "飞书群-项目组",
		ExportFiles:   []string{"report.pdf", "data.csv"},
		Sanitized:     true,
		Reason:        "发送项目报告",
		TTLMinutes:    30,
	})

	header := card["header"].(map[string]interface{})
	if header["template"] != "purple" {
		t.Errorf("data_export card template = %q, want purple", header["template"])
	}

	if _, err := json.Marshal(card); err != nil {
		t.Fatalf("data_export card not JSON serializable: %v", err)
	}
}

func TestFeishuTypedResultCard_MountAccess(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedResultCard(TypedApprovalResultNotification{
		Type:       ApprovalTypeMountAccess,
		ID:         "mount-result-1",
		Approved:   true,
		TTLMinutes: 30,
		MountPath:  "/Users/admin/Documents",
		MountMode:  "ro",
	})

	header := card["header"].(map[string]interface{})
	if header["template"] != "green" {
		t.Errorf("mount_access result card template = %q, want green", header["template"])
	}

	if _, err := json.Marshal(card); err != nil {
		t.Fatalf("mount_access result card not JSON serializable: %v", err)
	}
}

func TestFeishuTypedResultCard_MountAccessTaskScoped(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedResultCard(TypedApprovalResultNotification{
		Type:       ApprovalTypeMountAccess,
		ID:         "mount-result-task-1",
		Approved:   true,
		MountPath:  "/Users/admin/Documents",
		MountMode:  "ro",
		TaskScoped: true,
	})

	elements := card["elements"].([]interface{})
	bodyText := elements[0].(map[string]interface{})["text"].(map[string]interface{})["content"].(string)
	if !containsAll(bodyText, "当前任务", "自动回收") {
		t.Fatalf("expected task-scoped wording in mount_access result card, got %q", bodyText)
	}
}

func TestFeishuTypedResultCard_DataExport(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	card := provider.renderTypedResultCard(TypedApprovalResultNotification{
		Type:          ApprovalTypeDataExport,
		ID:            "export-result-1",
		Approved:      false,
		Reason:        "管理员拒绝",
		TargetChannel: "feishu:oc_target",
		ExportFiles:   []string{"report.pdf"},
	})

	header := card["header"].(map[string]interface{})
	if header["template"] != "red" {
		t.Errorf("data_export result card template = %q, want red", header["template"])
	}

	if _, err := json.Marshal(card); err != nil {
		t.Fatalf("data_export result card not JSON serializable: %v", err)
	}
}

func TestFeishuTypedCard_ResultReviewIncludesWorkflow(t *testing.T) {
	provider := &feishuProvider{config: &FeishuProviderConfig{}}
	workflow := runner.NewSingleStageApprovalWorkflow(
		"签收子智能体结果",
		runner.ApprovalTypeResultReview,
		"result_review（交付前最终签收）",
	).MarkStagePending(runner.ApprovalTypeResultReview, "result-1")
	card := provider.renderTypedCard(TypedApprovalRequest{
		Type:          ApprovalTypeResultReview,
		ID:            "result-1",
		ResultSummary: "报告已生成，等待签收",
		ReviewSummary: "质量审核通过",
		TTLMinutes:    3,
		Workflow:      workflow,
	})

	elements := card["elements"].([]interface{})
	bodyText := ""
	for _, item := range elements {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		text, ok := block["text"].(map[string]interface{})
		if !ok {
			continue
		}
		if content, ok := text["content"].(string); ok {
			bodyText += content
		}
	}
	if !containsAll(bodyText, "审批流程", "当前阶段", "result_review") {
		t.Fatalf("expected workflow details in result_review card, got %q", bodyText)
	}
}

func TestFeishuBroadcastCard_RequestBodyStillValidJSON(t *testing.T) {
	var decodeErr atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			decodeErr.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	provider := &feishuProvider{
		config: &FeishuProviderConfig{UserID: "ou_user_1"},
		client: &http.Client{Transport: &feishuRewriteTransport{target: targetURL, base: http.DefaultTransport}},
	}
	if err := provider.broadcastCard(context.Background(), "dummy-token", map[string]interface{}{"k": "v"}); err != nil {
		t.Fatalf("broadcast card should succeed: %v", err)
	}
	if decodeErr.Load() {
		t.Fatal("expected valid JSON request body")
	}
}
