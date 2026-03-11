package gateway

import "testing"

func TestResolveGatewayBootstrapPlan_Default(t *testing.T) {
	plan := resolveGatewayBootstrapPlan(GatewayServerOptions{})
	if plan.DisableFileLogging {
		t.Fatal("default plan should keep file logging")
	}
	if !plan.PersistGeneratedGatewayToken {
		t.Fatal("default plan should persist generated gateway token")
	}
	if !plan.PersistentStore {
		t.Fatal("default plan should keep persistent store")
	}
	if !plan.StartCron {
		t.Fatal("default plan should start cron")
	}
	if !plan.StartChannels {
		t.Fatal("default plan should start channels")
	}
	if !plan.StartExtensionRelay {
		t.Fatal("default plan should start extension relay")
	}
	if !plan.InitializeArgus {
		t.Fatal("default plan should initialize Argus")
	}
	if !plan.InitializeAuthManager {
		t.Fatal("default plan should initialize auth manager")
	}
	if !plan.InitializeMediaSubsystem {
		t.Fatal("default plan should initialize media subsystem")
	}
	if !plan.InitializePackageCenter {
		t.Fatal("default plan should initialize package center")
	}
}

func TestResolveGatewayBootstrapPlan_ReadonlyBootstrap(t *testing.T) {
	plan := resolveGatewayBootstrapPlan(GatewayServerOptions{
		BootstrapProfile: GatewayBootstrapProfileReadonlyBootstrap,
	})
	if !plan.DisableFileLogging {
		t.Fatal("readonly bootstrap should disable file logging")
	}
	if plan.PersistGeneratedGatewayToken {
		t.Fatal("readonly bootstrap should not persist generated gateway token")
	}
	if plan.PersistentStore {
		t.Fatal("readonly bootstrap should disable persistent store")
	}
	if plan.StartCron {
		t.Fatal("readonly bootstrap should not start cron")
	}
	if plan.StartChannels {
		t.Fatal("readonly bootstrap should not start channels")
	}
	if plan.StartExtensionRelay {
		t.Fatal("readonly bootstrap should not start extension relay")
	}
	if plan.InitializeArgus {
		t.Fatal("readonly bootstrap should not initialize Argus")
	}
	if plan.InitializeAuthManager {
		t.Fatal("readonly bootstrap should not initialize auth manager")
	}
	if plan.InitializeMediaSubsystem {
		t.Fatal("readonly bootstrap should not initialize media subsystem")
	}
	if plan.InitializePackageCenter {
		t.Fatal("readonly bootstrap should not initialize package center")
	}
}
