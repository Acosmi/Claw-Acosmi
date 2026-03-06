/// Health check commands for Claw Acosmi CLI.
///
/// Provides the `health` command that queries the running gateway for health
/// status, displays channel probes, agent heartbeat intervals, session store
/// summaries, and supports `--json` output for machine consumption.
///
/// Source: `src/commands/health.ts`, `src/commands/health-format.ts`

mod format;
mod snapshot;
mod types;

use anyhow::Result;
use clap::Parser;

use oa_cli_shared::progress::with_progress;
use oa_config::io::load_config;
use oa_gateway_rpc::call::{CallGatewayOptions, build_gateway_connection_details, call_gateway};
use oa_infra::env::is_truthy_env_value;
use oa_terminal::theme::Theme;

use crate::format::{format_health_channel_lines, style_health_channel_line};
use crate::snapshot::{build_session_summary, resolve_agent_order};
use crate::types::HealthSummary;

/// CLI arguments for the `health` command.
///
/// Source: `src/commands/health.ts` - `healthCommand` opts
#[derive(Debug, Parser)]
pub struct HealthArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,

    /// Enable verbose output with gateway connection details and all agents.
    #[arg(long, short)]
    pub verbose: bool,

    /// Timeout in milliseconds for the gateway health call.
    #[arg(long)]
    pub timeout_ms: Option<u64>,
}

/// Execute the health command.
///
/// Queries the running gateway for a health summary, then displays channel
/// probe results, agent heartbeat intervals, and session store summaries.
///
/// Source: `src/commands/health.ts` - `healthCommand`
pub async fn execute(args: &HealthArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();

    let mut call_opts = CallGatewayOptions {
        method: "health".to_string(),
        config: Some(cfg.clone()),
        timeout_ms: args.timeout_ms,
        ..Default::default()
    };

    if args.verbose {
        call_opts.params = Some(serde_json::json!({ "probe": true }));
    }

    let summary: HealthSummary = if args.json {
        call_gateway(call_opts).await?
    } else {
        with_progress(
            "Checking gateway health\u{2026}",
            call_gateway(call_opts),
        )
        .await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&summary).unwrap_or_default()
        );
        return Ok(());
    }

    let debug_enabled = is_truthy_env_value(
        std::env::var("OPENACOSMI_DEBUG_HEALTH")
            .ok()
            .as_deref(),
    );

    if args.verbose {
        let details = build_gateway_connection_details(&cfg, None, None);
        println!("{}", Theme::info("Gateway connection:"));
        for line in details.message.lines() {
            println!("  {line}");
        }
    }

    let local_agents = resolve_agent_order(&cfg);
    let default_agent_id = if summary.default_agent_id.is_empty() {
        &local_agents.default_agent_id
    } else {
        &summary.default_agent_id
    };

    let agents = if summary.agents.is_empty() {
        &local_agents
            .ordered
            .iter()
            .map(|entry| {
                let store_path = oa_config::sessions::paths::resolve_store_path(
                    cfg.session.as_ref().and_then(|s| s.store.as_deref()),
                    Some(&entry.id),
                );
                types::AgentHealthSummary {
                    agent_id: entry.id.clone(),
                    name: entry.name.clone(),
                    is_default: entry.id == local_agents.default_agent_id,
                    heartbeat: types::HeartbeatSummary::default(),
                    sessions: build_session_summary(&store_path),
                }
            })
            .collect::<Vec<_>>()
    } else {
        &summary.agents
    };

    let display_agents: Vec<&types::AgentHealthSummary> = if args.verbose {
        agents.iter().collect()
    } else {
        agents
            .iter()
            .filter(|a| a.agent_id == *default_agent_id)
            .collect()
    };

    // Print channel lines
    let channel_lines = format_health_channel_lines(&summary, args.verbose);
    for line in &channel_lines {
        println!("{}", style_health_channel_line(line));
    }

    // Print debug info if enabled
    if debug_enabled {
        print_debug_info(&summary);
    }

    // Print agent labels
    if !agents.is_empty() {
        let labels: Vec<String> = agents
            .iter()
            .map(|a| {
                if a.is_default {
                    format!("{} (default)", a.agent_id)
                } else {
                    a.agent_id.clone()
                }
            })
            .collect();
        println!("{}", Theme::info(&format!("Agents: {}", labels.join(", "))));
    }

    // Print heartbeat intervals
    let heartbeat_parts: Vec<String> = display_agents
        .iter()
        .map(|agent| {
            let label = agent
                .heartbeat
                .every_ms
                .filter(|&ms| ms > 0)
                .map_or_else(|| "disabled".to_string(), format_duration_parts);
            format!("{label} ({})", agent.agent_id)
        })
        .collect();
    if !heartbeat_parts.is_empty() {
        println!(
            "{}",
            Theme::info(&format!(
                "Heartbeat interval: {}",
                heartbeat_parts.join(", ")
            ))
        );
    }

    // Print session store info
    if display_agents.is_empty() {
        println!(
            "{}",
            Theme::info(&format!(
                "Session store: {} ({} entries)",
                summary.sessions.path, summary.sessions.count
            ))
        );
        for r in &summary.sessions.recent {
            let age_label = r
                .updated_at
                .filter(|&ts| ts > 0)
                .map_or("no activity".to_string(), |ts| {
                    let now = now_ms();
                    let age_minutes = (now.saturating_sub(ts)) / 60_000;
                    format!("{age_minutes}m ago")
                });
            println!("- {} ({age_label})", r.key);
        }
    } else {
        for agent in &display_agents {
            println!(
                "{}",
                Theme::info(&format!(
                    "Session store ({}): {} ({} entries)",
                    agent.agent_id, agent.sessions.path, agent.sessions.count
                ))
            );
            for r in &agent.sessions.recent {
                let age_label = r
                    .updated_at
                    .filter(|&ts| ts > 0)
                    .map_or("no activity".to_string(), |ts| {
                        let now = now_ms();
                        let age_minutes = (now.saturating_sub(ts)) / 60_000;
                        format!("{age_minutes}m ago")
                    });
                println!("- {} ({age_label})", r.key);
            }
        }
    }

    Ok(())
}

/// Print debug health info to stdout.
///
/// Source: `src/commands/health.ts` - debug logging block
fn print_debug_info(summary: &HealthSummary) {
    println!("{}", Theme::info("[debug] gateway channel probes"));
    for (channel_id, channel_summary) in &summary.channels {
        let accounts = channel_summary.accounts.as_ref();
        let probes = accounts.map_or_else(String::new, |accts| {
            accts
                .iter()
                .map(|(account_id, account_summary)| {
                    let probe = account_summary.probe.as_ref();
                    let username = probe
                        .and_then(|p| p.get("bot"))
                        .and_then(|b| b.get("username"))
                        .and_then(serde_json::Value::as_str)
                        .unwrap_or("(no bot)");
                    format!("{account_id}={username}")
                })
                .collect::<Vec<_>>()
                .join(", ")
        });
        println!("  {channel_id}: {}", if probes.is_empty() { "(none)" } else { &probes });
    }
}

/// Format a duration in milliseconds into human-readable parts.
///
/// Source: `src/commands/health.ts` - `formatDurationParts`
fn format_duration_parts(ms: u64) -> String {
    if ms < 1000 {
        return format!("{ms}ms");
    }

    let units: &[(&str, u64)] = &[
        ("w", 7 * 24 * 60 * 60 * 1000),
        ("d", 24 * 60 * 60 * 1000),
        ("h", 60 * 60 * 1000),
        ("m", 60 * 1000),
        ("s", 1000),
    ];

    let mut remaining = ms;
    let mut parts = Vec::new();
    for (label, size) in units {
        let value = remaining / size;
        if value > 0 {
            parts.push(format!("{value}{label}"));
            remaining -= value * size;
        }
    }

    if parts.is_empty() {
        "0s".to_string()
    } else {
        parts.join(" ")
    }
}

/// Get current time in milliseconds since epoch.
///
/// Source: internal helper
fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_or(0, |d| d.as_millis() as u64)
}

/// Format a health check failure error for display.
///
/// Source: `src/commands/health-format.ts` - `formatHealthCheckFailure`
pub fn format_health_check_failure(err: &dyn std::error::Error) -> String {
    let message = err.to_string();
    let lines: Vec<&str> = message
        .lines()
        .map(str::trim_end)
        .filter(|l| !l.is_empty())
        .collect();

    let details_idx = lines
        .iter()
        .position(|l| l.starts_with("Gateway target: "));

    let summary_lines = if let Some(idx) = details_idx {
        &lines[..idx]
    } else {
        &lines
    };

    let detail_lines = details_idx.map_or(&[] as &[&str], |idx| &lines[idx..]);

    let summary = if summary_lines.is_empty() {
        message.clone()
    } else {
        summary_lines
            .iter()
            .map(|l| l.trim())
            .filter(|l| !l.is_empty())
            .collect::<Vec<_>>()
            .join(" ")
    };

    let header = Theme::error("Health check failed");
    let mut out = vec![format!("{header}: {summary}")];

    for line in detail_lines {
        let formatted = format_kv_line(line);
        out.push(format!("  {formatted}"));
    }

    out.join("\n")
}

/// Format a key-value line with theme coloring.
///
/// Source: `src/commands/health-format.ts` - `formatKv`
fn format_kv_line(line: &str) -> String {
    let idx = line.find(": ");
    match idx {
        Some(i) if i > 0 => {
            let key = &line[..i];
            let value = &line[i + 2..];
            let value_color = match key {
                "Gateway target" | "Config" => Theme::command(value),
                "Source" => Theme::muted(value),
                _ => Theme::info(value),
            };
            format!("{}: {value_color}", Theme::muted(&format!("{key}:")))
        }
        _ => Theme::muted(line),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_duration_parts_sub_second() {
        assert_eq!(format_duration_parts(500), "500ms");
        assert_eq!(format_duration_parts(0), "0ms");
        assert_eq!(format_duration_parts(999), "999ms");
    }

    #[test]
    fn format_duration_parts_seconds() {
        assert_eq!(format_duration_parts(1000), "1s");
        assert_eq!(format_duration_parts(5000), "5s");
    }

    #[test]
    fn format_duration_parts_minutes() {
        assert_eq!(format_duration_parts(60_000), "1m");
        assert_eq!(format_duration_parts(90_000), "1m 30s");
    }

    #[test]
    fn format_duration_parts_hours() {
        assert_eq!(format_duration_parts(3_600_000), "1h");
        assert_eq!(format_duration_parts(7_200_000), "2h");
    }

    #[test]
    fn format_duration_parts_days() {
        assert_eq!(format_duration_parts(86_400_000), "1d");
    }

    #[test]
    fn format_duration_parts_weeks() {
        assert_eq!(format_duration_parts(7 * 86_400_000), "1w");
    }

    #[test]
    fn format_duration_parts_mixed() {
        // 1h 30m 15s = 5415000ms
        assert_eq!(format_duration_parts(5_415_000), "1h 30m 15s");
    }

    #[test]
    fn format_kv_line_gateway_target() {
        let result = format_kv_line("Gateway target: ws://127.0.0.1:19001");
        assert!(!result.is_empty());
    }

    #[test]
    fn format_kv_line_no_colon() {
        let result = format_kv_line("no colon here");
        assert!(!result.is_empty());
    }

    #[test]
    fn health_check_failure_simple() {
        let err = std::io::Error::new(std::io::ErrorKind::ConnectionRefused, "connection refused");
        let formatted = format_health_check_failure(&err);
        assert!(formatted.contains("Health check failed"));
        assert!(formatted.contains("connection refused"));
    }

    #[test]
    fn now_ms_returns_positive() {
        let ms = now_ms();
        assert!(ms > 0);
    }
}
