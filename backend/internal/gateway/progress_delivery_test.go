package gateway

import (
	"context"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

type recordingProgressSendPlugin struct {
	id   channels.ChannelID
	sent []channels.OutboundSendParams
}

func (p *recordingProgressSendPlugin) ID() channels.ChannelID { return p.id }

func (p *recordingProgressSendPlugin) Start(accountID string) error { return nil }

func (p *recordingProgressSendPlugin) Stop(accountID string) error { return nil }

func (p *recordingProgressSendPlugin) SendMessage(params channels.OutboundSendParams) (*channels.OutboundSendResult, error) {
	p.sent = append(p.sent, params)
	return &channels.OutboundSendResult{
		Channel:   string(p.id),
		MessageID: "msg-progress",
		ChatID:    params.To,
	}, nil
}

func TestBuildRemoteProgressCallback_DeliversAndThrottles(t *testing.T) {
	t.Parallel()

	mgr := channels.NewManager()
	plugin := &recordingProgressSendPlugin{id: channels.ChannelSlack}
	mgr.RegisterPlugin(plugin)

	cb := buildRemoteProgressCallback(mgr, progressDeliveryTarget{
		Channel:   "slack",
		To:        "C123",
		AccountID: "acc-1",
		ThreadID:  "thread-1",
	})
	if cb == nil {
		t.Fatal("expected progress callback")
	}

	first := cb(context.Background(), runner.ProgressUpdate{
		Summary: "Built assets",
		Phase:   "building",
		Percent: 40,
	})
	if !first.RemoteDelivered {
		t.Fatalf("expected first update to be delivered: %+v", first)
	}
	if len(plugin.sent) != 1 {
		t.Fatalf("expected 1 progress send, got %d", len(plugin.sent))
	}
	got := plugin.sent[0]
	if got.To != "C123" || got.AccountID != "acc-1" || got.ThreadID != "thread-1" {
		t.Fatalf("unexpected outbound target: %+v", got)
	}
	if got.Text != "[Progress] Built assets (building | 40%)" {
		t.Fatalf("unexpected progress text: %q", got.Text)
	}

	second := cb(context.Background(), runner.ProgressUpdate{
		Summary: "Running tests",
		Phase:   "testing",
		Percent: 60,
	})
	if !second.Throttled {
		t.Fatalf("expected second update to be throttled: %+v", second)
	}
	if len(plugin.sent) != 1 {
		t.Fatalf("expected throttled update to skip send, got %d sends", len(plugin.sent))
	}
}

func TestDispatchInboundMessage_PassesProgressCallback(t *testing.T) {
	t.Parallel()

	progressCb := func(context.Context, runner.ProgressUpdate) runner.ProgressReportStatus {
		return runner.ProgressReportStatus{RemoteDelivered: true}
	}

	var captured *autoreply.GetReplyOptions
	result := DispatchInboundMessage(context.Background(), DispatchInboundParams{
		MsgCtx: &autoreply.MsgContext{
			Body: "hello",
		},
		Dispatcher: func(ctx context.Context, msgCtx *autoreply.MsgContext, opts *autoreply.GetReplyOptions) ([]autoreply.ReplyPayload, error) {
			captured = opts
			return nil, nil
		},
		OnProgress: progressCb,
	})
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if captured == nil {
		t.Fatal("expected dispatcher to receive GetReplyOptions")
	}
	if captured.OnProgress == nil {
		t.Fatal("expected progress callback to be propagated")
	}
	if _, ok := captured.OnProgress.(func(context.Context, runner.ProgressUpdate) runner.ProgressReportStatus); !ok {
		t.Fatalf("unexpected OnProgress type: %T", captured.OnProgress)
	}
}

func TestBuildChatProgressCallback_BroadcastsAndThrottles(t *testing.T) {
	t.Parallel()

	bc := NewBroadcaster()
	cb := buildChatProgressCallback(bc, "sess-123")
	if cb == nil {
		t.Fatal("expected non-nil callback")
	}

	// First call should deliver
	first := cb(context.Background(), runner.ProgressUpdate{
		Summary: "Compiling",
		Phase:   "build",
		Percent: 30,
	})
	if !first.RemoteDelivered {
		t.Fatalf("expected first update delivered: %+v", first)
	}

	// Same fingerprint → throttled
	dup := cb(context.Background(), runner.ProgressUpdate{
		Summary: "Compiling",
		Phase:   "build",
		Percent: 30,
	})
	if !dup.Throttled {
		t.Fatalf("expected duplicate to be throttled: %+v", dup)
	}

	// Different summary but within interval → throttled
	diff := cb(context.Background(), runner.ProgressUpdate{
		Summary: "Running tests",
		Phase:   "test",
		Percent: 60,
	})
	if !diff.Throttled {
		t.Fatalf("expected interval-throttled: %+v", diff)
	}
}

func TestBuildChatProgressCallback_NilBroadcaster(t *testing.T) {
	t.Parallel()

	cb := buildChatProgressCallback(nil, "sess-123")
	if cb != nil {
		t.Fatal("expected nil callback for nil broadcaster")
	}
}

func TestBuildChatProgressCallback_EmptySessionKey(t *testing.T) {
	t.Parallel()

	bc := NewBroadcaster()
	cb := buildChatProgressCallback(bc, "")
	if cb != nil {
		t.Fatal("expected nil callback for empty sessionKey")
	}
}
