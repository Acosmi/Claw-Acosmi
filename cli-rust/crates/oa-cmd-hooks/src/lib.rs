/// Hook management commands for Claw Acosmi CLI.
///
/// Provides `hooks` subcommands: list, info, check, enable, disable, install, update.
/// Hooks are managed via config — no dedicated Gateway RPC.

use anyhow::Result;
use clap::Parser;

use oa_cli_shared::progress::with_progress;
use oa_config::io::load_config;
use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

/// CLI arguments for `hooks list`.
#[derive(Debug, Parser)]
pub struct HooksListArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,

    /// Show only eligible hooks.
    #[arg(long)]
    pub eligible: bool,

    /// Show verbose details.
    #[arg(long, short)]
    pub verbose: bool,
}

/// CLI arguments for `hooks info`.
#[derive(Debug, Parser)]
pub struct HooksInfoArgs {
    /// Hook name.
    pub name: String,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `hooks check`.
#[derive(Debug, Parser)]
pub struct HooksCheckArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `hooks enable`.
#[derive(Debug, Parser)]
pub struct HooksEnableArgs {
    /// Hook name.
    pub name: String,
}

/// CLI arguments for `hooks disable`.
#[derive(Debug, Parser)]
pub struct HooksDisableArgs {
    /// Hook name.
    pub name: String,
}

/// CLI arguments for `hooks install`.
#[derive(Debug, Parser)]
pub struct HooksInstallArgs {
    /// Path or npm spec.
    pub spec: String,

    /// Link local path instead of copying (dev mode).
    #[arg(long, short)]
    pub link: bool,
}

/// CLI arguments for `hooks update`.
#[derive(Debug, Parser)]
pub struct HooksUpdateArgs {
    /// Specific hook-pack ID (optional if --all).
    pub id: Option<String>,

    /// Update all hook packs.
    #[arg(long)]
    pub all: bool,

    /// Dry run (preview without executing).
    #[arg(long)]
    pub dry_run: bool,
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

/// Execute `hooks list` — list configured hooks via config.get.
pub async fn hooks_list_command(args: &HooksListArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "config.get".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "path": "hooks" })),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading hooks\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
        return Ok(());
    }

    // Display hooks from config
    if let Some(hooks) = result.as_object() {
        if hooks.is_empty() {
            println!("No hooks configured.");
        } else {
            for (name, value) in hooks {
                let enabled = value
                    .get("enabled")
                    .and_then(|e| e.as_bool())
                    .unwrap_or(true);
                let status = if enabled { "enabled" } else { "disabled" };
                println!("  {name} ({status})");
                if args.verbose {
                    if let Some(desc) = value.get("description").and_then(|d| d.as_str()) {
                        println!("    {desc}");
                    }
                }
            }
        }
    } else {
        println!("No hooks configured.");
    }

    Ok(())
}

/// Execute `hooks info` — show hook details.
pub async fn hooks_info_command(args: &HooksInfoArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "config.get".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "path": format!("hooks.{}", args.name) })),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading hook info\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        println!("Hook: {}", args.name);
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    }
    Ok(())
}

/// Execute `hooks check` — verify hook system health.
pub async fn hooks_check_command(args: &HooksCheckArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "config.get".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "path": "hooks" })),
        ..Default::default()
    };

    let result: serde_json::Value =
        with_progress("Checking hooks\u{2026}", call_gateway(opts)).await?;

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        println!("Hook system: OK");
        if let Some(hooks) = result.as_object() {
            println!("  {} hooks configured", hooks.len());
        }
    }
    Ok(())
}

/// Execute `hooks enable` — enable a hook via config.patch.
pub async fn hooks_enable_command(args: &HooksEnableArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "config.patch".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({
            "config": { "hooks": { &args.name: { "enabled": true } } }
        })),
        ..Default::default()
    };

    let _: serde_json::Value =
        with_progress("Enabling hook\u{2026}", call_gateway(opts)).await?;

    println!("Hook '{}' enabled.", args.name);
    Ok(())
}

/// Execute `hooks disable` — disable a hook via config.patch.
pub async fn hooks_disable_command(args: &HooksDisableArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "config.patch".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({
            "config": { "hooks": { &args.name: { "enabled": false } } }
        })),
        ..Default::default()
    };

    let _: serde_json::Value =
        with_progress("Disabling hook\u{2026}", call_gateway(opts)).await?;

    println!("Hook '{}' disabled.", args.name);
    Ok(())
}

/// Execute `hooks install` — install a hook pack (stub).
pub async fn hooks_install_command(args: &HooksInstallArgs) -> Result<()> {
    println!("Installing hook: {}", args.spec);
    if args.link {
        println!("  Mode: link (dev)");
    }
    println!("  (not yet fully implemented)");
    Ok(())
}

/// Execute `hooks update` — update hook packs (stub).
pub async fn hooks_update_command(args: &HooksUpdateArgs) -> Result<()> {
    if let Some(ref id) = args.id {
        println!("Updating hook pack: {id}");
    } else if args.all {
        println!("Updating all hook packs");
    } else {
        println!("Specify a hook-pack ID or use --all");
        return Ok(());
    }
    if args.dry_run {
        println!("  (dry run — no changes made)");
    } else {
        println!("  (not yet fully implemented)");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hooks_list_args_defaults() {
        let args = HooksListArgs {
            json: false,
            eligible: false,
            verbose: false,
        };
        assert!(!args.json);
        assert!(!args.eligible);
        assert!(!args.verbose);
    }

    #[test]
    fn hooks_info_args_construction() {
        let args = HooksInfoArgs {
            name: "my-hook".to_string(),
            json: true,
        };
        assert_eq!(args.name, "my-hook");
        assert!(args.json);
    }

    #[test]
    fn hooks_update_args_optional_id() {
        let args = HooksUpdateArgs {
            id: None,
            all: true,
            dry_run: false,
        };
        assert!(args.id.is_none());
        assert!(args.all);
        assert!(!args.dry_run);
    }

    #[test]
    fn hooks_install_args_link_mode() {
        let args = HooksInstallArgs {
            spec: "path/to/hook".to_string(),
            link: true,
        };
        assert!(args.link);
    }
}
