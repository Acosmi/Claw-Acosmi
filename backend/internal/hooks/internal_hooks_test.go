package hooks

import (
	"errors"
	"sync"
	"testing"
)

// ============================================================================
// internal_hooks_test.go — 内部钩子注册表测试
// ============================================================================

func TestRegisterAndTrigger(t *testing.T) {
	ClearInternalHooks()
	defer ClearInternalHooks()

	var calls []string
	var mu sync.Mutex

	RegisterInternalHook("command", func(event *InternalHookEvent) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, "type:"+string(event.Type))
		return nil
	})

	RegisterInternalHook("command:new", func(event *InternalHookEvent) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, "specific:"+event.Action)
		return nil
	})

	event := CreateInternalHookEvent(HookEventCommand, "new", "sess-1", nil)
	TriggerInternalHook(event)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(calls), calls)
	}
	if calls[0] != "type:command" {
		t.Errorf("expected type handler first, got %s", calls[0])
	}
	if calls[1] != "specific:new" {
		t.Errorf("expected specific handler second, got %s", calls[1])
	}
}

func TestTriggerNoHandlers(t *testing.T) {
	ClearInternalHooks()
	defer ClearInternalHooks()

	// Should not panic
	event := CreateInternalHookEvent(HookEventSession, "start", "sess-1", nil)
	TriggerInternalHook(event)
}

func TestTriggerHandlerError(t *testing.T) {
	ClearInternalHooks()
	defer ClearInternalHooks()

	called := false
	RegisterInternalHook("command", func(event *InternalHookEvent) error {
		return errors.New("handler failed")
	})
	RegisterInternalHook("command", func(event *InternalHookEvent) error {
		called = true
		return nil
	})

	event := CreateInternalHookEvent(HookEventCommand, "test", "sess-1", nil)
	TriggerInternalHook(event) // Should not panic even with error

	if !called {
		t.Error("second handler should still be called after first one errors")
	}
}

func TestClearInternalHooks(t *testing.T) {
	ClearInternalHooks()
	defer ClearInternalHooks()

	RegisterInternalHook("test", func(event *InternalHookEvent) error { return nil })
	keys := GetRegisteredEventKeys()
	if len(keys) == 0 {
		t.Fatal("expected at least one key")
	}

	ClearInternalHooks()
	keys = GetRegisteredEventKeys()
	if len(keys) != 0 {
		t.Errorf("expected no keys after clear, got %d", len(keys))
	}
}

func TestCreateInternalHookEvent(t *testing.T) {
	event := CreateInternalHookEvent(HookEventAgent, "bootstrap", "sess-1", map[string]interface{}{
		"workspaceDir": "/tmp/test",
	})
	if event.Type != HookEventAgent {
		t.Errorf("expected type agent, got %s", event.Type)
	}
	if event.Action != "bootstrap" {
		t.Errorf("expected action bootstrap, got %s", event.Action)
	}
	if event.Timestamp == 0 {
		t.Error("timestamp should be non-zero")
	}
	if len(event.Messages) != 0 {
		t.Error("messages should be empty initially")
	}
}

func TestIsAgentBootstrapEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    *InternalHookEvent
		expected bool
	}{
		{
			name: "valid bootstrap event",
			event: &InternalHookEvent{
				Type: HookEventAgent, Action: "bootstrap",
				Context: map[string]interface{}{
					"workspaceDir":            "/tmp/test",
					"workspaceBootstrapFiles": []interface{}{},
				},
			},
			expected: true,
		},
		{
			name:     "wrong type",
			event:    &InternalHookEvent{Type: HookEventCommand, Action: "bootstrap"},
			expected: false,
		},
		{
			name:     "wrong action",
			event:    &InternalHookEvent{Type: HookEventAgent, Action: "init"},
			expected: false,
		},
		{
			name: "missing workspaceDir",
			event: &InternalHookEvent{
				Type: HookEventAgent, Action: "bootstrap",
				Context: map[string]interface{}{"workspaceBootstrapFiles": []interface{}{}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAgentBootstrapEvent(tt.event); got != tt.expected {
				t.Errorf("IsAgentBootstrapEvent() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetRegisteredEventKeys(t *testing.T) {
	ClearInternalHooks()
	defer ClearInternalHooks()

	RegisterInternalHook("command", func(event *InternalHookEvent) error { return nil })
	RegisterInternalHook("session", func(event *InternalHookEvent) error { return nil })
	RegisterInternalHook("command:new", func(event *InternalHookEvent) error { return nil })

	keys := GetRegisteredEventKeys()
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestConcurrentRegisterAndTrigger(t *testing.T) {
	ClearInternalHooks()
	defer ClearInternalHooks()

	var wg sync.WaitGroup

	// Register handlers concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RegisterInternalHook("command", func(event *InternalHookEvent) error {
				return nil
			})
		}()
	}

	// Trigger concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := CreateInternalHookEvent(HookEventCommand, "test", "s", nil)
			TriggerInternalHook(event)
		}()
	}

	wg.Wait()
}
