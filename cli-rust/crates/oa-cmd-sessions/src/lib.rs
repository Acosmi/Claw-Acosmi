/// Session management commands for Claw Acosmi CLI.
///
/// Provides the `sessions` command that lists session entries from the session
/// store, with support for filtering by activity time, tabular display with
/// color-coded columns, and `--json` output.
///
/// Source: `src/commands/sessions.ts`

mod format;
mod types;

use anyhow::Result;
use clap::Parser;

use oa_agents::defaults::{DEFAULT_CONTEXT_TOKENS, DEFAULT_MODEL, DEFAULT_PROVIDER};
use oa_agents::model_selection::resolve_configured_model_ref;
use oa_config::io::load_config;
use oa_config::sessions::paths::resolve_store_path;
use oa_config::sessions::store::load_session_store;
use oa_terminal::theme::{Theme, is_rich};

use crate::format::{
    format_age_cell, format_flags_cell, format_kind_cell, format_model_cell, format_tokens_cell,
    truncate_key,
};
use crate::types::{SessionRow, to_rows};

/// Column padding constants matching the TS source.
///
/// Source: `src/commands/sessions.ts` - padding constants
const KIND_PAD: usize = 6;
const KEY_PAD: usize = 26;
const AGE_PAD: usize = 9;
const MODEL_PAD: usize = 14;
const TOKENS_PAD: usize = 20;

/// CLI arguments for the `sessions` command.
///
/// Source: `src/commands/sessions.ts` - `sessionsCommand` opts
#[derive(Debug, Parser)]
pub struct SessionsArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,

    /// Override the session store path.
    #[arg(long)]
    pub store: Option<String>,

    /// Filter to sessions active within the last N minutes.
    #[arg(long)]
    pub active: Option<String>,
}

/// Execute the sessions command.
///
/// Lists session entries from the session store with key, kind, age, model,
/// token usage, and flags. Supports `--json` and `--active` filtering.
///
/// Source: `src/commands/sessions.ts` - `sessionsCommand`
pub async fn execute(args: &SessionsArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();

    let resolved = resolve_configured_model_ref(&cfg, DEFAULT_PROVIDER, DEFAULT_MODEL);
    let config_context_tokens = cfg
        .agents
        .as_ref()
        .and_then(|a| a.defaults.as_ref())
        .and_then(|d| d.context_tokens)
        .unwrap_or(DEFAULT_CONTEXT_TOKENS as u64);
    let config_model = resolved.model.clone();

    let store_override = args.store.as_deref().or_else(|| {
        cfg.session
            .as_ref()
            .and_then(|s| s.store.as_deref())
    });
    let store_path = resolve_store_path(store_override, None);
    let store = load_session_store(&store_path);

    // Parse --active filter
    let active_minutes: Option<u64> = if let Some(ref active_str) = args.active {
        let parsed = active_str
            .parse::<u64>()
            .ok()
            .filter(|&v| v > 0);
        if parsed.is_none() {
            eprintln!("--active must be a positive integer (minutes)");
            std::process::exit(1);
        }
        parsed
    } else {
        None
    };

    let now = now_ms();
    let all_rows = to_rows(&store);
    let rows: Vec<SessionRow> = all_rows
        .into_iter()
        .filter(|row| {
            if let Some(minutes) = active_minutes {
                if let Some(updated_at) = row.updated_at {
                    now.saturating_sub(updated_at) <= minutes * 60_000
                } else {
                    false
                }
            } else {
                true
            }
        })
        .collect();

    if args.json {
        let json_output = serde_json::json!({
            "path": store_path.display().to_string(),
            "count": rows.len(),
            "activeMinutes": active_minutes,
            "sessions": rows.iter().map(|r| {
                let model = r.model.as_deref().unwrap_or(&config_model);
                let context_tokens = r.context_tokens.unwrap_or(config_context_tokens);
                let mut row_json = serde_json::to_value(r).unwrap_or(serde_json::Value::Null);
                if let Some(obj) = row_json.as_object_mut() {
                    obj.insert("contextTokens".to_string(), serde_json::json!(context_tokens));
                    obj.insert("model".to_string(), serde_json::json!(model));
                }
                row_json
            }).collect::<Vec<_>>()
        });
        println!("{}", serde_json::to_string_pretty(&json_output).unwrap_or_default());
        return Ok(());
    }

    println!(
        "{}",
        Theme::info(&format!(
            "Session store: {}",
            store_path.display()
        ))
    );
    println!(
        "{}",
        Theme::info(&format!("Sessions listed: {}", rows.len()))
    );
    if let Some(minutes) = active_minutes {
        println!(
            "{}",
            Theme::info(&format!("Filtered to last {minutes} minute(s)"))
        );
    }
    if rows.is_empty() {
        println!("No sessions found.");
        return Ok(());
    }

    let rich = is_rich();

    // Print header
    let header = format!(
        "{} {} {} {} {} {}",
        pad_str("Kind", KIND_PAD),
        pad_str("Key", KEY_PAD),
        pad_str("Age", AGE_PAD),
        pad_str("Model", MODEL_PAD),
        pad_str("Tokens (ctx %)", TOKENS_PAD),
        "Flags",
    );
    println!("{}", if rich { Theme::heading(&header) } else { header });

    // Print rows
    for row in &rows {
        let model = row.model.as_deref().unwrap_or(&config_model);
        let context_tokens = row.context_tokens.unwrap_or(config_context_tokens);
        let input = row.input_tokens.unwrap_or(0);
        let output = row.output_tokens.unwrap_or(0);
        let total = row.total_tokens.unwrap_or(input + output);

        let key_label = pad_str(&truncate_key(&row.key, KEY_PAD), KEY_PAD);
        let key_cell = if rich {
            Theme::accent(&key_label)
        } else {
            key_label
        };

        let line = format!(
            "{} {} {} {} {} {}",
            format_kind_cell(&row.kind, rich, KIND_PAD),
            key_cell,
            format_age_cell(row.updated_at, rich, AGE_PAD),
            format_model_cell(Some(model), rich, MODEL_PAD),
            format_tokens_cell(total, Some(context_tokens), rich, TOKENS_PAD),
            format_flags_cell(row, rich),
        );

        println!("{}", line.trim_end());
    }

    Ok(())
}

/// Pad a string to the given width, right-padding with spaces.
///
/// Source: `src/commands/sessions.ts` - `.padEnd()` calls
fn pad_str(s: &str, width: usize) -> String {
    if s.len() >= width {
        s.to_string()
    } else {
        format!("{s}{}", " ".repeat(width - s.len()))
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pad_str_shorter() {
        assert_eq!(pad_str("abc", 6), "abc   ");
    }

    #[test]
    fn pad_str_exact() {
        assert_eq!(pad_str("abcdef", 6), "abcdef");
    }

    #[test]
    fn pad_str_longer() {
        assert_eq!(pad_str("abcdefgh", 6), "abcdefgh");
    }

    #[test]
    fn now_ms_positive() {
        assert!(now_ms() > 0);
    }
}
