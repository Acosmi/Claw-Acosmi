/// status --all command implementation.
///
/// Produces a comprehensive, pasteable debug report including configuration,
/// channels, agents, services, logs, and diagnosis.
///
/// Source: `src/commands/status-all.ts`, `src/commands/status-all/*.ts`

use anyhow::Result;

use oa_cli_shared::command_format::format_cli_command;
use oa_terminal::theme::Theme;

use crate::daemon::{get_daemon_status_summary, get_node_daemon_status_summary};
use crate::format::{
    format_duration, format_gateway_auth_used, format_time_ago, redact_secrets, shorten_text,
};
use crate::gateway_probe::resolve_gateway_probe_auth;
use crate::scan::{scan_status, StatusScanResult};

/// Execute the status --all command.
///
/// Source: `src/commands/status-all.ts` - `statusAllCommand`
pub async fn status_all_command(timeout_ms: Option<u64>) -> Result<()> {
    let scan = scan_status(false, timeout_ms, true).await?;
    let lines = build_status_all_lines(&scan).await;
    println!("{}", lines.join("\n"));
    Ok(())
}

/// Build the full status --all report lines.
///
/// Source: `src/commands/status-all/report-lines.ts` - `buildStatusAllReportLines`
async fn build_status_all_lines(scan: &StatusScanResult) -> Vec<String> {
    let mut lines: Vec<String> = Vec::new();

    // Heading.
    lines.push(Theme::heading("OpenAcosmi status --all"));
    lines.push(String::new());

    // Overview.
    lines.push(Theme::heading("Overview"));
    let daemon = get_daemon_status_summary();
    let node_daemon = get_node_daemon_status_summary();

    let gateway_target = if scan.remote_url_missing {
        format!("fallback {}", scan.gateway_connection.url)
    } else {
        scan.gateway_connection.url.clone()
    };
    let gateway_status = if scan.gateway_reachable {
        let latency = scan
            .gateway_probe
            .as_ref()
            .and_then(|p| p.connect_latency_ms);
        format!("reachable {}", format_duration(latency))
    } else {
        let error = scan
            .gateway_probe
            .as_ref()
            .and_then(|p| p.error.as_deref())
            .unwrap_or("unreachable");
        format!("unreachable ({error})")
    };

    let probe_auth = resolve_gateway_probe_auth(&scan.cfg);
    let auth_label = if scan.gateway_reachable {
        format!(" \u{00b7} auth {}", format_gateway_auth_used(Some(&probe_auth)))
    } else {
        String::new()
    };

    let self_line = scan
        .gateway_self
        .as_ref()
        .map(|s| {
            let parts: Vec<String> = [
                s.host.as_deref().map(String::from),
                s.ip.as_deref().map(|ip| format!("({ip})")),
                s.version.as_deref().map(|v| format!("app {v}")),
                s.platform.clone(),
            ]
            .into_iter()
            .flatten()
            .collect();
            parts.join(" ")
        })
        .filter(|s| !s.is_empty());

    let alive_threshold_ms: u64 = 10 * 60_000;
    let alive_agents = scan
        .agent_status
        .agents
        .iter()
        .filter(|a| {
            a.last_active_age_ms
                .is_some_and(|age| age <= alive_threshold_ms)
        })
        .count();

    let daemon_value = format_daemon_line(&daemon);
    let node_value = format_daemon_line(&node_daemon);

    let overview_rows: Vec<(&str, String)> = vec![
        ("OS", scan.os_summary_label.clone()),
        (
            "Gateway",
            format!(
                "{}{} \u{00b7} {} ({}) \u{00b7} {gateway_status}{auth_label}",
                scan.gateway_mode,
                if scan.remote_url_missing {
                    " (remote.url missing)"
                } else {
                    ""
                },
                gateway_target,
                scan.gateway_connection.url_source,
            ),
        ),
        (
            "Gateway self",
            self_line.unwrap_or_else(|| "unknown".to_string()),
        ),
        ("Gateway service", daemon_value),
        ("Node service", node_value),
        (
            "Agents",
            format!(
                "{} total \u{00b7} {} bootstrapping \u{00b7} {alive_agents} active \u{00b7} {} sessions",
                scan.agent_status.agents.len(),
                scan.agent_status.bootstrap_pending_count,
                scan.agent_status.total_sessions,
            ),
        ),
        (
            "Security",
            format!(
                "Run: {}",
                format_cli_command("openacosmi security audit --deep")
            ),
        ),
    ];

    for (item, value) in &overview_rows {
        lines.push(format!("  {item}: {value}"));
    }

    // Channels.
    lines.push(String::new());
    lines.push(Theme::heading("Channels"));
    if scan.channels.rows.is_empty() {
        lines.push(Theme::muted("  No channel plugins loaded (Rust migration)."));
    } else {
        for row in &scan.channels.rows {
            let state_label = match row.state.as_str() {
                "ok" => Theme::success("OK"),
                "warn" => Theme::warn("WARN"),
                "off" => Theme::muted("OFF"),
                _ => Theme::accent_dim("SETUP"),
            };
            let enabled_label = if row.enabled {
                Theme::success("ON")
            } else {
                Theme::muted("OFF")
            };
            lines.push(format!(
                "  {} \u{00b7} {enabled_label} \u{00b7} {state_label} \u{00b7} {}",
                row.label, row.detail
            ));
        }
    }

    // Agents.
    lines.push(String::new());
    lines.push(Theme::heading("Agents"));
    for agent in &scan.agent_status.agents {
        let name_suffix = agent
            .name
            .as_deref()
            .filter(|n| !n.trim().is_empty())
            .map_or(String::new(), |n| format!(" ({n})"));
        let bootstrap = match agent.bootstrap_pending {
            Some(true) => Theme::warn("PENDING"),
            Some(false) => Theme::success("OK"),
            None => "unknown".to_string(),
        };
        let active = agent
            .last_active_age_ms
            .map_or("unknown".to_string(), |ms| format_time_ago(ms));
        lines.push(format!(
            "  {}{name_suffix} \u{00b7} bootstrap {bootstrap} \u{00b7} {} sessions \u{00b7} active {active} \u{00b7} {}",
            agent.id, agent.sessions_count, agent.sessions_path,
        ));
    }

    // Diagnosis.
    lines.push(String::new());
    lines.push(Theme::heading("Diagnosis (read-only)"));
    lines.push(String::new());
    lines.push(Theme::muted("Gateway connection details:"));
    for line in redact_secrets(&scan.gateway_connection.message)
        .lines()
        .map(str::trim_end)
    {
        lines.push(format!("  {}", Theme::muted(line)));
    }

    if scan.remote_url_missing {
        lines.push(String::new());
        lines.push(format!(
            "{} {}",
            Theme::warn("!"),
            Theme::warn("Gateway remote mode misconfigured (gateway.remote.url missing)")
        ));
        lines.push(format!(
            "  {}",
            Theme::muted("Fix: set gateway.remote.url, or set gateway.mode=local.")
        ));
    }

    // Channel issues.
    if !scan.channel_issues.is_empty() {
        lines.push(String::new());
        lines.push(format!(
            "{} Channel issues ({})",
            Theme::warn("!"),
            scan.channel_issues.len()
        ));
        for issue in scan.channel_issues.iter().take(12) {
            lines.push(format!(
                "  - {}: {}",
                issue.channel,
                shorten_text(&issue.message, 90)
            ));
        }
        if scan.channel_issues.len() > 12 {
            lines.push(format!(
                "  {}",
                Theme::muted(&format!(
                    "\u{2026} +{} more",
                    scan.channel_issues.len() - 12
                ))
            ));
        }
    }

    lines.push(String::new());
    lines.push(Theme::muted(
        "Pasteable debug report. Auth tokens redacted.",
    ));
    lines.push("Troubleshooting: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/general/troubleshooting".to_string());
    lines.push(String::new());

    lines
}

/// Format a daemon status summary as a single-line string.
///
/// Source: `src/commands/status-all.ts` - daemon formatting
fn format_daemon_line(daemon: &crate::daemon::DaemonStatusSummary) -> String {
    if daemon.installed == Some(false) {
        return format!("{} not installed", daemon.label);
    }
    let installed_prefix = if daemon.installed == Some(true) {
        "installed \u{00b7} "
    } else {
        ""
    };
    let runtime = daemon
        .runtime_short
        .as_deref()
        .map_or(String::new(), |r| format!(" \u{00b7} {r}"));
    format!(
        "{} {installed_prefix}{}{}",
        daemon.label, daemon.loaded_text, runtime
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::daemon::DaemonStatusSummary;

    #[test]
    fn format_daemon_not_installed() {
        let d = DaemonStatusSummary {
            label: "LaunchAgent".to_string(),
            installed: Some(false),
            loaded_text: "not loaded".to_string(),
            runtime_short: None,
        };
        let line = format_daemon_line(&d);
        assert!(line.contains("not installed"));
    }

    #[test]
    fn format_daemon_installed_loaded() {
        let d = DaemonStatusSummary {
            label: "LaunchAgent".to_string(),
            installed: Some(true),
            loaded_text: "loaded".to_string(),
            runtime_short: Some("running (pid 42)".to_string()),
        };
        let line = format_daemon_line(&d);
        assert!(line.contains("installed"));
        assert!(line.contains("loaded"));
        assert!(line.contains("running (pid 42)"));
    }
}
