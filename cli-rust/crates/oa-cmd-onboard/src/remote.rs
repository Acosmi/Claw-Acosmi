/// Remote gateway configuration for onboarding.
///
/// Handles discovery (Bonjour/mDNS), URL input, and auth configuration
/// for connecting to a remote gateway during onboarding.
///
/// Source: `src/commands/onboard-remote.ts`

use anyhow::Result;
use tracing::info;

use oa_types::config::OpenAcosmiConfig;
use oa_types::gateway::{GatewayConfig, GatewayMode, GatewayRemoteConfig};

use crate::helpers::{detect_binary, DEFAULT_GATEWAY_URL};

/// A discovered gateway beacon from Bonjour/mDNS.
///
/// Source: `src/commands/onboard-remote.ts` - `GatewayBonjourBeacon`
#[derive(Debug, Clone)]
pub struct GatewayBeacon {
    /// Instance name from mDNS.
    pub instance_name: String,
    /// Display name for the beacon.
    pub display_name: Option<String>,
    /// LAN hostname.
    pub host: Option<String>,
    /// LAN-specific hostname.
    pub lan_host: Option<String>,
    /// Tailnet DNS name.
    pub tailnet_dns: Option<String>,
    /// Service port.
    pub port: Option<u16>,
    /// Gateway port (may differ from service port).
    pub gateway_port: Option<u16>,
    /// SSH port for tunnel mode.
    pub ssh_port: Option<u16>,
}

/// Pick the best host from a beacon for connection.
///
/// Prefers tailnet DNS, then LAN host, then generic host.
///
/// Source: `src/commands/onboard-remote.ts` - `pickHost`
pub fn pick_host(beacon: &GatewayBeacon) -> Option<&str> {
    beacon
        .tailnet_dns
        .as_deref()
        .or(beacon.lan_host.as_deref())
        .or(beacon.host.as_deref())
}

/// Build a display label for a gateway beacon.
///
/// Source: `src/commands/onboard-remote.ts` - `buildLabel`
pub fn build_beacon_label(beacon: &GatewayBeacon) -> String {
    let host = pick_host(beacon);
    let port = beacon.gateway_port.or(beacon.port).unwrap_or(19001);
    let title = beacon
        .display_name
        .as_deref()
        .unwrap_or(&beacon.instance_name);
    let hint = host
        .map(|h| format!("{h}:{port}"))
        .unwrap_or_else(|| "host unknown".to_string());
    format!("{title} ({hint})")
}

/// Ensure a value is a valid WebSocket URL, defaulting to the standard gateway URL.
///
/// Source: `src/commands/onboard-remote.ts` - `ensureWsUrl`
pub fn ensure_ws_url(value: &str) -> String {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        DEFAULT_GATEWAY_URL.to_string()
    } else {
        trimmed.to_string()
    }
}

/// Validate that a string is a valid WebSocket URL (starts with ws:// or wss://).
///
/// Source: `src/commands/onboard-remote.ts` - URL validation
pub fn validate_ws_url(value: &str) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.starts_with("ws://") || trimmed.starts_with("wss://") {
        None
    } else {
        Some("URL must start with ws:// or wss://".to_string())
    }
}

/// Check if Bonjour discovery tools are available.
///
/// Source: `src/commands/onboard-remote.ts` - binary detection
pub async fn has_bonjour_tool() -> bool {
    detect_binary("dns-sd").await || detect_binary("avahi-browse").await
}

/// Connection method choice for remote gateway.
///
/// Source: `src/commands/onboard-remote.ts` - connection method selection
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ConnectionMethod {
    /// Direct WebSocket connection.
    Direct,
    /// SSH tunnel (loopback).
    Ssh,
}

/// Build SSH tunnel instructions for display.
///
/// Source: `src/commands/onboard-remote.ts` - SSH tunnel note
pub fn build_ssh_tunnel_instructions(host: &str, ssh_port: Option<u16>) -> String {
    let port_arg = ssh_port
        .map(|p| format!(" -p {p}"))
        .unwrap_or_default();
    [
        "Start a tunnel before using the CLI:",
        &format!("ssh -N -L 19001:127.0.0.1:19001 <user>@{host}{port_arg}"),
        "Docs: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/gateway/remote",
    ]
    .join("\n")
}

/// Apply remote gateway configuration to the config.
///
/// Sets gateway mode to remote and configures the remote URL and token.
///
/// Source: `src/commands/onboard-remote.ts` - `promptRemoteGatewayConfig` result
pub fn apply_remote_gateway_config(
    cfg: OpenAcosmiConfig,
    url: &str,
    token: Option<&str>,
) -> OpenAcosmiConfig {
    let existing_gw = cfg.gateway.unwrap_or_default();

    OpenAcosmiConfig {
        gateway: Some(GatewayConfig {
            mode: Some(GatewayMode::Remote),
            remote: Some(GatewayRemoteConfig {
                url: Some(url.to_string()),
                token: token.map(str::to_string),
                ..Default::default()
            }),
            ..existing_gw
        }),
        ..cfg
    }
}

/// Run the remote gateway configuration prompt (non-interactive stub).
///
/// In the full implementation, this drives Bonjour discovery, URL input,
/// connection method selection, and auth configuration.
///
/// Source: `src/commands/onboard-remote.ts` - `promptRemoteGatewayConfig`
pub async fn prompt_remote_gateway_config(
    cfg: OpenAcosmiConfig,
) -> Result<OpenAcosmiConfig> {
    info!("Remote gateway configuration available via interactive mode.");

    let suggested_url = cfg
        .gateway
        .as_ref()
        .and_then(|g| g.remote.as_ref())
        .and_then(|r| r.url.as_deref())
        .unwrap_or(DEFAULT_GATEWAY_URL)
        .to_string();

    Ok(apply_remote_gateway_config(cfg, &suggested_url, None))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pick_host_prefers_tailnet() {
        let beacon = GatewayBeacon {
            instance_name: "test".to_string(),
            display_name: None,
            host: Some("192.168.1.100".to_string()),
            lan_host: Some("myhost.local".to_string()),
            tailnet_dns: Some("myhost.tailnet.ts.net".to_string()),
            port: Some(19001),
            gateway_port: None,
            ssh_port: None,
        };
        assert_eq!(pick_host(&beacon), Some("myhost.tailnet.ts.net"));
    }

    #[test]
    fn pick_host_falls_back_to_lan() {
        let beacon = GatewayBeacon {
            instance_name: "test".to_string(),
            display_name: None,
            host: Some("192.168.1.100".to_string()),
            lan_host: Some("myhost.local".to_string()),
            tailnet_dns: None,
            port: Some(19001),
            gateway_port: None,
            ssh_port: None,
        };
        assert_eq!(pick_host(&beacon), Some("myhost.local"));
    }

    #[test]
    fn pick_host_falls_back_to_host() {
        let beacon = GatewayBeacon {
            instance_name: "test".to_string(),
            display_name: None,
            host: Some("192.168.1.100".to_string()),
            lan_host: None,
            tailnet_dns: None,
            port: Some(19001),
            gateway_port: None,
            ssh_port: None,
        };
        assert_eq!(pick_host(&beacon), Some("192.168.1.100"));
    }

    #[test]
    fn pick_host_none_when_empty() {
        let beacon = GatewayBeacon {
            instance_name: "test".to_string(),
            display_name: None,
            host: None,
            lan_host: None,
            tailnet_dns: None,
            port: None,
            gateway_port: None,
            ssh_port: None,
        };
        assert!(pick_host(&beacon).is_none());
    }

    #[test]
    fn build_beacon_label_with_display_name() {
        let beacon = GatewayBeacon {
            instance_name: "instance".to_string(),
            display_name: Some("My Gateway".to_string()),
            host: Some("192.168.1.100".to_string()),
            lan_host: None,
            tailnet_dns: None,
            port: None,
            gateway_port: Some(9999),
            ssh_port: None,
        };
        let label = build_beacon_label(&beacon);
        assert_eq!(label, "My Gateway (192.168.1.100:9999)");
    }

    #[test]
    fn build_beacon_label_without_host() {
        let beacon = GatewayBeacon {
            instance_name: "instance".to_string(),
            display_name: None,
            host: None,
            lan_host: None,
            tailnet_dns: None,
            port: None,
            gateway_port: None,
            ssh_port: None,
        };
        let label = build_beacon_label(&beacon);
        assert_eq!(label, "instance (host unknown)");
    }

    #[test]
    fn ensure_ws_url_empty() {
        assert_eq!(ensure_ws_url(""), DEFAULT_GATEWAY_URL);
        assert_eq!(ensure_ws_url("  "), DEFAULT_GATEWAY_URL);
    }

    #[test]
    fn ensure_ws_url_valid() {
        assert_eq!(ensure_ws_url("ws://my-host:9999"), "ws://my-host:9999");
    }

    #[test]
    fn validate_ws_url_valid() {
        assert!(validate_ws_url("ws://localhost:19001").is_none());
        assert!(validate_ws_url("wss://secure.host").is_none());
    }

    #[test]
    fn validate_ws_url_invalid() {
        assert!(validate_ws_url("http://localhost").is_some());
        assert!(validate_ws_url("just-a-string").is_some());
    }

    #[test]
    fn ssh_tunnel_instructions() {
        let text = build_ssh_tunnel_instructions("myhost.local", None);
        assert!(text.contains("myhost.local"));
        assert!(text.contains("ssh -N -L"));
        assert!(!text.contains("-p "));
    }

    #[test]
    fn ssh_tunnel_instructions_with_port() {
        let text = build_ssh_tunnel_instructions("myhost.local", Some(2222));
        assert!(text.contains("-p 2222"));
    }

    #[test]
    fn apply_remote_gateway_config_sets_mode() {
        let cfg = OpenAcosmiConfig::default();
        let result = apply_remote_gateway_config(cfg, "ws://remote:9999", Some("my-token"));

        let gw = result.gateway.expect("gateway");
        assert_eq!(gw.mode, Some(GatewayMode::Remote));

        let remote = gw.remote.expect("remote");
        assert_eq!(remote.url.as_deref(), Some("ws://remote:9999"));
        assert_eq!(remote.token.as_deref(), Some("my-token"));
    }

    #[test]
    fn apply_remote_gateway_config_no_token() {
        let cfg = OpenAcosmiConfig::default();
        let result = apply_remote_gateway_config(cfg, "ws://host:19001", None);

        let remote = result
            .gateway
            .and_then(|g| g.remote)
            .expect("remote");
        assert!(remote.token.is_none());
    }
}
