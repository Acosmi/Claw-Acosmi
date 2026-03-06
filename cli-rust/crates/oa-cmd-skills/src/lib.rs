/// Skill management commands for Claw Acosmi CLI.
///
/// Provides `skills` subcommands: list, info, check.
/// Delegates to Gateway RPC: `skills.status`, `skills.bins`.

use anyhow::Result;
use clap::Parser;

use oa_cli_shared::progress::with_progress;
use oa_config::io::load_config;
use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

/// CLI arguments for `skills list`.
#[derive(Debug, Parser)]
pub struct SkillsListArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,

    /// Show only eligible skills for current agent.
    #[arg(long)]
    pub eligible: bool,

    /// Show verbose details.
    #[arg(long, short)]
    pub verbose: bool,
}

/// CLI arguments for `skills info`.
#[derive(Debug, Parser)]
pub struct SkillsInfoArgs {
    /// Skill name or ID.
    pub name: String,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `skills check`.
#[derive(Debug, Parser)]
pub struct SkillsCheckArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

/// Execute `skills list` — query Gateway for skill status.
pub async fn skills_list_command(args: &SkillsListArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "skills.status".to_string(),
        config: Some(cfg),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading skills\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
        return Ok(());
    }

    // Display skills from status response
    if let Some(skills) = result.get("skills").and_then(|s| s.as_array()) {
        if skills.is_empty() {
            println!("No skills loaded.");
            return Ok(());
        }
        for skill in skills {
            let name = skill
                .get("name")
                .and_then(|n| n.as_str())
                .unwrap_or("(unknown)");
            let category = skill
                .get("category")
                .and_then(|c| c.as_str())
                .unwrap_or("");
            let eligible = skill
                .get("eligible")
                .and_then(|e| e.as_bool())
                .unwrap_or(true);

            let marker = if !eligible && args.eligible {
                continue;
            } else if !eligible {
                " (ineligible)"
            } else {
                ""
            };

            if args.verbose {
                let desc = skill
                    .get("description")
                    .and_then(|d| d.as_str())
                    .unwrap_or("");
                println!("  {category}/{name}{marker}");
                if !desc.is_empty() {
                    println!("    {desc}");
                }
            } else {
                println!("  {category}/{name}{marker}");
            }
        }
    } else {
        println!("{}", serde_json::to_string_pretty(&result).unwrap_or_default());
    }

    Ok(())
}

/// Execute `skills info` — query Gateway for skill details.
pub async fn skills_info_command(args: &SkillsInfoArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "skills.bins".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "name": args.name })),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading skill info\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        println!("Skill: {}", args.name);
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    }
    Ok(())
}

/// Execute `skills check` — verify skill system health.
pub async fn skills_check_command(args: &SkillsCheckArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "skills.status".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "check": true })),
        ..Default::default()
    };

    let result: serde_json::Value =
        with_progress("Checking skills\u{2026}", call_gateway(opts)).await?;

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        let total = result
            .get("totalCount")
            .and_then(|t| t.as_u64())
            .unwrap_or(0);
        let indexed = result
            .get("indexedCount")
            .and_then(|i| i.as_u64())
            .unwrap_or(0);
        println!("Skills: {indexed}/{total} indexed");
        if let Some(errors) = result.get("errors").and_then(|e| e.as_array()) {
            if !errors.is_empty() {
                println!("Errors:");
                for err in errors {
                    println!("  - {}", err.as_str().unwrap_or("(unknown)"));
                }
            }
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn skills_list_args_defaults() {
        let args = SkillsListArgs {
            json: false,
            eligible: false,
            verbose: false,
        };
        assert!(!args.json);
        assert!(!args.eligible);
        assert!(!args.verbose);
    }

    #[test]
    fn skills_info_args_construction() {
        let args = SkillsInfoArgs {
            name: "test-skill".to_string(),
            json: true,
        };
        assert_eq!(args.name, "test-skill");
        assert!(args.json);
    }

    #[test]
    fn skills_check_args_defaults() {
        let args = SkillsCheckArgs { json: false };
        assert!(!args.json);
    }
}
