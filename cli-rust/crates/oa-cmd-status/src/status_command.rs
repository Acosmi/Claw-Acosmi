/// Main status command implementation.
///
/// Produces an overview dashboard with gateway, agents, channels, sessions,
/// security, and update information. Supports JSON output and rich terminal
/// rendering with tables.
///
/// Source: `src/commands/status.command.ts`

use std::collections::HashMap;

use anyhow::Result;

use oa_cli_shared::command_format::format_cli_command;
use oa_config::paths::resolve_gateway_port;
use oa_terminal::table::{render_table, BorderStyle, RenderTableOptions, TableColumn, Align};
use oa_terminal::theme::Theme;

use crate::daemon::{get_daemon_status_summary, get_node_daemon_status_summary};
use crate::format::{
    format_duration, format_time_ago, format_tokens_compact, shorten_text,
};
use crate::gateway_probe::resolve_gateway_probe_auth;
use crate::format::format_gateway_auth_used;
use crate::scan::scan_status;
use crate::status_all::status_all_command;
use crate::update::{
    format_update_available_hint, format_update_one_liner, resolve_update_availability,
};

/// Execute the status command.
///
/// Handles `--all`, `--json`, `--deep`, `--verbose`, and `--usage` flags.
///
/// Source: `src/commands/status.command.ts` - `statusCommand`
pub async fn status_command(
    json: bool,
    deep: bool,
    _usage: bool,
    timeout_ms: Option<u64>,
    verbose: bool,
    all: bool,
) -> Result<()> {
    // Delegate to status --all for non-JSON all mode.
    if all && !json {
        return status_all_command(timeout_ms).await;
    }

    let scan = scan_status(json, timeout_ms, all).await?;

    // JSON output mode.
    if json {
        let daemon = get_daemon_status_summary();
        let node_daemon = get_node_daemon_status_summary();
        let output = serde_json::json!({
            "sessions": scan.summary.sessions,
            "heartbeat": scan.summary.heartbeat,
            "channelSummary": scan.summary.channel_summary,
            "queuedSystemEvents": scan.summary.queued_system_events,
            "os": scan.os_summary_label,
            "update": scan.update,
            "memoryPlugin": scan.memory_plugin,
            "gateway": {
                "mode": scan.gateway_mode,
                "url": scan.gateway_connection.url,
                "urlSource": scan.gateway_connection.url_source,
                "misconfigured": scan.remote_url_missing,
                "reachable": scan.gateway_reachable,
                "connectLatencyMs": scan.gateway_probe.as_ref().and_then(|p| p.connect_latency_ms),
                "self": scan.gateway_self,
                "error": scan.gateway_probe.as_ref().and_then(|p| p.error.as_deref()),
            },
            "gatewayService": daemon,
            "nodeService": node_daemon,
            "agents": scan.agent_status,
        });
        println!(
            "{}",
            serde_json::to_string_pretty(&output).unwrap_or_default()
        );
        return Ok(());
    }

    // Verbose: print gateway connection details.
    if verbose {
        let details = oa_gateway_rpc::call::build_gateway_connection_details(
            &scan.cfg, None, None,
        );
        println!("{}", Theme::info("Gateway connection:"));
        for line in details.message.lines() {
            println!("  {line}");
        }
        println!();
    }

    // Dashboard URL.
    let dashboard = {
        let control_ui_enabled = scan
            .cfg
            .gateway
            .as_ref()
            .and_then(|g| g.control_ui.as_ref())
            .and_then(|c| c.enabled)
            .unwrap_or(true);
        if control_ui_enabled {
            let port = resolve_gateway_port(Some(&scan.cfg));
            format!("http://127.0.0.1:{port}")
        } else {
            "disabled".to_string()
        }
    };

    // Gateway value.
    let gateway_value = {
        let target = if scan.remote_url_missing {
            format!("fallback {}", scan.gateway_connection.url)
        } else {
            let source_suffix = if scan.gateway_connection.url_source.is_empty() {
                String::new()
            } else {
                format!(" ({})", scan.gateway_connection.url_source)
            };
            format!("{}{source_suffix}", scan.gateway_connection.url)
        };
        let reach = if scan.remote_url_missing {
            Theme::warn("misconfigured (remote.url missing)")
        } else if scan.gateway_reachable {
            let latency = scan
                .gateway_probe
                .as_ref()
                .and_then(|p| p.connect_latency_ms);
            Theme::success(&format!("reachable {}", format_duration(latency)))
        } else {
            let error = scan
                .gateway_probe
                .as_ref()
                .and_then(|p| p.error.as_deref())
                .unwrap_or("unreachable");
            Theme::warn(&format!("unreachable ({error})"))
        };
        let probe_auth = resolve_gateway_probe_auth(&scan.cfg);
        let auth_suffix = if scan.gateway_reachable && !scan.remote_url_missing {
            format!(
                " \u{00b7} auth {}",
                format_gateway_auth_used(Some(&probe_auth))
            )
        } else {
            String::new()
        };
        let self_suffix = scan
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
            .filter(|s| !s.is_empty())
            .map_or(String::new(), |s| format!(" \u{00b7} {s}"));
        format!(
            "{} \u{00b7} {} \u{00b7} {reach}{auth_suffix}{self_suffix}",
            scan.gateway_mode, target
        )
    };

    // Agents value.
    let agents_value = {
        let pending = if scan.agent_status.bootstrap_pending_count > 0 {
            format!(
                "{} bootstrapping",
                scan.agent_status.bootstrap_pending_count
            )
        } else {
            "no bootstraps".to_string()
        };
        let def = scan
            .agent_status
            .agents
            .iter()
            .find(|a| a.id == scan.agent_status.default_id);
        let def_suffix = def.map_or(String::new(), |d| {
            let active = d
                .last_active_age_ms
                .map_or("unknown".to_string(), format_time_ago);
            format!(
                " \u{00b7} default {} active {active}",
                d.id
            )
        });
        format!(
            "{} \u{00b7} {pending} \u{00b7} sessions {}{def_suffix}",
            scan.agent_status.agents.len(),
            scan.agent_status.total_sessions
        )
    };

    // Daemon values.
    let daemon = get_daemon_status_summary();
    let node_daemon = get_node_daemon_status_summary();
    let daemon_value = format_daemon_overview(&daemon);
    let node_value = format_daemon_overview(&node_daemon);

    // Defaults.
    let defaults = &scan.summary.sessions.defaults;
    let default_ctx = defaults
        .context_tokens
        .map_or(String::new(), |ct| format!(" ({}k ctx)", ct / 1000));

    // Events.
    let events_value = if scan.summary.queued_system_events.is_empty() {
        "none".to_string()
    } else {
        format!("{} queued", scan.summary.queued_system_events.len())
    };

    // Probes (stub: always skipped without --deep).
    let probes_value = if deep {
        Theme::success("enabled")
    } else {
        Theme::muted("skipped (use --deep)")
    };

    // Heartbeat.
    let heartbeat_value = {
        let parts: Vec<String> = scan
            .summary
            .heartbeat
            .agents
            .iter()
            .map(|agent| {
                if !agent.enabled || agent.every_ms.is_none() {
                    format!("disabled ({})", agent.agent_id)
                } else {
                    format!("{} ({})", agent.every, agent.agent_id)
                }
            })
            .collect();
        if parts.is_empty() {
            "disabled".to_string()
        } else {
            parts.join(", ")
        }
    };

    // Memory.
    let memory_value = if !scan.memory_plugin.enabled {
        let suffix = scan
            .memory_plugin
            .reason
            .as_deref()
            .map_or(String::new(), |r| format!(" ({r})"));
        Theme::muted(&format!("disabled{suffix}"))
    } else {
        let slot = scan
            .memory_plugin
            .slot
            .as_deref()
            .map_or("plugin".to_string(), |s| format!("plugin {s}"));
        Theme::muted(&format!("enabled ({slot}) \u{00b7} unavailable"))
    };

    // Tailscale.
    let tailscale_value = if scan.tailscale_mode == "off" {
        Theme::muted("off")
    } else if let (Some(dns), Some(https_url)) =
        (&scan.tailscale_dns, &scan.tailscale_https_url)
    {
        format!("{} \u{00b7} {dns} \u{00b7} {https_url}", scan.tailscale_mode)
    } else {
        Theme::warn(&format!(
            "{} \u{00b7} magicdns unknown",
            scan.tailscale_mode
        ))
    };

    // Update.
    let update_availability =
        resolve_update_availability(&scan.update, "dev");
    let update_line = format_update_one_liner(&scan.update, "dev")
        .trim_start_matches("Update: ")
        .to_string();
    let update_value = if update_availability.available {
        Theme::warn(&format!("available \u{00b7} {update_line}"))
    } else {
        update_line
    };

    // Store label.
    let store_label = if scan.summary.sessions.paths.len() > 1 {
        format!("{} stores", scan.summary.sessions.paths.len())
    } else {
        scan.summary
            .sessions
            .paths
            .first()
            .cloned()
            .unwrap_or_else(|| "unknown".to_string())
    };

    // Sessions value.
    let sessions_value = format!(
        "{} active \u{00b7} default {}{default_ctx} \u{00b7} {store_label}",
        scan.summary.sessions.count,
        defaults.model.as_deref().unwrap_or("unknown"),
    );

    // Build overview table rows.
    let overview_rows: Vec<HashMap<String, String>> = vec![
        make_row("Dashboard", &dashboard),
        make_row("OS", &scan.os_summary_label),
        make_row("Tailscale", &tailscale_value),
        make_row("Update", &update_value),
        make_row("Gateway", &gateway_value),
        make_row("Gateway service", &daemon_value),
        make_row("Node service", &node_value),
        make_row("Agents", &agents_value),
        make_row("Memory", &memory_value),
        make_row("Probes", &probes_value),
        make_row("Events", &events_value),
        make_row("Heartbeat", &heartbeat_value),
        make_row("Sessions", &sessions_value),
    ];

    // Print overview.
    println!("{}", Theme::heading("OpenAcosmi status"));
    println!();
    println!("{}", Theme::heading("Overview"));
    println!(
        "{}",
        render_table(&RenderTableOptions {
            columns: vec![
                TableColumn {
                    key: "Item".to_string(),
                    header: "Item".to_string(),
                    align: Align::Left,
                    min_width: Some(12),
                    max_width: None,
                    flex: false,
                },
                TableColumn {
                    key: "Value".to_string(),
                    header: "Value".to_string(),
                    align: Align::Left,
                    min_width: Some(32),
                    max_width: None,
                    flex: true,
                },
            ],
            rows: overview_rows,
            width: Some(120),
            padding: 1,
            border: BorderStyle::Unicode,
        })
        .trim_end()
    );

    // Security audit (stub: not available in Rust migration).
    println!();
    println!("{}", Theme::heading("Security audit"));
    println!(
        "{}",
        Theme::muted("Security audit not available (Rust migration). Run:")
    );
    println!(
        "  {}",
        Theme::muted(&format_cli_command("openacosmi security audit"))
    );
    println!(
        "  {}",
        Theme::muted(&format_cli_command("openacosmi security audit --deep"))
    );

    // Channels table.
    println!();
    println!("{}", Theme::heading("Channels"));
    if scan.channels.rows.is_empty() {
        println!(
            "{}",
            Theme::muted("  No channel plugins loaded (Rust migration).")
        );
    } else {
        let channel_rows: Vec<HashMap<String, String>> = scan
            .channels
            .rows
            .iter()
            .map(|row| {
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
                let mut m = HashMap::new();
                m.insert("Channel".to_string(), row.label.clone());
                m.insert("Enabled".to_string(), enabled_label);
                m.insert("State".to_string(), state_label);
                m.insert("Detail".to_string(), row.detail.clone());
                m
            })
            .collect();
        println!(
            "{}",
            render_table(&RenderTableOptions {
                columns: vec![
                    TableColumn {
                        key: "Channel".to_string(),
                        header: "Channel".to_string(),
                        align: Align::Left,
                        min_width: Some(10),
                        max_width: None,
                        flex: false,
                    },
                    TableColumn {
                        key: "Enabled".to_string(),
                        header: "Enabled".to_string(),
                        align: Align::Left,
                        min_width: Some(7),
                        max_width: None,
                        flex: false,
                    },
                    TableColumn {
                        key: "State".to_string(),
                        header: "State".to_string(),
                        align: Align::Left,
                        min_width: Some(8),
                        max_width: None,
                        flex: false,
                    },
                    TableColumn {
                        key: "Detail".to_string(),
                        header: "Detail".to_string(),
                        align: Align::Left,
                        min_width: Some(24),
                        max_width: None,
                        flex: true,
                    },
                ],
                rows: channel_rows,
                width: Some(120),
                padding: 1,
                border: BorderStyle::Unicode,
            })
            .trim_end()
        );
    }

    // Sessions table.
    println!();
    println!("{}", Theme::heading("Sessions"));
    let session_rows: Vec<HashMap<String, String>> = if scan.summary.sessions.recent.is_empty() {
        vec![{
            let mut m = HashMap::new();
            m.insert("Key".to_string(), Theme::muted("no sessions yet"));
            m.insert("Kind".to_string(), String::new());
            m.insert("Age".to_string(), String::new());
            m.insert("Model".to_string(), String::new());
            m.insert("Tokens".to_string(), String::new());
            m
        }]
    } else {
        scan.summary
            .sessions
            .recent
            .iter()
            .map(|sess| {
                let mut m = HashMap::new();
                m.insert("Key".to_string(), shorten_text(&sess.key, 32));
                m.insert("Kind".to_string(), format!("{:?}", sess.kind));
                m.insert(
                    "Age".to_string(),
                    sess.age
                        .map_or("no activity".to_string(), format_time_ago),
                );
                m.insert(
                    "Model".to_string(),
                    sess.model.as_deref().unwrap_or("unknown").to_string(),
                );
                m.insert("Tokens".to_string(), format_tokens_compact(sess));
                m
            })
            .collect()
    };
    println!(
        "{}",
        render_table(&RenderTableOptions {
            columns: vec![
                TableColumn {
                    key: "Key".to_string(),
                    header: "Key".to_string(),
                    align: Align::Left,
                    min_width: Some(20),
                    max_width: None,
                    flex: true,
                },
                TableColumn {
                    key: "Kind".to_string(),
                    header: "Kind".to_string(),
                    align: Align::Left,
                    min_width: Some(6),
                    max_width: None,
                    flex: false,
                },
                TableColumn {
                    key: "Age".to_string(),
                    header: "Age".to_string(),
                    align: Align::Left,
                    min_width: Some(9),
                    max_width: None,
                    flex: false,
                },
                TableColumn {
                    key: "Model".to_string(),
                    header: "Model".to_string(),
                    align: Align::Left,
                    min_width: Some(14),
                    max_width: None,
                    flex: false,
                },
                TableColumn {
                    key: "Tokens".to_string(),
                    header: "Tokens".to_string(),
                    align: Align::Left,
                    min_width: Some(16),
                    max_width: None,
                    flex: false,
                },
            ],
            rows: session_rows,
            width: Some(120),
            padding: 1,
            border: BorderStyle::Unicode,
        })
        .trim_end()
    );

    // System events.
    if !scan.summary.queued_system_events.is_empty() {
        println!();
        println!("{}", Theme::heading("System events"));
        let event_rows: Vec<HashMap<String, String>> = scan
            .summary
            .queued_system_events
            .iter()
            .take(5)
            .map(|event| {
                let mut m = HashMap::new();
                m.insert("Event".to_string(), event.clone());
                m
            })
            .collect();
        println!(
            "{}",
            render_table(&RenderTableOptions {
                columns: vec![TableColumn {
                    key: "Event".to_string(),
                    header: "Event".to_string(),
                    align: Align::Left,
                    min_width: Some(24),
                    max_width: None,
                    flex: true,
                }],
                rows: event_rows,
                width: Some(120),
                padding: 1,
                border: BorderStyle::Unicode,
            })
            .trim_end()
        );
        if scan.summary.queued_system_events.len() > 5 {
            println!(
                "{}",
                Theme::muted(&format!(
                    "\u{2026} +{} more",
                    scan.summary.queued_system_events.len() - 5
                ))
            );
        }
    }

    // Footer.
    println!();
    println!("FAQ: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/general/faq");
    println!("Troubleshooting: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/general/troubleshooting");
    println!();
    if let Some(update_hint) = format_update_available_hint(&scan.update, "dev") {
        println!("{}", Theme::warn(&update_hint));
        println!();
    }
    println!("Next steps:");
    println!(
        "  Need to share?      {}",
        format_cli_command("openacosmi status --all")
    );
    println!(
        "  Need to debug live? {}",
        format_cli_command("openacosmi logs --follow")
    );
    if scan.gateway_reachable {
        println!(
            "  Need to test channels? {}",
            format_cli_command("openacosmi status --deep")
        );
    } else {
        println!(
            "  Fix reachability first: {}",
            format_cli_command("openacosmi gateway probe")
        );
    }

    Ok(())
}

/// Format a daemon summary for the overview table.
///
/// Source: `src/commands/status.command.ts` - daemon formatting
fn format_daemon_overview(daemon: &crate::daemon::DaemonStatusSummary) -> String {
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

/// Create a table row with "Item" and "Value" keys.
fn make_row(item: &str, value: &str) -> HashMap<String, String> {
    let mut m = HashMap::new();
    m.insert("Item".to_string(), item.to_string());
    m.insert("Value".to_string(), value.to_string());
    m
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_daemon_overview_not_installed() {
        let d = crate::daemon::DaemonStatusSummary {
            label: "LaunchAgent".to_string(),
            installed: Some(false),
            loaded_text: "not loaded".to_string(),
            runtime_short: None,
        };
        let line = format_daemon_overview(&d);
        assert!(line.contains("not installed"));
    }

    #[test]
    fn format_daemon_overview_installed() {
        let d = crate::daemon::DaemonStatusSummary {
            label: "systemd".to_string(),
            installed: Some(true),
            loaded_text: "loaded".to_string(),
            runtime_short: Some("running (pid 42)".to_string()),
        };
        let line = format_daemon_overview(&d);
        assert!(line.contains("installed"));
        assert!(line.contains("loaded"));
        assert!(line.contains("running (pid 42)"));
    }

    #[test]
    fn make_row_creates_hashmap() {
        let row = make_row("key", "value");
        assert_eq!(row.get("Item").map(String::as_str), Some("key"));
        assert_eq!(row.get("Value").map(String::as_str), Some("value"));
    }
}
