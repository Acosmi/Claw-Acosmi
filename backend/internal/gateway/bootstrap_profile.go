package gateway

type GatewayBootstrapProfile string

const (
	GatewayBootstrapProfileDefault           GatewayBootstrapProfile = ""
	GatewayBootstrapProfileReadonlyBootstrap GatewayBootstrapProfile = "readonly-bootstrap"
)

type gatewayBootstrapPlan struct {
	Profile                      GatewayBootstrapProfile
	DisableFileLogging           bool
	PersistGeneratedGatewayToken bool
	PersistentStore              bool
	StartCron                    bool
	StartChannels                bool
	StartExtensionRelay          bool
	InitializeArgus              bool
	InitializeAuthManager        bool
	InitializeMediaSubsystem     bool
	InitializePackageCenter      bool
}

func resolveGatewayBootstrapPlan(opts GatewayServerOptions) gatewayBootstrapPlan {
	switch opts.BootstrapProfile {
	case GatewayBootstrapProfileReadonlyBootstrap:
		return gatewayBootstrapPlan{
			Profile:                      GatewayBootstrapProfileReadonlyBootstrap,
			DisableFileLogging:           true,
			PersistGeneratedGatewayToken: false,
			PersistentStore:              false,
			StartCron:                    false,
			StartChannels:                false,
			StartExtensionRelay:          false,
			InitializeArgus:              false,
			InitializeAuthManager:        false,
			InitializeMediaSubsystem:     false,
			InitializePackageCenter:      false,
		}
	default:
		return gatewayBootstrapPlan{
			Profile:                      GatewayBootstrapProfileDefault,
			DisableFileLogging:           false,
			PersistGeneratedGatewayToken: true,
			PersistentStore:              true,
			StartCron:                    true,
			StartChannels:                true,
			StartExtensionRelay:          true,
			InitializeArgus:              true,
			InitializeAuthManager:        true,
			InitializeMediaSubsystem:     true,
			InitializePackageCenter:      true,
		}
	}
}
