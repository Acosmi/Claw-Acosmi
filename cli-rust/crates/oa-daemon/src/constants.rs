/// Service naming constants and profile resolution for Claw Acosmi daemon services.
///
/// Provides canonical service labels for launchd (macOS), systemd (Linux),
/// and Windows scheduled tasks, plus functions to resolve profile-specific
/// service names.
///
/// Source: `src/daemon/constants.ts`

/// Default macOS LaunchAgent label for the gateway service.
///
/// Source: `src/daemon/constants.ts` - `GATEWAY_LAUNCH_AGENT_LABEL`
pub const GATEWAY_LAUNCH_AGENT_LABEL: &str = "ai.openacosmi.gateway";

/// Default Linux systemd service name for the gateway service.
///
/// Source: `src/daemon/constants.ts` - `GATEWAY_SYSTEMD_SERVICE_NAME`
pub const GATEWAY_SYSTEMD_SERVICE_NAME: &str = "openacosmi-gateway";

/// Default Windows scheduled task name for the gateway service.
///
/// Source: `src/daemon/constants.ts` - `GATEWAY_WINDOWS_TASK_NAME`
pub const GATEWAY_WINDOWS_TASK_NAME: &str = "OpenAcosmi Gateway";

/// Service marker identifier for gateway services.
///
/// Source: `src/daemon/constants.ts` - `GATEWAY_SERVICE_MARKER`
pub const GATEWAY_SERVICE_MARKER: &str = "openacosmi";

/// Service kind identifier for gateway services.
///
/// Source: `src/daemon/constants.ts` - `GATEWAY_SERVICE_KIND`
pub const GATEWAY_SERVICE_KIND: &str = "gateway";

/// Default macOS LaunchAgent label for the node service.
///
/// Source: `src/daemon/constants.ts` - `NODE_LAUNCH_AGENT_LABEL`
pub const NODE_LAUNCH_AGENT_LABEL: &str = "ai.openacosmi.node";

/// Default Linux systemd service name for the node service.
///
/// Source: `src/daemon/constants.ts` - `NODE_SYSTEMD_SERVICE_NAME`
pub const NODE_SYSTEMD_SERVICE_NAME: &str = "openacosmi-node";

/// Default Windows scheduled task name for the node service.
///
/// Source: `src/daemon/constants.ts` - `NODE_WINDOWS_TASK_NAME`
pub const NODE_WINDOWS_TASK_NAME: &str = "OpenAcosmi Node";

/// Service marker identifier for node services.
///
/// Source: `src/daemon/constants.ts` - `NODE_SERVICE_MARKER`
pub const NODE_SERVICE_MARKER: &str = "openacosmi";

/// Service kind identifier for node services.
///
/// Source: `src/daemon/constants.ts` - `NODE_SERVICE_KIND`
pub const NODE_SERVICE_KIND: &str = "node";

/// Windows task script name for the node service.
///
/// Source: `src/daemon/constants.ts` - `NODE_WINDOWS_TASK_SCRIPT_NAME`
pub const NODE_WINDOWS_TASK_SCRIPT_NAME: &str = "node.cmd";

/// Legacy gateway LaunchAgent labels (currently empty).
///
/// Source: `src/daemon/constants.ts` - `LEGACY_GATEWAY_LAUNCH_AGENT_LABELS`
pub const LEGACY_GATEWAY_LAUNCH_AGENT_LABELS: &[&str] = &[];

/// Legacy gateway systemd service names (currently empty).
///
/// Source: `src/daemon/constants.ts` - `LEGACY_GATEWAY_SYSTEMD_SERVICE_NAMES`
pub const LEGACY_GATEWAY_SYSTEMD_SERVICE_NAMES: &[&str] = &[];

/// Legacy gateway Windows task names (currently empty).
///
/// Source: `src/daemon/constants.ts` - `LEGACY_GATEWAY_WINDOWS_TASK_NAMES`
pub const LEGACY_GATEWAY_WINDOWS_TASK_NAMES: &[&str] = &[];

/// Normalize a gateway profile name.
///
/// Returns `None` for empty, whitespace-only, or "default" (case-insensitive) profiles.
/// Returns the trimmed profile name otherwise.
///
/// Source: `src/daemon/constants.ts` - `normalizeGatewayProfile`
pub fn normalize_gateway_profile(profile: Option<&str>) -> Option<String> {
    let trimmed = profile?.trim();
    if trimmed.is_empty() || trimmed.eq_ignore_ascii_case("default") {
        return None;
    }
    Some(trimmed.to_string())
}

/// Resolve a hyphenated profile suffix for service names.
///
/// Returns an empty string for default/unset profiles, or `-{profile}` for custom ones.
///
/// Source: `src/daemon/constants.ts` - `resolveGatewayProfileSuffix`
pub fn resolve_gateway_profile_suffix(profile: Option<&str>) -> String {
    match normalize_gateway_profile(profile) {
        Some(normalized) => format!("-{normalized}"),
        None => String::new(),
    }
}

/// Resolve the macOS LaunchAgent label for a given gateway profile.
///
/// Returns the default label for default/unset profiles, or `ai.openacosmi.{profile}`
/// for custom profiles.
///
/// Source: `src/daemon/constants.ts` - `resolveGatewayLaunchAgentLabel`
pub fn resolve_gateway_launch_agent_label(profile: Option<&str>) -> String {
    match normalize_gateway_profile(profile) {
        Some(normalized) => format!("ai.openacosmi.{normalized}"),
        None => GATEWAY_LAUNCH_AGENT_LABEL.to_string(),
    }
}

/// Resolve legacy gateway LaunchAgent labels for a given profile.
///
/// Currently always returns an empty vector.
///
/// Source: `src/daemon/constants.ts` - `resolveLegacyGatewayLaunchAgentLabels`
pub fn resolve_legacy_gateway_launch_agent_labels(_profile: Option<&str>) -> Vec<String> {
    Vec::new()
}

/// Resolve the Linux systemd service name for a given gateway profile.
///
/// Returns the default service name for default/unset profiles, or
/// `openacosmi-gateway-{profile}` for custom profiles.
///
/// Source: `src/daemon/constants.ts` - `resolveGatewaySystemdServiceName`
pub fn resolve_gateway_systemd_service_name(profile: Option<&str>) -> String {
    let suffix = resolve_gateway_profile_suffix(profile);
    if suffix.is_empty() {
        return GATEWAY_SYSTEMD_SERVICE_NAME.to_string();
    }
    format!("openacosmi-gateway{suffix}")
}

/// Resolve the Windows scheduled task name for a given gateway profile.
///
/// Returns the default task name for default/unset profiles, or
/// `Claw Acosmi Gateway ({profile})` for custom profiles.
///
/// Source: `src/daemon/constants.ts` - `resolveGatewayWindowsTaskName`
pub fn resolve_gateway_windows_task_name(profile: Option<&str>) -> String {
    match normalize_gateway_profile(profile) {
        Some(normalized) => format!("OpenAcosmi Gateway ({normalized})"),
        None => GATEWAY_WINDOWS_TASK_NAME.to_string(),
    }
}

/// Format a human-readable gateway service description.
///
/// Includes the profile and/or version in parentheses when provided.
///
/// Source: `src/daemon/constants.ts` - `formatGatewayServiceDescription`
pub fn format_gateway_service_description(
    profile: Option<&str>,
    version: Option<&str>,
) -> String {
    let normalized_profile = normalize_gateway_profile(profile);
    let trimmed_version = version.map(str::trim).filter(|v| !v.is_empty());

    let mut parts: Vec<String> = Vec::new();
    if let Some(ref p) = normalized_profile {
        parts.push(format!("profile: {p}"));
    }
    if let Some(v) = trimmed_version {
        parts.push(format!("v{v}"));
    }

    if parts.is_empty() {
        "OpenAcosmi Gateway".to_string()
    } else {
        format!("OpenAcosmi Gateway ({})", parts.join(", "))
    }
}

/// Resolve the macOS LaunchAgent label for the node service.
///
/// Source: `src/daemon/constants.ts` - `resolveNodeLaunchAgentLabel`
pub fn resolve_node_launch_agent_label() -> &'static str {
    NODE_LAUNCH_AGENT_LABEL
}

/// Resolve the Linux systemd service name for the node service.
///
/// Source: `src/daemon/constants.ts` - `resolveNodeSystemdServiceName`
pub fn resolve_node_systemd_service_name() -> &'static str {
    NODE_SYSTEMD_SERVICE_NAME
}

/// Resolve the Windows scheduled task name for the node service.
///
/// Source: `src/daemon/constants.ts` - `resolveNodeWindowsTaskName`
pub fn resolve_node_windows_task_name() -> &'static str {
    NODE_WINDOWS_TASK_NAME
}

/// Format a human-readable node service description.
///
/// Includes the version in parentheses when provided.
///
/// Source: `src/daemon/constants.ts` - `formatNodeServiceDescription`
pub fn format_node_service_description(version: Option<&str>) -> String {
    let trimmed = version.map(str::trim).filter(|v| !v.is_empty());
    match trimmed {
        Some(v) => format!("OpenAcosmi Node Host (v{v})"),
        None => "OpenAcosmi Node Host".to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // --- normalize_gateway_profile ---

    #[test]
    fn normalize_returns_none_for_no_profile() {
        assert_eq!(normalize_gateway_profile(None), None);
    }

    #[test]
    fn normalize_returns_none_for_empty_string() {
        assert_eq!(normalize_gateway_profile(Some("")), None);
    }

    #[test]
    fn normalize_returns_none_for_whitespace() {
        assert_eq!(normalize_gateway_profile(Some("   ")), None);
    }

    #[test]
    fn normalize_returns_none_for_default_lowercase() {
        assert_eq!(normalize_gateway_profile(Some("default")), None);
    }

    #[test]
    fn normalize_returns_none_for_default_mixed_case() {
        assert_eq!(normalize_gateway_profile(Some("Default")), None);
        assert_eq!(normalize_gateway_profile(Some("DeFaUlT")), None);
    }

    #[test]
    fn normalize_returns_trimmed_profile() {
        assert_eq!(
            normalize_gateway_profile(Some("  staging  ")),
            Some("staging".to_string())
        );
    }

    #[test]
    fn normalize_returns_custom_profile() {
        assert_eq!(
            normalize_gateway_profile(Some("dev")),
            Some("dev".to_string())
        );
    }

    // --- resolve_gateway_launch_agent_label ---

    #[test]
    fn launch_agent_label_default_when_no_profile() {
        assert_eq!(
            resolve_gateway_launch_agent_label(None),
            GATEWAY_LAUNCH_AGENT_LABEL
        );
    }

    #[test]
    fn launch_agent_label_default_when_profile_is_default() {
        assert_eq!(
            resolve_gateway_launch_agent_label(Some("default")),
            GATEWAY_LAUNCH_AGENT_LABEL
        );
    }

    #[test]
    fn launch_agent_label_default_when_profile_is_default_case_insensitive() {
        assert_eq!(
            resolve_gateway_launch_agent_label(Some("Default")),
            GATEWAY_LAUNCH_AGENT_LABEL
        );
    }

    #[test]
    fn launch_agent_label_with_custom_profile() {
        assert_eq!(
            resolve_gateway_launch_agent_label(Some("dev")),
            "ai.openacosmi.dev"
        );
    }

    #[test]
    fn launch_agent_label_with_work_profile() {
        assert_eq!(
            resolve_gateway_launch_agent_label(Some("work")),
            "ai.openacosmi.work"
        );
    }

    #[test]
    fn launch_agent_label_trims_whitespace() {
        assert_eq!(
            resolve_gateway_launch_agent_label(Some("  staging  ")),
            "ai.openacosmi.staging"
        );
    }

    #[test]
    fn launch_agent_label_default_for_empty_string() {
        assert_eq!(
            resolve_gateway_launch_agent_label(Some("")),
            GATEWAY_LAUNCH_AGENT_LABEL
        );
    }

    #[test]
    fn launch_agent_label_default_for_whitespace_only() {
        assert_eq!(
            resolve_gateway_launch_agent_label(Some("   ")),
            GATEWAY_LAUNCH_AGENT_LABEL
        );
    }

    // --- resolve_gateway_systemd_service_name ---

    #[test]
    fn systemd_service_name_default_when_no_profile() {
        assert_eq!(
            resolve_gateway_systemd_service_name(None),
            GATEWAY_SYSTEMD_SERVICE_NAME
        );
    }

    #[test]
    fn systemd_service_name_default_when_profile_is_default() {
        assert_eq!(
            resolve_gateway_systemd_service_name(Some("default")),
            GATEWAY_SYSTEMD_SERVICE_NAME
        );
    }

    #[test]
    fn systemd_service_name_default_case_insensitive() {
        assert_eq!(
            resolve_gateway_systemd_service_name(Some("DEFAULT")),
            GATEWAY_SYSTEMD_SERVICE_NAME
        );
    }

    #[test]
    fn systemd_service_name_with_custom_profile() {
        assert_eq!(
            resolve_gateway_systemd_service_name(Some("dev")),
            "openacosmi-gateway-dev"
        );
    }

    #[test]
    fn systemd_service_name_with_production_profile() {
        assert_eq!(
            resolve_gateway_systemd_service_name(Some("production")),
            "openacosmi-gateway-production"
        );
    }

    #[test]
    fn systemd_service_name_trims_whitespace() {
        assert_eq!(
            resolve_gateway_systemd_service_name(Some("  test  ")),
            "openacosmi-gateway-test"
        );
    }

    #[test]
    fn systemd_service_name_default_for_empty_string() {
        assert_eq!(
            resolve_gateway_systemd_service_name(Some("")),
            GATEWAY_SYSTEMD_SERVICE_NAME
        );
    }

    #[test]
    fn systemd_service_name_default_for_whitespace_only() {
        assert_eq!(
            resolve_gateway_systemd_service_name(Some("   ")),
            GATEWAY_SYSTEMD_SERVICE_NAME
        );
    }

    // --- resolve_gateway_windows_task_name ---

    #[test]
    fn windows_task_name_default_when_no_profile() {
        assert_eq!(
            resolve_gateway_windows_task_name(None),
            GATEWAY_WINDOWS_TASK_NAME
        );
    }

    #[test]
    fn windows_task_name_default_when_profile_is_default() {
        assert_eq!(
            resolve_gateway_windows_task_name(Some("default")),
            GATEWAY_WINDOWS_TASK_NAME
        );
    }

    #[test]
    fn windows_task_name_default_case_insensitive() {
        assert_eq!(
            resolve_gateway_windows_task_name(Some("DeFaUlT")),
            GATEWAY_WINDOWS_TASK_NAME
        );
    }

    #[test]
    fn windows_task_name_with_custom_profile() {
        assert_eq!(
            resolve_gateway_windows_task_name(Some("dev")),
            "OpenAcosmi Gateway (dev)"
        );
    }

    #[test]
    fn windows_task_name_with_work_profile() {
        assert_eq!(
            resolve_gateway_windows_task_name(Some("work")),
            "OpenAcosmi Gateway (work)"
        );
    }

    #[test]
    fn windows_task_name_trims_whitespace() {
        assert_eq!(
            resolve_gateway_windows_task_name(Some("  ci  ")),
            "OpenAcosmi Gateway (ci)"
        );
    }

    #[test]
    fn windows_task_name_default_for_empty_string() {
        assert_eq!(
            resolve_gateway_windows_task_name(Some("")),
            GATEWAY_WINDOWS_TASK_NAME
        );
    }

    #[test]
    fn windows_task_name_default_for_whitespace_only() {
        assert_eq!(
            resolve_gateway_windows_task_name(Some("   ")),
            GATEWAY_WINDOWS_TASK_NAME
        );
    }

    // --- resolve_gateway_profile_suffix ---

    #[test]
    fn profile_suffix_empty_when_no_profile() {
        assert_eq!(resolve_gateway_profile_suffix(None), "");
    }

    #[test]
    fn profile_suffix_empty_for_default() {
        assert_eq!(resolve_gateway_profile_suffix(Some("default")), "");
        assert_eq!(resolve_gateway_profile_suffix(Some(" Default ")), "");
    }

    #[test]
    fn profile_suffix_hyphenated_for_custom() {
        assert_eq!(resolve_gateway_profile_suffix(Some("dev")), "-dev");
    }

    #[test]
    fn profile_suffix_trims_whitespace() {
        assert_eq!(
            resolve_gateway_profile_suffix(Some("  staging  ")),
            "-staging"
        );
    }

    // --- format_gateway_service_description ---

    #[test]
    fn description_default_when_no_params() {
        assert_eq!(
            format_gateway_service_description(None, None),
            "OpenAcosmi Gateway"
        );
    }

    #[test]
    fn description_includes_profile() {
        assert_eq!(
            format_gateway_service_description(Some("work"), None),
            "OpenAcosmi Gateway (profile: work)"
        );
    }

    #[test]
    fn description_includes_version() {
        assert_eq!(
            format_gateway_service_description(None, Some("2026.1.10")),
            "OpenAcosmi Gateway (v2026.1.10)"
        );
    }

    #[test]
    fn description_includes_profile_and_version() {
        assert_eq!(
            format_gateway_service_description(Some("dev"), Some("1.2.3")),
            "OpenAcosmi Gateway (profile: dev, v1.2.3)"
        );
    }

    // --- node service functions ---

    #[test]
    fn node_launch_agent_label() {
        assert_eq!(resolve_node_launch_agent_label(), NODE_LAUNCH_AGENT_LABEL);
    }

    #[test]
    fn node_systemd_service_name() {
        assert_eq!(
            resolve_node_systemd_service_name(),
            NODE_SYSTEMD_SERVICE_NAME
        );
    }

    #[test]
    fn node_windows_task_name() {
        assert_eq!(resolve_node_windows_task_name(), NODE_WINDOWS_TASK_NAME);
    }

    #[test]
    fn node_description_default() {
        assert_eq!(
            format_node_service_description(None),
            "OpenAcosmi Node Host"
        );
    }

    #[test]
    fn node_description_with_version() {
        assert_eq!(
            format_node_service_description(Some("1.0.0")),
            "OpenAcosmi Node Host (v1.0.0)"
        );
    }
}
