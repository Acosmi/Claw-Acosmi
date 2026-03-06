/// Plugin management commands for Claw Acosmi CLI.
///
/// Provides `plugins` subcommands: list, info, install, enable, disable, doctor.
/// Delegates to Gateway RPC: `plugins.list`, `plugins.config.set`.

use anyhow::Result;
use clap::Parser;

use oa_cli_shared::progress::with_progress;
use oa_config::io::load_config;
use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

/// CLI arguments for `plugins list`.
#[derive(Debug, Parser)]
pub struct PluginsListArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `plugins info`.
#[derive(Debug, Parser)]
pub struct PluginsInfoArgs {
    /// Plugin ID.
    pub id: String,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `plugins install`.
#[derive(Debug, Parser)]
pub struct PluginsInstallArgs {
    /// Plugin path, tarball, or npm spec.
    pub spec: String,

    /// Link local path instead of copying (dev mode).
    #[arg(long, short)]
    pub link: bool,
}

/// CLI arguments for `plugins enable`.
#[derive(Debug, Parser)]
pub struct PluginsEnableArgs {
    /// Plugin ID.
    pub id: String,
}

/// CLI arguments for `plugins disable`.
#[derive(Debug, Parser)]
pub struct PluginsDisableArgs {
    /// Plugin ID.
    pub id: String,
}

/// CLI arguments for `plugins doctor`.
#[derive(Debug, Parser)]
pub struct PluginsDoctorArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

/// Execute `plugins list` — query Gateway for loaded plugins.
pub async fn plugins_list_command(args: &PluginsListArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "plugins.list".to_string(),
        config: Some(cfg),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading plugins\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
        return Ok(());
    }

    if let Some(plugins) = result.get("plugins").and_then(|p| p.as_array()) {
        if plugins.is_empty() {
            println!("No plugins loaded.");
            return Ok(());
        }
        for plugin in plugins {
            let id = plugin
                .get("id")
                .and_then(|i| i.as_str())
                .unwrap_or("(unknown)");
            let enabled = plugin
                .get("enabled")
                .and_then(|e| e.as_bool())
                .unwrap_or(false);
            let status = if enabled { "enabled" } else { "disabled" };
            let version = plugin
                .get("version")
                .and_then(|v| v.as_str())
                .unwrap_or("");
            println!("  {id} ({status}) {version}");
        }
    } else {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    }

    Ok(())
}

/// Execute `plugins info` — show plugin details.
pub async fn plugins_info_command(args: &PluginsInfoArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "plugins.list".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "id": args.id })),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading plugin info\u{2026}", call_gateway(opts)).await?
    };

    println!(
        "{}",
        serde_json::to_string_pretty(&result).unwrap_or_default()
    );
    Ok(())
}

/// Execute `plugins install` — install a plugin (stub, requires gateway restart).
pub async fn plugins_install_command(args: &PluginsInstallArgs) -> Result<()> {
    println!("Installing plugin: {}", args.spec);
    if args.link {
        println!("  Mode: link (dev)");
    }
    println!("  (not yet implemented — use config to add plugin paths)");
    println!("  After adding, restart the gateway: openacosmi gateway restart");
    Ok(())
}

/// Execute `plugins enable` — enable a plugin via config.
pub async fn plugins_enable_command(args: &PluginsEnableArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "plugins.config.set".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({
            "pluginId": args.id,
            "config": { "enabled": true }
        })),
        ..Default::default()
    };

    let _: serde_json::Value =
        with_progress("Enabling plugin\u{2026}", call_gateway(opts)).await?;

    println!("Plugin '{}' enabled. Gateway restart may be required.", args.id);
    Ok(())
}

/// Execute `plugins disable` — disable a plugin via config.
pub async fn plugins_disable_command(args: &PluginsDisableArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "plugins.config.set".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({
            "pluginId": args.id,
            "config": { "enabled": false }
        })),
        ..Default::default()
    };

    let _: serde_json::Value =
        with_progress("Disabling plugin\u{2026}", call_gateway(opts)).await?;

    println!(
        "Plugin '{}' disabled. Gateway restart may be required.",
        args.id
    );
    Ok(())
}

/// Execute `plugins doctor` — report plugin load errors.
pub async fn plugins_doctor_command(args: &PluginsDoctorArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "plugins.list".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "includeErrors": true })),
        ..Default::default()
    };

    let result: serde_json::Value =
        with_progress("Checking plugins\u{2026}", call_gateway(opts)).await?;

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        if let Some(errors) = result.get("errors").and_then(|e| e.as_array()) {
            if errors.is_empty() {
                println!("No plugin errors.");
            } else {
                for err in errors {
                    println!(
                        "  - {}",
                        err.as_str().unwrap_or(&err.to_string())
                    );
                }
            }
        } else {
            println!("No plugin errors.");
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn plugins_list_args_defaults() {
        let args = PluginsListArgs { json: false };
        assert!(!args.json);
    }

    #[test]
    fn plugins_info_args_construction() {
        let args = PluginsInfoArgs {
            id: "my-plugin".to_string(),
            json: true,
        };
        assert_eq!(args.id, "my-plugin");
        assert!(args.json);
    }

    #[test]
    fn plugins_install_args_defaults() {
        let args = PluginsInstallArgs {
            spec: "./my-plugin".to_string(),
            link: false,
        };
        assert_eq!(args.spec, "./my-plugin");
        assert!(!args.link);
    }

    #[test]
    fn plugins_enable_disable_args() {
        let en = PluginsEnableArgs {
            id: "test".to_string(),
        };
        let dis = PluginsDisableArgs {
            id: "test".to_string(),
        };
        assert_eq!(en.id, dis.id);
    }
}
