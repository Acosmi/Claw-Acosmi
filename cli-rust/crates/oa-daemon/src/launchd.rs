/// macOS launchd service management for Claw Acosmi gateway and node services.
///
/// Provides functions to build, install, uninstall, start, stop, and query
/// launchd launch agents using `launchctl`.
///
/// Source: `src/daemon/launchd.ts`, `src/daemon/launchd-plist.ts`

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::process::Command;

use crate::constants::{
    format_gateway_service_description, resolve_gateway_launch_agent_label,
    resolve_legacy_gateway_launch_agent_labels,
};
use crate::paths::{resolve_gateway_state_dir, resolve_home_dir};

// ---------------------------------------------------------------------------
// XML plist building
// ---------------------------------------------------------------------------

/// Escape a string for safe inclusion in a plist XML value.
///
/// Source: `src/daemon/launchd-plist.ts` - `plistEscape`
fn plist_escape(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&apos;")
}

/// Render the `EnvironmentVariables` dict section of a plist.
///
/// Source: `src/daemon/launchd-plist.ts` - `renderEnvDict`
fn render_env_dict(env: &HashMap<String, String>) -> String {
    let entries: Vec<(&String, &String)> = env
        .iter()
        .filter(|(_, v)| !v.trim().is_empty())
        .collect();
    if entries.is_empty() {
        return String::new();
    }
    let items: String = entries
        .iter()
        .map(|(key, value)| {
            format!(
                "\n    <key>{}</key>\n    <string>{}</string>",
                plist_escape(key),
                plist_escape(value.trim()),
            )
        })
        .collect::<Vec<_>>()
        .join("");
    format!("\n    <key>EnvironmentVariables</key>\n    <dict>{items}\n    </dict>")
}

/// Build a macOS LaunchAgent plist XML string.
///
/// Generates a complete XML plist with the specified label, program arguments,
/// working directory, log paths, and environment variables. The agent is
/// configured with `RunAtLoad` and `KeepAlive` set to true.
///
/// Source: `src/daemon/launchd-plist.ts` - `buildLaunchAgentPlist`
pub fn build_launch_agent_plist(
    label: &str,
    comment: Option<&str>,
    program_arguments: &[String],
    working_directory: Option<&str>,
    stdout_path: &str,
    stderr_path: &str,
    environment: &HashMap<String, String>,
) -> String {
    let args_xml: String = program_arguments
        .iter()
        .map(|arg| format!("\n      <string>{}</string>", plist_escape(arg)))
        .collect::<Vec<_>>()
        .join("");

    let working_dir_xml = match working_directory {
        Some(dir) if !dir.is_empty() => format!(
            "\n    <key>WorkingDirectory</key>\n    <string>{}</string>",
            plist_escape(dir)
        ),
        _ => String::new(),
    };

    let comment_xml = match comment.map(str::trim).filter(|c| !c.is_empty()) {
        Some(c) => format!(
            "\n    <key>Comment</key>\n    <string>{}</string>",
            plist_escape(c)
        ),
        None => String::new(),
    };

    let env_xml = render_env_dict(environment);

    format!(
        "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n\
         <!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n\
         <plist version=\"1.0\">\n\
         \x20 <dict>\n\
         \x20   <key>Label</key>\n\
         \x20   <string>{label_escaped}</string>\n\
         \x20   {comment_xml}\n\
         \x20   <key>RunAtLoad</key>\n\
         \x20   <true/>\n\
         \x20   <key>KeepAlive</key>\n\
         \x20   <true/>\n\
         \x20   <key>ProgramArguments</key>\n\
         \x20   <array>{args_xml}\n\
         \x20   </array>\n\
         \x20   {working_dir_xml}\n\
         \x20   <key>StandardOutPath</key>\n\
         \x20   <string>{stdout_escaped}</string>\n\
         \x20   <key>StandardErrorPath</key>\n\
         \x20   <string>{stderr_escaped}</string>{env_xml}\n\
         \x20 </dict>\n\
         </plist>\n",
        label_escaped = plist_escape(label),
        stdout_escaped = plist_escape(stdout_path),
        stderr_escaped = plist_escape(stderr_path),
    )
}

// ---------------------------------------------------------------------------
// launchctl execution
// ---------------------------------------------------------------------------

/// Result from executing a launchctl command.
struct LaunchctlResult {
    stdout: String,
    stderr: String,
    code: i32,
}

/// Execute a `launchctl` command and capture output.
///
/// Source: `src/daemon/launchd.ts` - `execLaunchctl`
fn exec_launchctl(args: &[&str]) -> LaunchctlResult {
    match Command::new("launchctl").args(args).output() {
        Ok(output) => {
            let code = output.status.code().unwrap_or(1);
            LaunchctlResult {
                stdout: String::from_utf8_lossy(&output.stdout).to_string(),
                stderr: String::from_utf8_lossy(&output.stderr).to_string(),
                code,
            }
        }
        Err(e) => LaunchctlResult {
            stdout: String::new(),
            stderr: e.to_string(),
            code: 1,
        },
    }
}

/// Resolve the GUI domain for the current user (e.g., `gui/501`).
///
/// Uses `id -u` to get the current user's UID on Unix systems.
///
/// Source: `src/daemon/launchd.ts` - `resolveGuiDomain`
fn resolve_gui_domain() -> String {
    let uid = Command::new("id")
        .arg("-u")
        .output()
        .ok()
        .and_then(|output| {
            String::from_utf8(output.stdout)
                .ok()
                .and_then(|s| s.trim().parse::<u32>().ok())
        })
        .unwrap_or(501);
    format!("gui/{uid}")
}

/// Information parsed from `launchctl print` output.
///
/// Source: `src/daemon/launchd.ts` - `LaunchctlPrintInfo`
#[derive(Debug, Default, PartialEq, Eq)]
pub struct LaunchctlPrintInfo {
    /// The agent state (e.g., "running", "waiting").
    pub state: Option<String>,
    /// The process ID if running.
    pub pid: Option<u32>,
    /// The last exit status code.
    pub last_exit_status: Option<i32>,
    /// The last exit reason string.
    pub last_exit_reason: Option<String>,
}

/// Parse key-value output from launchctl/systemctl using a separator.
///
/// Source: `src/daemon/runtime-parse.ts` - `parseKeyValueOutput`
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

/// Parse the output of `launchctl print` into structured info.
///
/// Source: `src/daemon/launchd.ts` - `parseLaunchctlPrint`
pub fn parse_launchctl_print(output: &str) -> LaunchctlPrintInfo {
    let entries = parse_key_value_output(output, "=");
    let mut info = LaunchctlPrintInfo::default();

    if let Some(state) = entries.get("state") {
        if !state.is_empty() {
            info.state = Some(state.clone());
        }
    }
    if let Some(pid_str) = entries.get("pid") {
        if let Ok(pid) = pid_str.parse::<u32>() {
            info.pid = Some(pid);
        }
    }
    if let Some(status_str) = entries.get("last exit status") {
        if let Ok(status) = status_str.parse::<i32>() {
            info.last_exit_status = Some(status);
        }
    }
    if let Some(reason) = entries.get("last exit reason") {
        if !reason.is_empty() {
            info.last_exit_reason = Some(reason.clone());
        }
    }

    info
}

// ---------------------------------------------------------------------------
// Label and path resolution
// ---------------------------------------------------------------------------

/// Resolve the LaunchAgent label from environment variables.
///
/// Prefers `OPENACOSMI_LAUNCHD_LABEL`, then falls back to profile-based label.
///
/// Source: `src/daemon/launchd.ts` - `resolveLaunchAgentLabel`
fn resolve_launch_agent_label<F>(env: &F) -> String
where
    F: Fn(&str) -> Option<String>,
{
    if let Some(label) = env("OPENACOSMI_LAUNCHD_LABEL") {
        let trimmed = label.trim().to_string();
        if !trimmed.is_empty() {
            return trimmed;
        }
    }
    resolve_gateway_launch_agent_label(env("OPENACOSMI_PROFILE").as_deref())
}

/// Resolve the plist file path for a specific label.
///
/// Returns `~/Library/LaunchAgents/{label}.plist`.
///
/// Source: `src/daemon/launchd.ts` - `resolveLaunchAgentPlistPathForLabel`
fn resolve_launch_agent_plist_path_for_label<F>(env: &F, label: &str) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    let home = resolve_home_dir(env)?;
    Ok(home
        .join("Library")
        .join("LaunchAgents")
        .join(format!("{label}.plist")))
}

/// Resolve the plist file path for the current environment.
///
/// Uses the resolved label from environment variables to determine the path.
///
/// Source: `src/daemon/launchd.ts` - `resolveLaunchAgentPlistPath`
pub fn resolve_launch_agent_plist_path<F>(env: &F) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    let label = resolve_launch_agent_label(env);
    resolve_launch_agent_plist_path_for_label(env, &label)
}

/// Compute the path for the LaunchAgent plist from a label.
///
/// This is a simpler variant that takes `home_dir` directly.
///
/// Source: `src/daemon/launchd.ts` - `resolveLaunchAgentPlistPathForLabel`
pub fn launch_agent_plist_path(label: &str, home_dir: &Path) -> PathBuf {
    home_dir
        .join("Library")
        .join("LaunchAgents")
        .join(format!("{label}.plist"))
}

/// Resolve log file paths for the gateway service.
///
/// Source: `src/daemon/launchd.ts` - `resolveGatewayLogPaths`
pub fn resolve_gateway_log_paths<F>(env: &F) -> anyhow::Result<(PathBuf, PathBuf, PathBuf)>
where
    F: Fn(&str) -> Option<String>,
{
    let state_dir = resolve_gateway_state_dir(|k| env(k))?;
    let log_dir = state_dir.join("logs");
    let prefix = env("OPENACOSMI_LOG_PREFIX")
        .map(|p| p.trim().to_string())
        .filter(|p| !p.is_empty())
        .unwrap_or_else(|| "gateway".to_string());
    let stdout_path = log_dir.join(format!("{prefix}.log"));
    let stderr_path = log_dir.join(format!("{prefix}.err.log"));
    Ok((log_dir, stdout_path, stderr_path))
}

// ---------------------------------------------------------------------------
// Public service operations
// ---------------------------------------------------------------------------

/// Check whether a LaunchAgent is loaded by querying `launchctl print`.
///
/// Source: `src/daemon/launchd.ts` - `isLaunchAgentLoaded`
pub fn is_launch_agent_loaded<F>(env: &F) -> anyhow::Result<bool>
where
    F: Fn(&str) -> Option<String>,
{
    let domain = resolve_gui_domain();
    let label = resolve_launch_agent_label(env);
    let res = exec_launchctl(&["print", &format!("{domain}/{label}")]);
    Ok(res.code == 0)
}

/// Check if the LaunchAgent appears in `launchctl list` output.
///
/// Source: `src/daemon/launchd.ts` - `isLaunchAgentListed`
pub fn is_launch_agent_listed<F>(env: &F) -> anyhow::Result<bool>
where
    F: Fn(&str) -> Option<String>,
{
    let label = resolve_launch_agent_label(env);
    let res = exec_launchctl(&["list"]);
    if res.code != 0 {
        return Ok(false);
    }
    Ok(res
        .stdout
        .lines()
        .any(|line| line.trim().split_whitespace().last() == Some(&label)))
}

/// Check if the plist file exists on disk.
///
/// Source: `src/daemon/launchd.ts` - `launchAgentPlistExists`
pub fn launch_agent_plist_exists<F>(env: &F) -> anyhow::Result<bool>
where
    F: Fn(&str) -> Option<String>,
{
    let plist_path = resolve_launch_agent_plist_path(env)?;
    Ok(plist_path.exists())
}

/// Install a LaunchAgent by writing the plist and bootstrapping it.
///
/// This function:
/// 1. Creates the log directory.
/// 2. Removes any legacy launch agents.
/// 3. Writes the plist file.
/// 4. Boots out any existing agent, enables it, bootstraps, and kickstarts it.
///
/// Source: `src/daemon/launchd.ts` - `installLaunchAgent`
pub fn install_launch_agent<F>(
    env: &F,
    program_arguments: &[String],
    working_directory: Option<&str>,
    environment: &HashMap<String, String>,
    description: Option<&str>,
) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    let (log_dir, stdout_path, stderr_path) = resolve_gateway_log_paths(env)?;
    std::fs::create_dir_all(&log_dir)?;

    let domain = resolve_gui_domain();
    let label = resolve_launch_agent_label(env);

    // Remove legacy agents
    for legacy_label in resolve_legacy_gateway_launch_agent_labels(env("OPENACOSMI_PROFILE").as_deref()) {
        if let Ok(legacy_path) = resolve_launch_agent_plist_path_for_label(env, &legacy_label) {
            let legacy_path_str = legacy_path.to_string_lossy().to_string();
            exec_launchctl(&["bootout", &domain, &legacy_path_str]);
            exec_launchctl(&["unload", &legacy_path_str]);
            let _ = std::fs::remove_file(&legacy_path);
        }
    }

    let plist_path = resolve_launch_agent_plist_path_for_label(env, &label)?;
    if let Some(parent) = plist_path.parent() {
        std::fs::create_dir_all(parent)?;
    }

    let service_description = match description {
        Some(d) => d.to_string(),
        None => format_gateway_service_description(
            env("OPENACOSMI_PROFILE").as_deref(),
            environment
                .get("OPENACOSMI_SERVICE_VERSION")
                .or_else(|| None)
                .map(String::as_str)
                .or_else(|| None),
        ),
    };

    let plist = build_launch_agent_plist(
        &label,
        Some(&service_description),
        program_arguments,
        working_directory,
        &stdout_path.to_string_lossy(),
        &stderr_path.to_string_lossy(),
        environment,
    );
    std::fs::write(&plist_path, &plist)?;

    let plist_path_str = plist_path.to_string_lossy().to_string();

    // Boot out existing, unload, enable, bootstrap, kickstart
    exec_launchctl(&["bootout", &domain, &plist_path_str]);
    exec_launchctl(&["unload", &plist_path_str]);
    exec_launchctl(&["enable", &format!("{domain}/{label}")]);

    let boot = exec_launchctl(&["bootstrap", &domain, &plist_path_str]);
    if boot.code != 0 {
        let detail = if boot.stderr.is_empty() {
            &boot.stdout
        } else {
            &boot.stderr
        };
        return Err(anyhow::anyhow!(
            "launchctl bootstrap failed: {}",
            detail.trim()
        ));
    }

    exec_launchctl(&["kickstart", "-k", &format!("{domain}/{label}")]);

    tracing::info!(
        plist_path = %plist_path.display(),
        stdout_path = %stdout_path.display(),
        "Installed LaunchAgent"
    );

    Ok(plist_path)
}

/// Uninstall a LaunchAgent by booting out and removing the plist file.
///
/// Moves the plist to `~/.Trash` if possible, otherwise leaves it in place.
///
/// Source: `src/daemon/launchd.ts` - `uninstallLaunchAgent`
pub fn uninstall_launch_agent<F>(env: &F) -> anyhow::Result<()>
where
    F: Fn(&str) -> Option<String>,
{
    let domain = resolve_gui_domain();
    let label = resolve_launch_agent_label(env);
    let plist_path = resolve_launch_agent_plist_path(env)?;
    let plist_path_str = plist_path.to_string_lossy().to_string();

    exec_launchctl(&["bootout", &domain, &plist_path_str]);
    exec_launchctl(&["unload", &plist_path_str]);

    if !plist_path.exists() {
        tracing::info!("LaunchAgent not found at {}", plist_path.display());
        return Ok(());
    }

    let home = resolve_home_dir(env)?;
    let trash_dir = home.join(".Trash");
    let dest = trash_dir.join(format!("{label}.plist"));

    if std::fs::create_dir_all(&trash_dir).is_ok() {
        if std::fs::rename(&plist_path, &dest).is_ok() {
            tracing::info!("Moved LaunchAgent to Trash: {}", dest.display());
        } else {
            tracing::warn!(
                "LaunchAgent remains at {} (could not move)",
                plist_path.display()
            );
        }
    } else {
        tracing::warn!(
            "LaunchAgent remains at {} (could not create Trash)",
            plist_path.display()
        );
    }

    Ok(())
}

/// Check if a launchctl error indicates the service is not loaded.
///
/// Source: `src/daemon/launchd.ts` - `isLaunchctlNotLoaded`
fn is_launchctl_not_loaded(res: &LaunchctlResult) -> bool {
    let detail = if res.stderr.is_empty() {
        &res.stdout
    } else {
        &res.stderr
    }
    .to_lowercase();
    detail.contains("no such process")
        || detail.contains("could not find service")
        || detail.contains("not found")
}

/// Stop a running LaunchAgent by booting out the service.
///
/// Source: `src/daemon/launchd.ts` - `stopLaunchAgent`
pub fn stop_launch_agent<F>(env: &F) -> anyhow::Result<()>
where
    F: Fn(&str) -> Option<String>,
{
    let domain = resolve_gui_domain();
    let label = resolve_launch_agent_label(env);
    let service_target = format!("{domain}/{label}");
    let res = exec_launchctl(&["bootout", &service_target]);
    if res.code != 0 && !is_launchctl_not_loaded(&res) {
        let detail = if res.stderr.is_empty() {
            &res.stdout
        } else {
            &res.stderr
        };
        return Err(anyhow::anyhow!(
            "launchctl bootout failed: {}",
            detail.trim()
        ));
    }
    tracing::info!("Stopped LaunchAgent: {service_target}");
    Ok(())
}

/// Restart a LaunchAgent using `launchctl kickstart -k`.
///
/// Source: `src/daemon/launchd.ts` - `restartLaunchAgent`
pub fn restart_launch_agent<F>(env: &F) -> anyhow::Result<()>
where
    F: Fn(&str) -> Option<String>,
{
    let domain = resolve_gui_domain();
    let label = resolve_launch_agent_label(env);
    let service_target = format!("{domain}/{label}");
    let res = exec_launchctl(&["kickstart", "-k", &service_target]);
    if res.code != 0 {
        let detail = if res.stderr.is_empty() {
            &res.stdout
        } else {
            &res.stderr
        };
        return Err(anyhow::anyhow!(
            "launchctl kickstart failed: {}",
            detail.trim()
        ));
    }
    tracing::info!("Restarted LaunchAgent: {service_target}");
    Ok(())
}

/// Read the runtime status of a LaunchAgent.
///
/// Queries `launchctl print` and returns structured runtime information.
///
/// Source: `src/daemon/launchd.ts` - `readLaunchAgentRuntime`
pub fn read_launch_agent_runtime<F>(env: &F) -> crate::service::ServiceRuntime
where
    F: Fn(&str) -> Option<String>,
{
    let domain = resolve_gui_domain();
    let label = resolve_launch_agent_label(env);
    let res = exec_launchctl(&["print", &format!("{domain}/{label}")]);

    if res.code != 0 {
        let detail = if res.stderr.is_empty() {
            &res.stdout
        } else {
            &res.stderr
        }
        .trim()
        .to_string();
        return crate::service::ServiceRuntime {
            status: crate::service::ServiceStatus::Unknown,
            state: None,
            sub_state: None,
            pid: None,
            last_exit_status: None,
            last_exit_reason: None,
            detail: if detail.is_empty() { None } else { Some(detail) },
            cached_label: false,
            missing_unit: true,
        };
    }

    let combined = if res.stdout.is_empty() {
        &res.stderr
    } else {
        &res.stdout
    };
    let parsed = parse_launchctl_print(combined);
    let plist_exists = launch_agent_plist_exists(env).unwrap_or(false);
    let state_lower = parsed.state.as_deref().map(str::to_lowercase);

    let status = if state_lower.as_deref() == Some("running") || parsed.pid.is_some() {
        crate::service::ServiceStatus::Running
    } else if state_lower.is_some() {
        crate::service::ServiceStatus::Stopped
    } else {
        crate::service::ServiceStatus::Unknown
    };

    crate::service::ServiceRuntime {
        status,
        state: parsed.state,
        sub_state: None,
        pid: parsed.pid,
        last_exit_status: parsed.last_exit_status,
        last_exit_reason: parsed.last_exit_reason,
        detail: None,
        cached_label: !plist_exists,
        missing_unit: false,
    }
}

/// Repair the LaunchAgent bootstrap by re-bootstrapping and kickstarting.
///
/// Source: `src/daemon/launchd.ts` - `repairLaunchAgentBootstrap`
pub fn repair_launch_agent_bootstrap<F>(env: &F) -> anyhow::Result<()>
where
    F: Fn(&str) -> Option<String>,
{
    let domain = resolve_gui_domain();
    let label = resolve_launch_agent_label(env);
    let plist_path = resolve_launch_agent_plist_path(env)?;
    let plist_path_str = plist_path.to_string_lossy().to_string();

    let boot = exec_launchctl(&["bootstrap", &domain, &plist_path_str]);
    if boot.code != 0 {
        let detail = if boot.stderr.is_empty() {
            &boot.stdout
        } else {
            &boot.stderr
        };
        return Err(anyhow::anyhow!(
            "launchctl bootstrap failed: {}",
            detail.trim()
        ));
    }

    let kick = exec_launchctl(&["kickstart", "-k", &format!("{domain}/{label}")]);
    if kick.code != 0 {
        let detail = if kick.stderr.is_empty() {
            &kick.stdout
        } else {
            &kick.stderr
        };
        return Err(anyhow::anyhow!(
            "launchctl kickstart failed: {}",
            detail.trim()
        ));
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    // --- plist_escape ---

    #[test]
    fn plist_escape_basic_entities() {
        assert_eq!(plist_escape("a&b"), "a&amp;b");
        assert_eq!(plist_escape("a<b"), "a&lt;b");
        assert_eq!(plist_escape("a>b"), "a&gt;b");
        assert_eq!(plist_escape("a\"b"), "a&quot;b");
        assert_eq!(plist_escape("a'b"), "a&apos;b");
    }

    #[test]
    fn plist_escape_no_change() {
        assert_eq!(plist_escape("hello"), "hello");
    }

    // --- parse_launchctl_print ---

    #[test]
    fn parse_launchctl_print_full_output() {
        let output = "state = running\npid = 4242\nlast exit status = 1\nlast exit reason = exited";
        let info = parse_launchctl_print(output);
        assert_eq!(
            info,
            LaunchctlPrintInfo {
                state: Some("running".to_string()),
                pid: Some(4242),
                last_exit_status: Some(1),
                last_exit_reason: Some("exited".to_string()),
            }
        );
    }

    #[test]
    fn parse_launchctl_print_empty() {
        let info = parse_launchctl_print("");
        assert_eq!(info, LaunchctlPrintInfo::default());
    }

    #[test]
    fn parse_launchctl_print_partial() {
        let output = "state = waiting\n";
        let info = parse_launchctl_print(output);
        assert_eq!(info.state, Some("waiting".to_string()));
        assert_eq!(info.pid, None);
    }

    // --- build_launch_agent_plist ---

    #[test]
    fn plist_contains_label() {
        let plist = build_launch_agent_plist(
            "ai.openacosmi.gateway",
            None,
            &["node".to_string(), "server.js".to_string()],
            None,
            "/tmp/stdout.log",
            "/tmp/stderr.log",
            &HashMap::new(),
        );
        assert!(plist.contains("<string>ai.openacosmi.gateway</string>"));
    }

    #[test]
    fn plist_contains_program_arguments() {
        let plist = build_launch_agent_plist(
            "test",
            None,
            &["/usr/bin/openacosmi".to_string(), "gateway".to_string()],
            None,
            "/tmp/out.log",
            "/tmp/err.log",
            &HashMap::new(),
        );
        assert!(plist.contains("<string>/usr/bin/openacosmi</string>"));
        assert!(plist.contains("<string>gateway</string>"));
    }

    #[test]
    fn plist_contains_comment_when_provided() {
        let plist = build_launch_agent_plist(
            "test",
            Some("OpenAcosmi Gateway"),
            &["/usr/bin/openacosmi".to_string()],
            None,
            "/tmp/out.log",
            "/tmp/err.log",
            &HashMap::new(),
        );
        assert!(plist.contains("<key>Comment</key>"));
        assert!(plist.contains("<string>OpenAcosmi Gateway</string>"));
    }

    #[test]
    fn plist_no_comment_when_empty() {
        let plist = build_launch_agent_plist(
            "test",
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            "/tmp/out.log",
            "/tmp/err.log",
            &HashMap::new(),
        );
        assert!(!plist.contains("<key>Comment</key>"));
    }

    #[test]
    fn plist_contains_working_directory() {
        let plist = build_launch_agent_plist(
            "test",
            None,
            &["/usr/bin/openacosmi".to_string()],
            Some("/home/user"),
            "/tmp/out.log",
            "/tmp/err.log",
            &HashMap::new(),
        );
        assert!(plist.contains("<key>WorkingDirectory</key>"));
        assert!(plist.contains("<string>/home/user</string>"));
    }

    #[test]
    fn plist_contains_environment_variables() {
        let mut env = HashMap::new();
        env.insert("FOO".to_string(), "bar".to_string());
        let plist = build_launch_agent_plist(
            "test",
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            "/tmp/out.log",
            "/tmp/err.log",
            &env,
        );
        assert!(plist.contains("<key>EnvironmentVariables</key>"));
        assert!(plist.contains("<key>FOO</key>"));
        assert!(plist.contains("<string>bar</string>"));
    }

    #[test]
    fn plist_contains_log_paths() {
        let plist = build_launch_agent_plist(
            "test",
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            "/tmp/gateway.log",
            "/tmp/gateway.err.log",
            &HashMap::new(),
        );
        assert!(plist.contains("<key>StandardOutPath</key>"));
        assert!(plist.contains("<string>/tmp/gateway.log</string>"));
        assert!(plist.contains("<key>StandardErrorPath</key>"));
        assert!(plist.contains("<string>/tmp/gateway.err.log</string>"));
    }

    #[test]
    fn plist_escapes_special_characters_in_args() {
        let plist = build_launch_agent_plist(
            "test",
            None,
            &["--name=foo&bar".to_string()],
            None,
            "/tmp/out.log",
            "/tmp/err.log",
            &HashMap::new(),
        );
        assert!(plist.contains("<string>--name=foo&amp;bar</string>"));
    }

    #[test]
    fn plist_has_run_at_load_and_keep_alive() {
        let plist = build_launch_agent_plist(
            "test",
            None,
            &["/usr/bin/openacosmi".to_string()],
            None,
            "/tmp/out.log",
            "/tmp/err.log",
            &HashMap::new(),
        );
        assert!(plist.contains("<key>RunAtLoad</key>"));
        assert!(plist.contains("<true/>"));
        assert!(plist.contains("<key>KeepAlive</key>"));
    }

    #[test]
    fn plist_is_valid_xml_structure() {
        let plist = build_launch_agent_plist(
            "ai.openacosmi.gateway",
            Some("Test Service"),
            &["/usr/bin/openacosmi".to_string(), "gateway".to_string(), "start".to_string()],
            Some("/tmp"),
            "/tmp/out.log",
            "/tmp/err.log",
            &HashMap::new(),
        );
        assert!(plist.starts_with("<?xml version=\"1.0\" encoding=\"UTF-8\"?>"));
        assert!(plist.contains("<!DOCTYPE plist"));
        assert!(plist.contains("<plist version=\"1.0\">"));
        assert!(plist.contains("</plist>"));
    }

    // --- launch_agent_plist_path ---

    #[test]
    fn plist_path_default_label() {
        let home = PathBuf::from("/Users/test");
        let path = launch_agent_plist_path(crate::constants::GATEWAY_LAUNCH_AGENT_LABEL, &home);
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/ai.openacosmi.gateway.plist")
        );
    }

    #[test]
    fn plist_path_custom_label() {
        let home = PathBuf::from("/Users/test");
        let path = launch_agent_plist_path("com.custom.label", &home);
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/com.custom.label.plist")
        );
    }

    // --- resolve_launch_agent_plist_path ---

    #[test]
    fn resolve_plist_path_default_profile() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                "OPENACOSMI_PROFILE" => Some("default".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/ai.openacosmi.gateway.plist")
        );
    }

    #[test]
    fn resolve_plist_path_unset_profile() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/ai.openacosmi.gateway.plist")
        );
    }

    #[test]
    fn resolve_plist_path_custom_profile() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                "OPENACOSMI_PROFILE" => Some("jbphoenix".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/ai.openacosmi.jbphoenix.plist")
        );
    }

    #[test]
    fn resolve_plist_path_prefers_launchd_label() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                "OPENACOSMI_PROFILE" => Some("jbphoenix".to_string()),
                "OPENACOSMI_LAUNCHD_LABEL" => Some("com.custom.label".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/com.custom.label.plist")
        );
    }

    #[test]
    fn resolve_plist_path_trims_launchd_label() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                "OPENACOSMI_LAUNCHD_LABEL" => Some("  com.custom.label  ".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/com.custom.label.plist")
        );
    }

    #[test]
    fn resolve_plist_path_ignores_empty_launchd_label() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                "OPENACOSMI_PROFILE" => Some("myprofile".to_string()),
                "OPENACOSMI_LAUNCHD_LABEL" => Some("   ".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/ai.openacosmi.myprofile.plist")
        );
    }

    #[test]
    fn resolve_plist_path_case_insensitive_default() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                "OPENACOSMI_PROFILE" => Some("Default".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/ai.openacosmi.gateway.plist")
        );
    }

    #[test]
    fn resolve_plist_path_trims_profile_whitespace() {
        let env_fn = |key: &str| -> Option<String> {
            match key {
                "HOME" => Some("/Users/test".to_string()),
                "OPENACOSMI_PROFILE" => Some("  myprofile  ".to_string()),
                _ => None,
            }
        };
        let path = resolve_launch_agent_plist_path(&env_fn).expect("should resolve");
        assert_eq!(
            path,
            PathBuf::from("/Users/test/Library/LaunchAgents/ai.openacosmi.myprofile.plist")
        );
    }

    // --- parse_key_value_output ---

    #[test]
    fn parse_kv_basic() {
        let output = "key1=value1\nkey2=value2";
        let result = parse_key_value_output(output, "=");
        assert_eq!(result.get("key1"), Some(&"value1".to_string()));
        assert_eq!(result.get("key2"), Some(&"value2".to_string()));
    }

    #[test]
    fn parse_kv_with_spaces() {
        let output = "state = running\npid = 1234";
        let result = parse_key_value_output(output, "=");
        assert_eq!(result.get("state"), Some(&"running".to_string()));
        assert_eq!(result.get("pid"), Some(&"1234".to_string()));
    }

    #[test]
    fn parse_kv_skips_empty_lines() {
        let output = "key1=val1\n\nkey2=val2";
        let result = parse_key_value_output(output, "=");
        assert_eq!(result.len(), 2);
    }

    #[test]
    fn parse_kv_lowercases_keys() {
        let output = "State=running\nPID=123";
        let result = parse_key_value_output(output, "=");
        assert_eq!(result.get("state"), Some(&"running".to_string()));
        assert_eq!(result.get("pid"), Some(&"123".to_string()));
    }
}
