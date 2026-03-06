/// Linux systemd user service management for Claw Acosmi gateway and node services.
///
/// Provides functions to build systemd unit files, and install, uninstall,
/// start, stop, and query systemd user services using `systemctl --user`.
///
/// Source: `src/daemon/systemd.ts`, `src/daemon/systemd-unit.ts`

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::process::Command;

use crate::constants::{
    format_gateway_service_description, resolve_gateway_systemd_service_name,
    LEGACY_GATEWAY_SYSTEMD_SERVICE_NAMES,
};
use crate::paths::resolve_home_dir;

// ---------------------------------------------------------------------------
// Systemd unit file building
// ---------------------------------------------------------------------------

/// Escape a single argument for systemd `ExecStart=` lines.
///
/// Wraps the value in double quotes and escapes backslashes/quotes
/// if the value contains whitespace, quotes, or backslashes.
///
/// Source: `src/daemon/systemd-unit.ts` - `systemdEscapeArg`
fn systemd_escape_arg(value: &str) -> String {
    if !value.contains(|c: char| c.is_whitespace() || c == '"' || c == '\\') {
        return value.to_string();
    }
    let escaped = value.replace('\\', "\\\\").replace('"', "\\\"");
    format!("\"{escaped}\"")
}

/// Render `Environment=` lines for a systemd unit file.
///
/// Source: `src/daemon/systemd-unit.ts` - `renderEnvLines`
fn render_env_lines(env: &HashMap<String, String>) -> Vec<String> {
    let mut lines = Vec::new();
    for (key, value) in env {
        let trimmed = value.trim();
        if trimmed.is_empty() {
            continue;
        }
        lines.push(format!(
            "Environment={}",
            systemd_escape_arg(&format!("{key}={trimmed}"))
        ));
    }
    lines
}

/// Build a systemd user service unit file.
///
/// Generates a complete `[Unit]`, `[Service]`, `[Install]` unit file with
/// the specified description, exec command, working directory, and environment.
/// The service is configured with `Restart=always`, `RestartSec=5`, and
/// `KillMode=process`.
///
/// Source: `src/daemon/systemd-unit.ts` - `buildSystemdUnit`
pub fn build_systemd_unit(
    description: Option<&str>,
    program_arguments: &[String],
    working_directory: Option<&str>,
    environment: &HashMap<String, String>,
) -> String {
    let exec_start = program_arguments
        .iter()
        .map(|a| systemd_escape_arg(a))
        .collect::<Vec<_>>()
        .join(" ");
    let description_line = format!(
        "Description={}",
        description
            .map(str::trim)
            .filter(|d| !d.is_empty())
            .unwrap_or("OpenAcosmi Gateway")
    );

    let mut lines = vec![
        "[Unit]".to_string(),
        description_line,
        "After=network-online.target".to_string(),
        "Wants=network-online.target".to_string(),
        String::new(),
        "[Service]".to_string(),
        format!("ExecStart={exec_start}"),
        "Restart=always".to_string(),
        "RestartSec=5".to_string(),
        "KillMode=process".to_string(),
    ];

    if let Some(dir) = working_directory.filter(|d| !d.is_empty()) {
        lines.push(format!("WorkingDirectory={}", systemd_escape_arg(dir)));
    }

    lines.extend(render_env_lines(environment));

    lines.push(String::new());
    lines.push("[Install]".to_string());
    lines.push("WantedBy=default.target".to_string());
    lines.push(String::new());

    lines.join("\n")
}

/// Parse a systemd `ExecStart=` value into individual arguments.
///
/// Handles double-quoted strings and backslash escapes.
///
/// Source: `src/daemon/systemd-unit.ts` - `parseSystemdExecStart`
pub fn parse_systemd_exec_start(value: &str) -> Vec<String> {
    let mut args = Vec::new();
    let mut current = String::new();
    let mut in_quotes = false;
    let mut escape_next = false;

    for ch in value.chars() {
        if escape_next {
            current.push(ch);
            escape_next = false;
            continue;
        }
        if ch == '\\' {
            escape_next = true;
            continue;
        }
        if ch == '"' {
            in_quotes = !in_quotes;
            continue;
        }
        if !in_quotes && ch.is_whitespace() {
            if !current.is_empty() {
                args.push(std::mem::take(&mut current));
            }
            continue;
        }
        current.push(ch);
    }
    if !current.is_empty() {
        args.push(current);
    }
    args
}

/// Parse a systemd `Environment=` value into a key-value pair.
///
/// Handles quoted and unquoted `KEY=VALUE` assignments.
///
/// Source: `src/daemon/systemd-unit.ts` - `parseSystemdEnvAssignment`
pub fn parse_systemd_env_assignment(raw: &str) -> Option<(String, String)> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return None;
    }

    // Unquote if wrapped in double quotes
    let unquoted = if trimmed.starts_with('"') && trimmed.ends_with('"') && trimmed.len() >= 2 {
        let inner = &trimmed[1..trimmed.len() - 1];
        let mut out = String::new();
        let mut escape_next = false;
        for ch in inner.chars() {
            if escape_next {
                out.push(ch);
                escape_next = false;
                continue;
            }
            if ch == '\\' {
                escape_next = true;
                continue;
            }
            out.push(ch);
        }
        out
    } else {
        trimmed.to_string()
    };

    let eq_pos = unquoted.find('=')?;
    if eq_pos == 0 {
        return None;
    }
    let key = unquoted[..eq_pos].trim().to_string();
    if key.is_empty() {
        return None;
    }
    let value = unquoted[eq_pos + 1..].to_string();
    Some((key, value))
}

// ---------------------------------------------------------------------------
// systemctl execution
// ---------------------------------------------------------------------------

/// Result from executing a systemctl command.
struct SystemctlResult {
    stdout: String,
    stderr: String,
    code: i32,
}

/// Execute a `systemctl` command and capture output.
///
/// Source: `src/daemon/systemd.ts` - `execSystemctl`
fn exec_systemctl(args: &[&str]) -> SystemctlResult {
    match Command::new("systemctl").args(args).output() {
        Ok(output) => {
            let code = output.status.code().unwrap_or(1);
            SystemctlResult {
                stdout: String::from_utf8_lossy(&output.stdout).to_string(),
                stderr: String::from_utf8_lossy(&output.stderr).to_string(),
                code,
            }
        }
        Err(e) => SystemctlResult {
            stdout: String::new(),
            stderr: e.to_string(),
            code: 1,
        },
    }
}

// ---------------------------------------------------------------------------
// Service name and path resolution
// ---------------------------------------------------------------------------

/// Resolve the systemd service name from environment variables.
///
/// Prefers `OPENACOSMI_SYSTEMD_UNIT`, then falls back to profile-based name.
///
/// Source: `src/daemon/systemd.ts` - `resolveSystemdServiceName`
fn resolve_systemd_service_name<F>(env: &F) -> String
where
    F: Fn(&str) -> Option<String>,
{
    if let Some(unit) = env("OPENACOSMI_SYSTEMD_UNIT") {
        let trimmed = unit.trim().to_string();
        if !trimmed.is_empty() {
            return if let Some(stripped) = trimmed.strip_suffix(".service") {
                stripped.to_string()
            } else {
                trimmed
            };
        }
    }
    resolve_gateway_systemd_service_name(env("OPENACOSMI_PROFILE").as_deref())
}

/// Resolve the systemd unit file path for a specific service name.
///
/// Returns `~/.config/systemd/user/{name}.service`.
///
/// Source: `src/daemon/systemd.ts` - `resolveSystemdUnitPathForName`
fn resolve_systemd_unit_path_for_name<F>(env: &F, name: &str) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    let home = resolve_home_dir(env)?;
    Ok(home
        .join(".config")
        .join("systemd")
        .join("user")
        .join(format!("{name}.service")))
}

/// Resolve the systemd unit file path for the current environment.
///
/// Source: `src/daemon/systemd.ts` - `resolveSystemdUnitPath`
fn resolve_systemd_unit_path<F>(env: &F) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    let name = resolve_systemd_service_name(env);
    resolve_systemd_unit_path_for_name(env, &name)
}

/// Compute the systemd unit file path from a service name and home directory.
///
/// This is a simpler variant that takes `home_dir` directly.
///
/// Source: `src/daemon/systemd.ts` - `resolveSystemdUnitPathForName`
pub fn systemd_unit_path(name: &str, home_dir: &Path) -> PathBuf {
    home_dir
        .join(".config")
        .join("systemd")
        .join("user")
        .join(format!("{name}.service"))
}

/// Resolve the systemd user unit path for the current environment.
///
/// Public convenience wrapper.
///
/// Source: `src/daemon/systemd.ts` - `resolveSystemdUserUnitPath`
pub fn resolve_systemd_user_unit_path<F>(env: &F) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    resolve_systemd_unit_path(env)
}

// ---------------------------------------------------------------------------
// systemd availability check
// ---------------------------------------------------------------------------

/// Check whether systemd user services are available.
///
/// Source: `src/daemon/systemd.ts` - `isSystemdUserServiceAvailable`
pub fn is_systemd_user_service_available() -> bool {
    let res = exec_systemctl(&["--user", "status"]);
    if res.code == 0 {
        return true;
    }
    let detail = format!("{} {}", res.stderr, res.stdout).to_lowercase();
    if detail.is_empty() {
        return false;
    }
    // Any of these patterns means systemd is not usable
    !(detail.contains("not found")
        || detail.contains("failed to connect")
        || detail.contains("not been booted")
        || detail.contains("no such file or directory")
        || detail.contains("not supported"))
}

/// Assert that systemd user services are available, returning an error if not.
///
/// Source: `src/daemon/systemd.ts` - `assertSystemdAvailable`
fn assert_systemd_available() -> anyhow::Result<()> {
    let res = exec_systemctl(&["--user", "status"]);
    if res.code == 0 {
        return Ok(());
    }
    let detail = if res.stderr.is_empty() {
        &res.stdout
    } else {
        &res.stderr
    };
    if detail.to_lowercase().contains("not found") {
        return Err(anyhow::anyhow!(
            "systemctl not available; systemd user services are required on Linux."
        ));
    }
    Err(anyhow::anyhow!(
        "systemctl --user unavailable: {}",
        if detail.is_empty() {
            "unknown error"
        } else {
            detail.trim()
        }
    ))
}

// ---------------------------------------------------------------------------
// Parsed systemd service info
// ---------------------------------------------------------------------------

/// Information parsed from `systemctl show` output.
///
/// Source: `src/daemon/systemd.ts` - `SystemdServiceInfo`
#[derive(Debug, Default, PartialEq, Eq)]
pub struct SystemdServiceInfo {
    /// The active state (e.g., "active", "inactive").
    pub active_state: Option<String>,
    /// The sub state (e.g., "running", "dead").
    pub sub_state: Option<String>,
    /// The main process ID.
    pub main_pid: Option<u32>,
    /// The main process exit status.
    pub exec_main_status: Option<i32>,
    /// The main process exit code description.
    pub exec_main_code: Option<String>,
}

/// Parse key-value output from systemctl using `=` separator.
///
/// Source: `src/daemon/systemd.ts` - relies on `parseKeyValueOutput`
fn parse_key_value_output(output: &str, separator: &str) -> HashMap<String, String> {
    let mut entries = HashMap::new();
    for line in output.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        if let Some(idx) = trimmed.find(separator) {
            if idx == 0 {
                continue;
            }
            let key = trimmed[..idx].trim().to_lowercase();
            if key.is_empty() {
                continue;
            }
            let value = trimmed[idx + separator.len()..].trim().to_string();
            entries.insert(key, value);
        }
    }
    entries
}

/// Parse the output of `systemctl show` into structured info.
///
/// Source: `src/daemon/systemd.ts` - `parseSystemdShow`
pub fn parse_systemd_show(output: &str) -> SystemdServiceInfo {
    let entries = parse_key_value_output(output, "=");
    let mut info = SystemdServiceInfo::default();

    if let Some(state) = entries.get("activestate") {
        if !state.is_empty() {
            info.active_state = Some(state.clone());
        }
    }
    if let Some(sub) = entries.get("substate") {
        if !sub.is_empty() {
            info.sub_state = Some(sub.clone());
        }
    }
    if let Some(pid_str) = entries.get("mainpid") {
        if let Ok(pid) = pid_str.parse::<u32>() {
            if pid > 0 {
                info.main_pid = Some(pid);
            }
        }
    }
    if let Some(status_str) = entries.get("execmainstatus") {
        if let Ok(status) = status_str.parse::<i32>() {
            info.exec_main_status = Some(status);
        }
    }
    if let Some(code) = entries.get("execmaincode") {
        if !code.is_empty() {
            info.exec_main_code = Some(code.clone());
        }
    }

    info
}

// ---------------------------------------------------------------------------
// Public service operations
// ---------------------------------------------------------------------------

/// Install a systemd user service by writing the unit file and enabling/starting it.
///
/// This function:
/// 1. Asserts systemd is available.
/// 2. Writes the unit file.
/// 3. Runs `daemon-reload`, `enable`, and `restart`.
///
/// Source: `src/daemon/systemd.ts` - `installSystemdService`
pub fn install_systemd_service<F>(
    env: &F,
    program_arguments: &[String],
    working_directory: Option<&str>,
    environment: &HashMap<String, String>,
    description: Option<&str>,
) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    assert_systemd_available()?;

    let unit_path = resolve_systemd_unit_path(env)?;
    if let Some(parent) = unit_path.parent() {
        std::fs::create_dir_all(parent)?;
    }

    let service_description = match description {
        Some(d) => d.to_string(),
        None => format_gateway_service_description(
            env("OPENACOSMI_PROFILE").as_deref(),
            environment
                .get("OPENACOSMI_SERVICE_VERSION")
                .map(String::as_str),
        ),
    };

    let unit = build_systemd_unit(
        Some(&service_description),
        program_arguments,
        working_directory,
        environment,
    );
    std::fs::write(&unit_path, &unit)?;

    let service_name = resolve_gateway_systemd_service_name(env("OPENACOSMI_PROFILE").as_deref());
    let unit_name = format!("{service_name}.service");

    let reload = exec_systemctl(&["--user", "daemon-reload"]);
    if reload.code != 0 {
        let detail = if reload.stderr.is_empty() {
            &reload.stdout
        } else {
            &reload.stderr
        };
        return Err(anyhow::anyhow!(
            "systemctl daemon-reload failed: {}",
            detail.trim()
        ));
    }

    let enable = exec_systemctl(&["--user", "enable", &unit_name]);
    if enable.code != 0 {
        let detail = if enable.stderr.is_empty() {
            &enable.stdout
        } else {
            &enable.stderr
        };
        return Err(anyhow::anyhow!(
            "systemctl enable failed: {}",
            detail.trim()
        ));
    }

    let restart = exec_systemctl(&["--user", "restart", &unit_name]);
    if restart.code != 0 {
        let detail = if restart.stderr.is_empty() {
            &restart.stdout
        } else {
            &restart.stderr
        };
        return Err(anyhow::anyhow!(
            "systemctl restart failed: {}",
            detail.trim()
        ));
    }

    tracing::info!(unit_path = %unit_path.display(), "Installed systemd service");

    Ok(unit_path)
}

/// Uninstall a systemd user service by disabling and removing the unit file.
///
/// Source: `src/daemon/systemd.ts` - `uninstallSystemdService`
pub fn uninstall_systemd_service<F>(env: &F) -> anyhow::Result<()>
where
    F: Fn(&str) -> Option<String>,
{
    assert_systemd_available()?;

    let service_name = resolve_gateway_systemd_service_name(env("OPENACOSMI_PROFILE").as_deref());
    let unit_name = format!("{service_name}.service");
    exec_systemctl(&["--user", "disable", "--now", &unit_name]);

    let unit_path = resolve_systemd_unit_path(env)?;
    match std::fs::remove_file(&unit_path) {
        Ok(()) => {
            tracing::info!(unit_path = %unit_path.display(), "Removed systemd service");
        }
        Err(_) => {
            tracing::info!("Systemd service not found at {}", unit_path.display());
        }
    }

    Ok(())
}

/// Stop a running systemd user service.
///
/// Source: `src/daemon/systemd.ts` - `stopSystemdService`
pub fn stop_systemd_service<F>(env: &F) -> anyhow::Result<()>
where
    F: Fn(&str) -> Option<String>,
{
    assert_systemd_available()?;

    let service_name = resolve_systemd_service_name(env);
    let unit_name = format!("{service_name}.service");
    let res = exec_systemctl(&["--user", "stop", &unit_name]);
    if res.code != 0 {
        let detail = if res.stderr.is_empty() {
            &res.stdout
        } else {
            &res.stderr
        };
        return Err(anyhow::anyhow!(
            "systemctl stop failed: {}",
            detail.trim()
        ));
    }
    tracing::info!("Stopped systemd service: {unit_name}");
    Ok(())
}

/// Restart a systemd user service.
///
/// Source: `src/daemon/systemd.ts` - `restartSystemdService`
pub fn restart_systemd_service<F>(env: &F) -> anyhow::Result<()>
where
    F: Fn(&str) -> Option<String>,
{
    assert_systemd_available()?;

    let service_name = resolve_systemd_service_name(env);
    let unit_name = format!("{service_name}.service");
    let res = exec_systemctl(&["--user", "restart", &unit_name]);
    if res.code != 0 {
        let detail = if res.stderr.is_empty() {
            &res.stdout
        } else {
            &res.stderr
        };
        return Err(anyhow::anyhow!(
            "systemctl restart failed: {}",
            detail.trim()
        ));
    }
    tracing::info!("Restarted systemd service: {unit_name}");
    Ok(())
}

/// Check whether a systemd user service is enabled.
///
/// Source: `src/daemon/systemd.ts` - `isSystemdServiceEnabled`
pub fn is_systemd_service_enabled<F>(env: &F) -> anyhow::Result<bool>
where
    F: Fn(&str) -> Option<String>,
{
    assert_systemd_available()?;

    let service_name = resolve_systemd_service_name(env);
    let unit_name = format!("{service_name}.service");
    let res = exec_systemctl(&["--user", "is-enabled", &unit_name]);
    Ok(res.code == 0)
}

/// Read the runtime status of a systemd user service.
///
/// Queries `systemctl --user show` and returns structured runtime information.
///
/// Source: `src/daemon/systemd.ts` - `readSystemdServiceRuntime`
pub fn read_systemd_service_runtime<F>(env: &F) -> crate::service::ServiceRuntime
where
    F: Fn(&str) -> Option<String>,
{
    if let Err(err) = assert_systemd_available() {
        return crate::service::ServiceRuntime {
            status: crate::service::ServiceStatus::Unknown,
            detail: Some(err.to_string()),
            ..Default::default()
        };
    }

    let service_name = resolve_systemd_service_name(env);
    let unit_name = format!("{service_name}.service");
    let res = exec_systemctl(&[
        "--user",
        "show",
        &unit_name,
        "--no-page",
        "--property",
        "ActiveState,SubState,MainPID,ExecMainStatus,ExecMainCode",
    ]);

    if res.code != 0 {
        let detail = if res.stderr.is_empty() {
            &res.stdout
        } else {
            &res.stderr
        }
        .trim()
        .to_string();
        let missing = detail.to_lowercase().contains("not found");
        return crate::service::ServiceRuntime {
            status: if missing {
                crate::service::ServiceStatus::Stopped
            } else {
                crate::service::ServiceStatus::Unknown
            },
            detail: if detail.is_empty() { None } else { Some(detail) },
            missing_unit: missing,
            ..Default::default()
        };
    }

    let parsed = parse_systemd_show(&res.stdout);
    let active_state = parsed.active_state.as_deref().map(str::to_lowercase);
    let status = if active_state.as_deref() == Some("active") {
        crate::service::ServiceStatus::Running
    } else if active_state.is_some() {
        crate::service::ServiceStatus::Stopped
    } else {
        crate::service::ServiceStatus::Unknown
    };

    crate::service::ServiceRuntime {
        status,
        state: parsed.active_state,
        sub_state: parsed.sub_state,
        pid: parsed.main_pid,
        last_exit_status: parsed.exec_main_status,
        last_exit_reason: parsed.exec_main_code,
        ..Default::default()
    }
}

/// Read the `ExecStart=` and environment from a systemd unit file.
///
/// Source: `src/daemon/systemd.ts` - `readSystemdServiceExecStart`
pub fn read_systemd_service_exec_start<F>(
    env: &F,
) -> anyhow::Result<
    Option<(
        Vec<String>,
        Option<String>,
        HashMap<String, String>,
        PathBuf,
    )>,
>
where
    F: Fn(&str) -> Option<String>,
{
    let unit_path = resolve_systemd_unit_path(env)?;
    let content = match std::fs::read_to_string(&unit_path) {
        Ok(c) => c,
        Err(_) => return Ok(None),
    };

    let mut exec_start = String::new();
    let mut working_directory = String::new();
    let mut environment: HashMap<String, String> = HashMap::new();

    for line in content.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        if let Some(rest) = trimmed.strip_prefix("ExecStart=") {
            exec_start = rest.trim().to_string();
        } else if let Some(rest) = trimmed.strip_prefix("WorkingDirectory=") {
            working_directory = rest.trim().to_string();
        } else if let Some(rest) = trimmed.strip_prefix("Environment=") {
            if let Some((key, value)) = parse_systemd_env_assignment(rest.trim()) {
                environment.insert(key, value);
            }
        }
    }

    if exec_start.is_empty() {
        return Ok(None);
    }

    let program_arguments = parse_systemd_exec_start(&exec_start);
    Ok(Some((
        program_arguments,
        if working_directory.is_empty() {
            None
        } else {
            Some(working_directory)
        },
        environment,
        unit_path,
    )))
}

/// Find legacy systemd units that should be cleaned up.
///
/// Source: `src/daemon/systemd.ts` - `findLegacySystemdUnits`
pub fn find_legacy_systemd_units<F>(env: &F) -> Vec<(String, PathBuf, bool, bool)>
where
    F: Fn(&str) -> Option<String>,
{
    let mut results = Vec::new();
    let systemctl_available = is_systemctl_available();

    for name in LEGACY_GATEWAY_SYSTEMD_SERVICE_NAMES {
        let unit_path = match resolve_systemd_unit_path_for_name(env, name) {
            Ok(p) => p,
            Err(_) => continue,
        };

        let exists = unit_path.exists();
        let mut enabled = false;
        if systemctl_available {
            let res = exec_systemctl(&["--user", "is-enabled", &format!("{name}.service")]);
            enabled = res.code == 0;
        }

        if exists || enabled {
            results.push((name.to_string(), unit_path, enabled, exists));
        }
    }

    results
}

/// Check if systemctl is available (can execute).
fn is_systemctl_available() -> bool {
    let res = exec_systemctl(&["--user", "status"]);
    if res.code == 0 {
        return true;
    }
    let detail = format!("{} {}", res.stderr, res.stdout).to_lowercase();
    !detail.contains("not found")
}

#[cfg(test)]
mod tests {
    use super::*;

    // --- build_systemd_unit ---

    #[test]
    fn unit_has_all_sections() {
        let unit = build_systemd_unit(
            Some("Test Service"),
            &["/usr/bin/openacosmi".to_string(), "gateway".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(unit.contains("[Unit]"));
        assert!(unit.contains("[Service]"));
        assert!(unit.contains("[Install]"));
    }

    #[test]
    fn unit_contains_description() {
        let unit = build_systemd_unit(
            Some("My Gateway"),
            &["/usr/bin/openacosmi".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(unit.contains("Description=My Gateway"));
    }

    #[test]
    fn unit_default_description() {
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(unit.contains("Description=OpenAcosmi Gateway"));
    }

    #[test]
    fn unit_contains_exec_start() {
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string(), "gateway".to_string(), "start".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(unit.contains("ExecStart=/usr/bin/openacosmi gateway start"));
    }

    #[test]
    fn unit_contains_working_directory() {
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string()],
            Some("/home/user"),
            &HashMap::new(),
        );
        assert!(unit.contains("WorkingDirectory=/home/user"));
    }

    #[test]
    fn unit_no_working_directory_when_none() {
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(!unit.contains("WorkingDirectory"));
    }

    #[test]
    fn unit_contains_environment_variables() {
        let mut env = HashMap::new();
        env.insert("FOO".to_string(), "bar".to_string());
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            &env,
        );
        assert!(unit.contains("Environment=FOO=bar"));
    }

    #[test]
    fn unit_contains_restart_config() {
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(unit.contains("Restart=always"));
        assert!(unit.contains("RestartSec=5"));
        assert!(unit.contains("KillMode=process"));
    }

    #[test]
    fn unit_contains_network_dependencies() {
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(unit.contains("After=network-online.target"));
        assert!(unit.contains("Wants=network-online.target"));
    }

    #[test]
    fn unit_contains_install_target() {
        let unit = build_systemd_unit(
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            &HashMap::new(),
        );
        assert!(unit.contains("WantedBy=default.target"));
    }

    // --- systemd_escape_arg ---

    #[test]
    fn escape_arg_no_change() {
        assert_eq!(systemd_escape_arg("hello"), "hello");
        assert_eq!(systemd_escape_arg("/usr/bin/foo"), "/usr/bin/foo");
    }

    #[test]
    fn escape_arg_with_spaces() {
        assert_eq!(systemd_escape_arg("hello world"), "\"hello world\"");
    }

    #[test]
    fn escape_arg_with_quotes() {
        assert_eq!(systemd_escape_arg("say\"hi"), "\"say\\\"hi\"");
    }

    // --- parse_systemd_exec_start ---

    #[test]
    fn parse_exec_start_simple() {
        let args = parse_systemd_exec_start("/usr/bin/openacosmi gateway start --foo bar");
        assert_eq!(
            args,
            vec!["/usr/bin/openacosmi", "gateway", "start", "--foo", "bar"]
        );
    }

    #[test]
    fn parse_exec_start_quoted() {
        let args = parse_systemd_exec_start("/usr/bin/openacosmi gateway start --name \"My Bot\"");
        assert_eq!(
            args,
            vec!["/usr/bin/openacosmi", "gateway", "start", "--name", "My Bot"]
        );
    }

    #[test]
    fn parse_exec_start_with_path() {
        let args =
            parse_systemd_exec_start("/usr/bin/openacosmi gateway start --path /tmp/openacosmi");
        assert_eq!(
            args,
            vec![
                "/usr/bin/openacosmi",
                "gateway",
                "start",
                "--path",
                "/tmp/openacosmi"
            ]
        );
    }

    // --- parse_systemd_env_assignment ---

    #[test]
    fn parse_env_simple() {
        let result = parse_systemd_env_assignment("FOO=bar");
        assert_eq!(result, Some(("FOO".to_string(), "bar".to_string())));
    }

    #[test]
    fn parse_env_quoted() {
        let result = parse_systemd_env_assignment("\"FOO=bar baz\"");
        assert_eq!(result, Some(("FOO".to_string(), "bar baz".to_string())));
    }

    #[test]
    fn parse_env_empty() {
        assert_eq!(parse_systemd_env_assignment(""), None);
        assert_eq!(parse_systemd_env_assignment("   "), None);
    }

    #[test]
    fn parse_env_no_equals() {
        assert_eq!(parse_systemd_env_assignment("FOOBAR"), None);
    }

    // --- parse_systemd_show ---

    #[test]
    fn parse_show_full() {
        let output = "ActiveState=active\nSubState=running\nMainPID=1234\nExecMainStatus=0\nExecMainCode=exited";
        let info = parse_systemd_show(output);
        assert_eq!(
            info,
            SystemdServiceInfo {
                active_state: Some("active".to_string()),
                sub_state: Some("running".to_string()),
                main_pid: Some(1234),
                exec_main_status: Some(0),
                exec_main_code: Some("exited".to_string()),
            }
        );
    }

    #[test]
    fn parse_show_empty() {
        let info = parse_systemd_show("");
        assert_eq!(info, SystemdServiceInfo::default());
    }

    #[test]
    fn parse_show_zero_pid_ignored() {
        let output = "MainPID=0";
        let info = parse_systemd_show(output);
        assert_eq!(info.main_pid, None);
    }

    // --- systemd_unit_path ---

    #[test]
    fn unit_path_default() {
        let home = std::path::PathBuf::from("/home/user");
        let path = systemd_unit_path("openacosmi-gateway", &home);
        assert_eq!(
            path,
            std::path::PathBuf::from(
                "/home/user/.config/systemd/user/openacosmi-gateway.service"
            )
        );
    }

    #[test]
    fn unit_path_custom_name() {
        let home = std::path::PathBuf::from("/home/user");
        let path = systemd_unit_path("openacosmi-gateway-dev", &home);
        assert_eq!(
            path,
            std::path::PathBuf::from(
                "/home/user/.config/systemd/user/openacosmi-gateway-dev.service"
            )
        );
    }
}
