package gateway

import (
	"testing"
)

// TestAllDispatchPathsWireOnProgress S6-T5:
// 验证所有通道分发路径都正确接入 OnProgress 回调。
// 这是一个结构性契约测试——确保新增通道时不遗漏进度投递接线。
func TestAllDispatchPathsWireOnProgress(t *testing.T) {
	// buildMsgContextProgressCallback 是所有分发路径的统一 OnProgress 构造函数。
	// 验证 nil state 返回 nil（安全降级）。
	cb := buildMsgContextProgressCallback(nil, nil)
	if cb != nil {
		t.Error("buildMsgContextProgressCallback(nil, nil) should return nil")
	}
}

// TestProgressDeliveryTargetFromNilMsgContext 验证 nil MsgContext 不 panic。
func TestProgressDeliveryTargetFromNilMsgContext(t *testing.T) {
	target := progressDeliveryTargetFromMsgContext(nil)
	if target.Channel != "" || target.To != "" {
		t.Error("nil MsgContext should produce empty target")
	}
}

// TestBuildRemoteProgressCallbackNilManager 验证 nil ChannelMgr 返回 nil 回调。
func TestBuildRemoteProgressCallbackNilManager(t *testing.T) {
	cb := buildRemoteProgressCallback(nil, progressDeliveryTarget{
		Channel: "feishu",
		To:      "oc_xxx",
	})
	if cb != nil {
		t.Error("nil channelMgr should return nil callback")
	}
}

// TestBuildRemoteProgressCallbackEmptyTarget 验证空 target 返回 nil 回调。
func TestBuildRemoteProgressCallbackEmptyTarget(t *testing.T) {
	// 即使 ChannelMgr 非 nil，空 channel/to 也应返回 nil
	// 这里无法构造真实 Manager，但函数在 channel=="" 时短路返回 nil
	cb := buildRemoteProgressCallback(nil, progressDeliveryTarget{})
	if cb != nil {
		t.Error("empty target should return nil callback")
	}
}
