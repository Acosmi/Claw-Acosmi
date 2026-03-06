/// System diagnostics commands for Claw Acosmi CLI.
///
/// The `doctor` command inspects and repairs the configuration, auth profiles,
/// gateway service, sandbox images, shell completion, state integrity, and
/// security posture.  Each sub-concern is isolated in its own module, mirroring
/// the original TypeScript source files.
///
/// Source: `src/commands/doctor*.ts`

// ── Sub-modules (one per TS source file) ────────────────────────────────────

/// Authentication profile health checks and repair.
/// Source: `src/commands/doctor-auth.ts`
pub mod auth;

/// Shell completion install / upgrade / cache generation.
/// Source: `src/commands/doctor-completion.ts`
pub mod completion;

/// Config loading, legacy migration, unknown-key stripping, plugin auto-enable.
/// Source: `src/commands/doctor-config-flow.ts`
pub mod config_flow;

/// Gateway daemon runtime summary and hint formatting.
/// Source: `src/commands/doctor-format.ts`
pub mod format;

/// Gateway daemon repair flow (install / start / restart / LaunchAgent bootstrap).
/// Source: `src/commands/doctor-gateway-daemon-flow.ts`
pub mod gateway_daemon_flow;

/// Gateway health check and channel-status probe.
/// Source: `src/commands/doctor-gateway-health.ts`
pub mod gateway_health;

/// Gateway service config audit, legacy cleanup, extra-service scanning.
/// Source: `src/commands/doctor-gateway-services.ts`
pub mod gateway_services;

/// Source-install integrity checks (pnpm workspace, tsx binary, etc.).
/// Source: `src/commands/doctor-install.ts`
pub mod install;

/// Legacy config value normalization (ackReaction migration).
/// Source: `src/commands/doctor-legacy-config.ts`
pub mod legacy_config;

/// Platform-specific notes (macOS LaunchAgent overrides, launchctl env, legacy env vars).
/// Source: `src/commands/doctor-platform-notes.ts`
pub mod platform_notes;

/// Doctor prompter factory — interactive / yes / non-interactive modes.
/// Source: `src/commands/doctor-prompter.ts`
pub mod prompter;

/// Sandbox image presence checks and build helpers.
/// Source: `src/commands/doctor-sandbox.ts`
pub mod sandbox;

/// Security warnings (gateway network exposure, channel DM policies).
/// Source: `src/commands/doctor-security.ts`
pub mod security;

/// State directory integrity, permissions, session-transcript presence.
/// Source: `src/commands/doctor-state-integrity.ts`
pub mod state_integrity;

/// Legacy state migration detection and execution.
/// Source: `src/commands/doctor-state-migrations.ts`
pub mod state_migrations;

/// UI protocol freshness check and rebuild prompt.
/// Source: `src/commands/doctor-ui.ts`
pub mod ui;

/// Pre-doctor update offer (git or package manager).
/// Source: `src/commands/doctor-update.ts`
pub mod update;

/// Workspace status reporting (skills, plugins, legacy dirs).
/// Source: `src/commands/doctor-workspace-status.ts`
pub mod workspace_status;

/// Memory-system suggestion and legacy workspace detection.
/// Source: `src/commands/doctor-workspace.ts`
pub mod workspace;

// ── Re-exports ──────────────────────────────────────────────────────────────

pub use prompter::{DoctorOptions, DoctorPrompter};

// ── Top-level doctor command ────────────────────────────────────────────────

use anyhow::Result;
use tracing::warn;

use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::{read_config_file_snapshot, write_config_file};
use oa_config::paths::resolve_config_path;
use oa_terminal::note::note;
use oa_types::config::OpenAcosmiConfig;
use oa_types::gateway::GatewayMode;

/// Resolve the effective gateway mode from config.
///
/// Source: `src/commands/doctor.ts` — `resolveMode`
pub fn resolve_mode(cfg: &OpenAcosmiConfig) -> GatewayMode {
    cfg.gateway
        .as_ref()
        .and_then(|gw| gw.mode.clone())
        .unwrap_or(GatewayMode::Local)
}

/// Generate a random hex token (32 bytes, 64 hex chars).
///
/// Source: `src/commands/onboard-helpers.ts` — `randomToken`
fn random_token() -> String {
    uuid::Uuid::new_v4().to_string().replace('-', "")
}

/// Execute the full doctor command.
///
/// Mirrors the TypeScript `doctorCommand` in `src/commands/doctor.ts`.
///
/// Source: `src/commands/doctor.ts`
pub async fn execute(options: DoctorOptions) -> Result<()> {
    let mut prompter_state = prompter::create_doctor_prompter(&options);

    note("OpenAcosmi doctor", Some("Doctor"));

    // ── 1. Pre-doctor update offer ──
    let update_result = update::maybe_offer_update_before_doctor(&options).await;
    if update_result.handled {
        return Ok(());
    }

    // ── 2. UI freshness ──
    ui::maybe_repair_ui_protocol_freshness(&mut prompter_state).await;

    // ── 3. Source install issues ──
    install::note_source_install_issues(None);

    // ── 4. Deprecated legacy env vars ──
    platform_notes::note_deprecated_legacy_env_vars(None);

    // ── 5. Config loading + migrations ──
    let config_result = config_flow::load_and_maybe_migrate_doctor_config(&options, &mut prompter_state).await;
    let mut cfg = config_result.cfg;
    let config_path = config_result
        .path
        .unwrap_or_else(|| resolve_config_path().display().to_string());

    // ── 6. Gateway mode check ──
    if cfg.gateway.as_ref().and_then(|gw| gw.mode.as_ref()).is_none() {
        let lines = vec![
            "gateway.mode is unset; gateway start will be blocked.".to_string(),
            format!(
                "Fix: run {} and set Gateway mode (local/remote).",
                format_cli_command("openacosmi configure")
            ),
            format!(
                "Or set directly: {}",
                format_cli_command("openacosmi config set gateway.mode local")
            ),
        ];
        if !std::path::Path::new(&config_path).exists() {
            note(
                &format!(
                    "{}\nMissing config: run {} first.",
                    lines.join("\n"),
                    format_cli_command("openacosmi setup")
                ),
                Some("Gateway"),
            );
        } else {
            note(&lines.join("\n"), Some("Gateway"));
        }
    }

    // ── 7. Auth profile repair ──
    cfg = auth::maybe_repair_anthropic_oauth_profile_id(cfg, &mut prompter_state).await;
    cfg = auth::maybe_remove_deprecated_cli_auth_profiles(cfg, &mut prompter_state).await;
    auth::note_auth_profile_health(&cfg, &mut prompter_state).await;

    // ── 8. Gateway token check (local mode) ──
    if resolve_mode(&cfg) == GatewayMode::Local {
        let auth_config = cfg.gateway.as_ref().and_then(|gw| gw.auth.as_ref());
        let auth_mode = auth_config.and_then(|a| a.mode.as_ref());
        let has_token = auth_config
            .and_then(|a| a.token.as_ref())
            .is_some_and(|t| !t.trim().is_empty());
        let needs_token = !matches!(
            auth_mode,
            Some(oa_types::gateway::GatewayAuthMode::Password)
        ) && !(matches!(
            auth_mode,
            Some(oa_types::gateway::GatewayAuthMode::Token)
        ) && has_token);

        if needs_token {
            note(
                "Gateway auth is off or missing a token. Token auth is now the recommended default (including loopback).",
                Some("Gateway auth"),
            );

            let should_set = if options.generate_gateway_token == Some(true) {
                true
            } else if options.non_interactive == Some(true) {
                false
            } else {
                prompter_state
                    .confirm_repair("Generate and configure a gateway token now?", true)
                    .await
            };

            if should_set {
                let next_token = random_token();
                let mut gw = cfg.gateway.clone().unwrap_or_default();
                let mut gw_auth = gw.auth.clone().unwrap_or_default();
                gw_auth.mode = Some(oa_types::gateway::GatewayAuthMode::Token);
                gw_auth.token = Some(next_token);
                gw.auth = Some(gw_auth);
                cfg.gateway = Some(gw);
                note("Gateway token configured.", Some("Gateway auth"));
            }
        }
    }

    // ── 9. Legacy state migrations ──
    let legacy_state = state_migrations::detect_legacy_state_migrations(&cfg).await;
    if !legacy_state.preview.is_empty() {
        note(&legacy_state.preview.join("\n"), Some("Legacy state detected"));
        let migrate = if options.non_interactive == Some(true) {
            true
        } else {
            prompter_state
                .confirm(
                    "Migrate legacy state (sessions/agent/WhatsApp auth) now?",
                    true,
                )
                .await
        };
        if migrate {
            let migrated = state_migrations::run_legacy_state_migrations(&legacy_state).await;
            if !migrated.changes.is_empty() {
                note(&migrated.changes.join("\n"), Some("Doctor changes"));
            }
            if !migrated.warnings.is_empty() {
                note(&migrated.warnings.join("\n"), Some("Doctor warnings"));
            }
        }
    }

    // ── 10. State integrity ──
    state_integrity::note_state_integrity(&cfg, &mut prompter_state, Some(&config_path)).await;

    // ── 11. Sandbox images ──
    cfg = sandbox::maybe_repair_sandbox_images(cfg, &mut prompter_state).await;
    sandbox::note_sandbox_scope_warnings(&cfg);

    // ── 12. Gateway services ──
    gateway_services::maybe_scan_extra_gateway_services(&options, &mut prompter_state).await;
    gateway_services::maybe_repair_gateway_service_config(
        &cfg,
        &resolve_mode(&cfg),
        &mut prompter_state,
    )
    .await;

    // ── 13. Platform-specific notes ──
    platform_notes::note_mac_launch_agent_overrides().await;
    platform_notes::note_mac_launchctl_gateway_env_overrides(&cfg).await;

    // ── 14. Security ──
    security::note_security_warnings(&cfg).await;

    // ── 15. Workspace status ──
    workspace_status::note_workspace_status(&cfg);

    // ── 16. Shell completion ──
    completion::doctor_shell_completion(&mut prompter_state, &options).await;

    // ── 17. Gateway health ──
    let timeout_ms = if options.non_interactive == Some(true) {
        3_000
    } else {
        10_000
    };
    let health = gateway_health::check_gateway_health(&cfg, timeout_ms).await;
    gateway_daemon_flow::maybe_repair_gateway_daemon(
        &cfg,
        &mut prompter_state,
        &options,
        health.health_ok,
    )
    .await;

    // ── 18. Write config if needed ──
    let should_write = prompter_state.should_repair || config_result.should_write_config;
    if should_write {
        if let Err(e) = write_config_file(&cfg).await {
            warn!("Failed to write config: {e}");
        } else {
            note("Config updated.", Some("Doctor"));
            let backup_path = format!("{config_path}.bak");
            if std::path::Path::new(&backup_path).exists() {
                note(
                    &format!("Backup: {backup_path}"),
                    Some("Doctor"),
                );
            }
        }
    } else {
        note(
            &format!(
                "Run \"{}\" to apply changes.",
                format_cli_command("openacosmi doctor --fix")
            ),
            Some("Doctor"),
        );
    }

    // ── 19. Workspace suggestions ──
    if options.workspace_suggestions != Some(false) {
        let state_dir = oa_config::paths::resolve_state_dir();
        let workspace_dir = state_dir.join("workspace");
        state_integrity::note_workspace_backup_tip(&workspace_dir);
        if workspace::should_suggest_memory_system(&workspace_dir).await {
            note(&workspace::MEMORY_SYSTEM_PROMPT, Some("Workspace"));
        }
    }

    // ── 20. Final config validation ──
    match read_config_file_snapshot().await {
        Ok(snapshot) => {
            if snapshot.exists && !snapshot.valid {
                warn!("Invalid config:");
                for issue in &snapshot.issues {
                    let path = if issue.path.is_empty() {
                        "<root>"
                    } else {
                        &issue.path
                    };
                    warn!("- {path}: {}", issue.message);
                }
            }
        }
        Err(e) => {
            warn!("Failed to read final config snapshot: {e}");
        }
    }

    note("Doctor complete.", Some("Doctor"));

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolve_mode_defaults_to_local() {
        let cfg = OpenAcosmiConfig::default();
        assert_eq!(resolve_mode(&cfg), GatewayMode::Local);
    }

    #[test]
    fn resolve_mode_remote_when_set() {
        let mut cfg = OpenAcosmiConfig::default();
        cfg.gateway = Some(oa_types::gateway::GatewayConfig {
            mode: Some(GatewayMode::Remote),
            ..Default::default()
        });
        assert_eq!(resolve_mode(&cfg), GatewayMode::Remote);
    }

    #[test]
    fn random_token_is_hex_and_nonempty() {
        let token = random_token();
        assert!(!token.is_empty());
        assert!(token.chars().all(|c| c.is_ascii_hexdigit()));
    }
}
