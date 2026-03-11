package gateway

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestNewGatewayMethodContextIncludesRuntimeRestartDependencies(t *testing.T) {
	state := NewGatewayState()
	sentinel := stubRestartSentinelWriter{}
	restarter := &stubGatewayRestarter{}
	liveCfg := &types.OpenAcosmiConfig{}

	ctx := newGatewayMethodContext(WsServerConfig{
		State:              state,
		SessionStore:       NewSessionStore(""),
		RestartSentinel:    sentinel,
		GatewayRestarter:   restarter,
		StorePath:          "store",
		LogFilePath:        "gateway.log",
		CronStorePath:      "cron",
		MediaSubsystem:     nil,
		SkillStoreClient:   nil,
		RemoteMCPBridge:    nil,
		PipelineDispatcher: nil,
	}, liveCfg)

	if ctx.RestartSentinel != sentinel {
		t.Fatal("restart sentinel was not propagated into method context")
	}
	if ctx.GatewayRestarter != restarter {
		t.Fatal("gateway restarter was not propagated into method context")
	}
	if ctx.Config != liveCfg {
		t.Fatal("live config was not propagated into method context")
	}
	if ctx.State != state {
		t.Fatal("gateway state was not propagated into method context")
	}
}
