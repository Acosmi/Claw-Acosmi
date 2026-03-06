/// Security audit and management commands for Claw Acosmi CLI.
///
/// Provides `security` subcommands: audit.
/// Delegates to Gateway RPC: `security.get`.

use anyhow::Result;
use clap::Parser;

use oa_cli_shared::progress::with_progress;
use oa_config::io::load_config;
use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

/// CLI arguments for `security audit`.
#[derive(Debug, Parser)]
pub struct SecurityAuditArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,

    /// Include deep filesystem permission checks.
    #[arg(long)]
    pub deep: bool,

    /// Auto-fix safe defaults (chmod state/config).
    #[arg(long)]
    pub fix: bool,
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

/// Execute `security audit` — audit config and local state.
pub async fn security_audit_command(args: &SecurityAuditArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let mut params = serde_json::json!({});

    if args.deep {
        params["deep"] = serde_json::json!(true);
    }
    if args.fix {
        params["fix"] = serde_json::json!(true);
    }

    let opts = CallGatewayOptions {
        method: "security.get".to_string(),
        config: Some(cfg),
        params: Some(params),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Running security audit\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
        return Ok(());
    }

    // Display audit findings
    if let Some(findings) = result.get("findings").and_then(|f| f.as_array()) {
        if findings.is_empty() {
            println!("No security issues found.");
        } else {
            println!("Security audit findings:");
            for finding in findings {
                let severity = finding
                    .get("severity")
                    .and_then(|s| s.as_str())
                    .unwrap_or("info");
                let message = finding
                    .get("message")
                    .and_then(|m| m.as_str())
                    .unwrap_or("");
                let icon = match severity {
                    "error" | "critical" => "!",
                    "warning" | "warn" => "~",
                    _ => "-",
                };
                println!("  {icon} [{severity}] {message}");
            }
        }
    } else {
        // Fallback: raw output
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn security_audit_args_defaults() {
        let args = SecurityAuditArgs {
            json: false,
            deep: false,
            fix: false,
        };
        assert!(!args.json);
        assert!(!args.deep);
        assert!(!args.fix);
    }

    #[test]
    fn security_audit_args_all_flags() {
        let args = SecurityAuditArgs {
            json: true,
            deep: true,
            fix: true,
        };
        assert!(args.json);
        assert!(args.deep);
        assert!(args.fix);
    }
}
