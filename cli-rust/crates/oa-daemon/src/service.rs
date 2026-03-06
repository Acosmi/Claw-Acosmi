/// Cross-platform daemon service interface for Claw Acosmi.
///
/// Provides a unified API for managing gateway services across macOS (launchd),
/// Linux (systemd), and Windows (scheduled tasks). Platform-specific code is
/// conditionally compiled and dispatched at runtime.
///
/// Source: `src/daemon/service.ts`

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

/// The status of a daemon service.
///
/// Source: `src/daemon/service-runtime.ts` - `GatewayServiceRuntime.status`
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum ServiceStatus {
    /// The service is currently running.
    Running,
    /// The service is stopped.
    Stopped,
    /// The service status could not be determined.
    #[default]
    Unknown,
}

impl std::fmt::Display for ServiceStatus {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Running => write!(f, "running"),
            Self::Stopped => write!(f, "stopped"),
            Self::Unknown => write!(f, "unknown"),
        }
    }
}

/// Runtime information about a gateway service.
///
/// Source: `src/daemon/service-runtime.ts` - `GatewayServiceRuntime`
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ServiceRuntime {
    /// Overall service status.
    pub status: ServiceStatus,
    /// Platform-specific state string (e.g., "running", "active").
    #[serde(skip_serializing_if = "Option::is_none")]
    pub state: Option<String>,
    /// Sub-state (systemd-specific, e.g., "running", "dead").
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sub_state: Option<String>,
    /// Process ID if the service is running.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pid: Option<u32>,
    /// Last exit status code.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_exit_status: Option<i32>,
    /// Last exit reason or code description.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_exit_reason: Option<String>,
    /// Diagnostic detail string.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
    /// Whether the agent is cached in launchd but the plist is missing.
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub cached_label: bool,
    /// Whether the unit/plist file is missing entirely.
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub missing_unit: bool,
}

/// The platform-specific service backend label.
///
/// Source: `src/daemon/service.ts` - `GatewayService.label`
pub fn service_backend_label() -> &'static str {
    #[cfg(target_os = "macos")]
    {
        "LaunchAgent"
    }
    #[cfg(target_os = "linux")]
    {
        "systemd"
    }
    #[cfg(target_os = "windows")]
    {
        "Scheduled Task"
    }
    #[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
    {
        "unsupported"
    }
}

/// Install the gateway service for the current platform.
///
/// Delegates to the platform-specific implementation (launchd on macOS,
/// systemd on Linux). Windows support is stubbed out.
///
/// Source: `src/daemon/service.ts` - `resolveGatewayService().install`
pub fn install_gateway_service(
    profile: Option<&str>,
    program_arguments: &[String],
    working_directory: Option<&str>,
    environment: &HashMap<String, String>,
    description: Option<&str>,
) -> anyhow::Result<()> {
    let env_fn = build_env_fn(profile);

    #[cfg(target_os = "macos")]
    {
        crate::launchd::install_launch_agent(
            &env_fn,
            program_arguments,
            working_directory,
            environment,
            description,
        )?;
        Ok(())
    }

    #[cfg(target_os = "linux")]
    {
        crate::systemd::install_systemd_service(
            &env_fn,
            program_arguments,
            working_directory,
            environment,
            description,
        )?;
        Ok(())
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        let _ = (program_arguments, working_directory, environment, description, env_fn);
        Err(anyhow::anyhow!(
            "Gateway service install not supported on this platform"
        ))
    }
}

/// Uninstall the gateway service for the current platform.
///
/// Source: `src/daemon/service.ts` - `resolveGatewayService().uninstall`
pub fn uninstall_gateway_service(profile: Option<&str>) -> anyhow::Result<()> {
    let env_fn = build_env_fn(profile);

    #[cfg(target_os = "macos")]
    {
        crate::launchd::uninstall_launch_agent(&env_fn)
    }

    #[cfg(target_os = "linux")]
    {
        crate::systemd::uninstall_systemd_service(&env_fn)
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        let _ = env_fn;
        Err(anyhow::anyhow!(
            "Gateway service uninstall not supported on this platform"
        ))
    }
}

/// Stop the gateway service for the current platform.
///
/// Source: `src/daemon/service.ts` - `resolveGatewayService().stop`
pub fn stop_gateway_service(profile: Option<&str>) -> anyhow::Result<()> {
    let env_fn = build_env_fn(profile);

    #[cfg(target_os = "macos")]
    {
        crate::launchd::stop_launch_agent(&env_fn)
    }

    #[cfg(target_os = "linux")]
    {
        crate::systemd::stop_systemd_service(&env_fn)
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        let _ = env_fn;
        Err(anyhow::anyhow!(
            "Gateway service stop not supported on this platform"
        ))
    }
}

/// Restart the gateway service for the current platform.
///
/// Source: `src/daemon/service.ts` - `resolveGatewayService().restart`
pub fn restart_gateway_service(profile: Option<&str>) -> anyhow::Result<()> {
    let env_fn = build_env_fn(profile);

    #[cfg(target_os = "macos")]
    {
        crate::launchd::restart_launch_agent(&env_fn)
    }

    #[cfg(target_os = "linux")]
    {
        crate::systemd::restart_systemd_service(&env_fn)
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        let _ = env_fn;
        Err(anyhow::anyhow!(
            "Gateway service restart not supported on this platform"
        ))
    }
}

/// Check whether the gateway service is enabled/loaded on the current platform.
///
/// Source: `src/daemon/service.ts` - `resolveGatewayService().isLoaded`
pub fn is_gateway_service_enabled(profile: Option<&str>) -> anyhow::Result<bool> {
    let env_fn = build_env_fn(profile);

    #[cfg(target_os = "macos")]
    {
        crate::launchd::is_launch_agent_loaded(&env_fn)
    }

    #[cfg(target_os = "linux")]
    {
        crate::systemd::is_systemd_service_enabled(&env_fn)
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        let _ = env_fn;
        Err(anyhow::anyhow!(
            "Gateway service status not supported on this platform"
        ))
    }
}

/// Read the runtime status of the gateway service on the current platform.
///
/// Returns `None` if the platform is not supported.
///
/// Source: `src/daemon/service.ts` - `resolveGatewayService().readRuntime`
pub fn read_gateway_service_runtime(profile: Option<&str>) -> anyhow::Result<Option<ServiceRuntime>> {
    let env_fn = build_env_fn(profile);

    #[cfg(target_os = "macos")]
    {
        Ok(Some(crate::launchd::read_launch_agent_runtime(&env_fn)))
    }

    #[cfg(target_os = "linux")]
    {
        Ok(Some(crate::systemd::read_systemd_service_runtime(&env_fn)))
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        let _ = env_fn;
        Ok(None)
    }
}

/// Build an environment lookup function from a profile.
///
/// The returned closure reads real environment variables and overrides
/// `OPENACOSMI_PROFILE` if a profile is specified.
fn build_env_fn(profile: Option<&str>) -> impl Fn(&str) -> Option<String> {
    let profile_owned = profile.map(String::from);
    move |key: &str| -> Option<String> {
        if key == "OPENACOSMI_PROFILE" {
            if let Some(ref p) = profile_owned {
                return Some(p.clone());
            }
        }
        std::env::var(key).ok()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn service_status_display() {
        assert_eq!(ServiceStatus::Running.to_string(), "running");
        assert_eq!(ServiceStatus::Stopped.to_string(), "stopped");
        assert_eq!(ServiceStatus::Unknown.to_string(), "unknown");
    }

    #[test]
    fn service_runtime_default() {
        let rt = ServiceRuntime::default();
        assert_eq!(rt.status, ServiceStatus::Unknown);
        assert!(rt.state.is_none());
        assert!(rt.pid.is_none());
        assert!(!rt.cached_label);
        assert!(!rt.missing_unit);
    }

    #[test]
    fn service_runtime_serialization() {
        let rt = ServiceRuntime {
            status: ServiceStatus::Running,
            pid: Some(1234),
            state: Some("running".to_string()),
            ..Default::default()
        };
        let json = serde_json::to_string(&rt).expect("should serialize");
        assert!(json.contains("\"status\":\"running\""));
        assert!(json.contains("\"pid\":1234"));
        // Verify that None fields are omitted
        assert!(!json.contains("sub_state"));
        assert!(!json.contains("cached_label"));
    }

    #[test]
    fn build_env_fn_with_profile() {
        let env_fn = build_env_fn(Some("dev"));
        assert_eq!(env_fn("OPENACOSMI_PROFILE"), Some("dev".to_string()));
    }

    #[test]
    fn build_env_fn_without_profile() {
        let env_fn = build_env_fn(None);
        // Without setting env var, this should return whatever is in the env
        // (likely None in test)
        let result = env_fn("OPENACOSMI_PROFILE");
        // We can't assert the exact value since it depends on the test env,
        // but we can assert the function doesn't panic
        let _ = result;
    }

    #[test]
    fn service_backend_label_is_known() {
        let label = service_backend_label();
        assert!(
            ["LaunchAgent", "systemd", "Scheduled Task", "unsupported"].contains(&label),
            "unexpected label: {label}"
        );
    }
}
