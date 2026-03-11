package gateway

import (
	"strings"
	"testing"
)

// ---------- AuthorizeGatewayMethod ----------

func TestAuthorizeGatewayMethod_NilClient(t *testing.T) {
	if err := AuthorizeGatewayMethod("sessions.list", nil); err != nil {
		t.Errorf("nil client should be allowed, got %v", err)
	}
}

func TestAuthorizeGatewayMethod_AdminScope(t *testing.T) {
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.admin"},
	}}
	// Admin can do anything
	for _, method := range []string{"sessions.list", "config.get", "chat.send"} {
		if err := AuthorizeGatewayMethod(method, client); err != nil {
			t.Errorf("admin should access %q, got %v", method, err)
		}
	}
}

func TestAuthorizeGatewayMethod_ReadScope(t *testing.T) {
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.read"},
	}}
	if err := AuthorizeGatewayMethod("sessions.list", client); err != nil {
		t.Errorf("read scope should access sessions.list, got %v", err)
	}
	if err := AuthorizeGatewayMethod("config.get", client); err == nil {
		t.Error("read scope should NOT access config.get")
	}
}

func TestAuthorizeGatewayMethod_WriteScope(t *testing.T) {
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.write"},
	}}
	if err := AuthorizeGatewayMethod("chat.send", client); err != nil {
		t.Errorf("write scope should access chat.send, got %v", err)
	}
	if err := AuthorizeGatewayMethod("sessions.delete", client); err == nil {
		t.Error("write scope should NOT access sessions.delete (admin)")
	}
}

func TestAuthorizeGatewayMethod_NodeRole(t *testing.T) {
	client := &GatewayClient{Connect: &ConnectParamsFull{Role: "node"}}
	if err := AuthorizeGatewayMethod("node.invoke.result", client); err != nil {
		t.Errorf("node should access node.invoke.result, got %v", err)
	}
	if err := AuthorizeGatewayMethod("sessions.list", client); err == nil {
		t.Error("node should NOT access sessions.list")
	}
}

// ---------- MethodRegistry ----------

func TestMethodRegistry_RegisterAndGet(t *testing.T) {
	r := NewMethodRegistry()
	called := false
	r.Register("test.method", func(ctx *MethodHandlerContext) {
		called = true
	})

	handler := r.Get("test.method")
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
	handler(&MethodHandlerContext{})
	if !called {
		t.Error("handler was not called")
	}
}

func TestMethodRegistry_UnknownMethod(t *testing.T) {
	r := NewMethodRegistry()
	if r.Get("nonexistent") != nil {
		t.Error("unknown method should return nil")
	}
}

// ---------- HandleGatewayRequest ----------

func TestHandleGatewayRequest_UnknownMethod(t *testing.T) {
	r := NewMethodRegistry()
	req := &RequestFrame{Method: "nonexistent"}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("should not be ok")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request error, got %v", gotErr)
	}
}

func TestHandleGatewayRequest_Success(t *testing.T) {
	r := NewMethodRegistry()
	r.Register("echo", func(ctx *MethodHandlerContext) {
		ctx.Respond(true, ctx.Params, nil)
	})
	req := &RequestFrame{Method: "echo", Params: map[string]interface{}{"key": "val"}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Error("should be ok")
	}
	m, _ := gotPayload.(map[string]interface{})
	if m["key"] != "val" {
		t.Errorf("expected key=val, got %v", gotPayload)
	}
}

// ---------- SessionsHandlers ----------

func TestSessionsHandlers_ListEmpty(t *testing.T) {
	store := NewSessionStore("")
	r := NewMethodRegistry()
	r.RegisterAll(SessionsHandlers())

	req := &RequestFrame{Method: "sessions.list", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{SessionStore: store}, respond)
	if !gotOK {
		t.Error("should be ok")
	}
	result, ok := gotPayload.(SessionsListResult)
	if !ok {
		t.Fatalf("expected SessionsListResult, got %T", gotPayload)
	}
	if result.Count != 0 {
		t.Errorf("expected 0 sessions, got %d", result.Count)
	}
}

func TestSessionsHandlers_PatchAndResolve(t *testing.T) {
	store := NewSessionStore("")
	r := NewMethodRegistry()
	r.RegisterAll(SessionsHandlers())

	// Patch
	patchReq := &RequestFrame{Method: "sessions.patch", Params: map[string]interface{}{
		"key": "test-session", "displayName": "My Session",
	}}
	var patchOK bool
	HandleGatewayRequest(r, patchReq, nil, &GatewayMethodContext{SessionStore: store},
		func(ok bool, _ interface{}, _ *ErrorShape) { patchOK = ok })
	if !patchOK {
		t.Error("patch should succeed")
	}

	// Resolve
	resolveReq := &RequestFrame{Method: "sessions.resolve", Params: map[string]interface{}{"key": "test-session"}}
	var resolvePayload interface{}
	HandleGatewayRequest(r, resolveReq, nil, &GatewayMethodContext{SessionStore: store},
		func(_ bool, payload interface{}, _ *ErrorShape) { resolvePayload = payload })
	m, _ := resolvePayload.(map[string]interface{})
	if m["key"] != "test-session" {
		t.Errorf("expected key=test-session, got %v", m)
	}
}

func TestSessionsHandlers_Delete(t *testing.T) {
	store := NewSessionStore("")
	store.Save(&SessionEntry{SessionKey: "to-delete"})
	r := NewMethodRegistry()
	r.RegisterAll(SessionsHandlers())

	req := &RequestFrame{Method: "sessions.delete", Params: map[string]interface{}{"key": "to-delete"}}
	var gotOK bool
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{SessionStore: store},
		func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok })
	if !gotOK {
		t.Error("delete should succeed")
	}
	if store.Count() != 0 {
		t.Errorf("store should be empty, got %d", store.Count())
	}
}

// ---------- P2: sessions.create ----------

func TestSessionsHandlers_Create(t *testing.T) {
	store := NewSessionStore("")
	r := NewMethodRegistry()
	r.RegisterAll(SessionsHandlers())

	req := &RequestFrame{Method: "sessions.create", Params: map[string]interface{}{
		"label": "test session",
	}}
	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{SessionStore: store},
		func(ok bool, payload interface{}, err *ErrorShape) {
			gotOK = ok
			gotPayload = payload
		})
	if !gotOK {
		t.Fatal("sessions.create should succeed")
	}

	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	sessionKey, _ := m["sessionKey"].(string)
	sessionId, _ := m["sessionId"].(string)
	if !strings.HasPrefix(sessionKey, "user:") {
		t.Fatalf("sessionKey should have user: prefix, got %q", sessionKey)
	}
	if sessionId == "" {
		t.Fatal("sessionId should not be empty")
	}

	// 验证 store 中已持久化
	entry := store.LoadSessionEntry(sessionKey)
	if entry == nil {
		t.Fatal("session should be persisted in store")
	}
	if entry.Label != "test session" {
		t.Fatalf("label = %q, want 'test session'", entry.Label)
	}
	if entry.CreatedAt == 0 {
		t.Fatal("CreatedAt should be set")
	}
}

func TestSessionsHandlers_CreateMultiple(t *testing.T) {
	store := NewSessionStore("")
	r := NewMethodRegistry()
	r.RegisterAll(SessionsHandlers())

	keys := make(map[string]bool)
	for i := 0; i < 3; i++ {
		var payload interface{}
		HandleGatewayRequest(r, &RequestFrame{Method: "sessions.create", Params: map[string]interface{}{}}, nil,
			&GatewayMethodContext{SessionStore: store},
			func(ok bool, p interface{}, _ *ErrorShape) { payload = p })
		m, _ := payload.(map[string]interface{})
		key, _ := m["sessionKey"].(string)
		if keys[key] {
			t.Fatalf("duplicate sessionKey: %q", key)
		}
		keys[key] = true
	}
	if store.Count() != 3 {
		t.Fatalf("expected 3 sessions, got %d", store.Count())
	}
}

// ---------- P2: sessions.ensureMain ----------

func TestSessionsHandlers_EnsureMain_CreatesIfMissing(t *testing.T) {
	store := NewSessionStore("")
	r := NewMethodRegistry()
	r.RegisterAll(SessionsHandlers())

	req := &RequestFrame{Method: "sessions.ensureMain", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{SessionStore: store},
		func(ok bool, payload interface{}, _ *ErrorShape) {
			gotOK = ok
			gotPayload = payload
		})
	if !gotOK {
		t.Fatal("sessions.ensureMain should succeed")
	}

	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	sessionKey, _ := m["sessionKey"].(string)
	created, _ := m["created"].(bool)
	if sessionKey == "" {
		t.Fatal("sessionKey should not be empty")
	}
	if !created {
		t.Fatal("should report created=true for new main session")
	}

	// 验证 store 中已持久化
	entry := store.LoadSessionEntry(sessionKey)
	if entry == nil {
		t.Fatal("main session should be persisted in store")
	}
}

func TestSessionsHandlers_EnsureMain_Idempotent(t *testing.T) {
	store := NewSessionStore("")
	r := NewMethodRegistry()
	r.RegisterAll(SessionsHandlers())

	mctx := &GatewayMethodContext{SessionStore: store}

	// 第一次调用 — 创建
	var firstPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "sessions.ensureMain", Params: map[string]interface{}{}}, nil,
		mctx, func(_ bool, p interface{}, _ *ErrorShape) { firstPayload = p })
	m1, _ := firstPayload.(map[string]interface{})
	key1, _ := m1["sessionKey"].(string)
	id1, _ := m1["sessionId"].(string)
	created1, _ := m1["created"].(bool)
	if !created1 {
		t.Fatal("first call should create")
	}

	// 第二次调用 — 幂等返回
	var secondPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "sessions.ensureMain", Params: map[string]interface{}{}}, nil,
		mctx, func(_ bool, p interface{}, _ *ErrorShape) { secondPayload = p })
	m2, _ := secondPayload.(map[string]interface{})
	key2, _ := m2["sessionKey"].(string)
	id2, _ := m2["sessionId"].(string)
	created2, _ := m2["created"].(bool)

	if key2 != key1 {
		t.Fatalf("sessionKey changed: %q → %q", key1, key2)
	}
	if id2 != id1 {
		t.Fatalf("sessionId changed: %q → %q", id1, id2)
	}
	if created2 {
		t.Fatal("second call should report created=false")
	}
	if store.Count() != 1 {
		t.Fatalf("expected 1 session, got %d", store.Count())
	}
}
