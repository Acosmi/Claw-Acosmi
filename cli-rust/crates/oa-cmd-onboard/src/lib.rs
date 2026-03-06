/// Onboarding wizard commands for Claw Acosmi CLI.
///
/// Provides the `onboard` command with both interactive and non-interactive
/// flows for initial setup of the gateway, auth providers, channels, skills,
/// and workspace configuration.
///
/// Source: `src/commands/onboard*.ts`

pub mod types;
pub mod helpers;
pub mod auth;
pub mod channels;
pub mod hooks;
pub mod interactive;
pub mod non_interactive;
pub mod remote;
pub mod skills;

use anyhow::Result;
use tracing::warn;

use oa_config::io::read_config_file_snapshot;
use oa_types::config::OpenAcosmiConfig;

use crate::helpers::{handle_reset, resolve_user_path, DEFAULT_WORKSPACE};
use crate::types::OnboardOptions;

/// Execute the onboard command.
///
/// Normalizes deprecated auth choices, validates risk acknowledgement for
/// non-interactive mode, handles reset, and dispatches to the appropriate
/// interactive or non-interactive flow.
///
/// Source: `src/commands/onboard.ts` - `onboardCommand`
pub async fn execute(opts: OnboardOptions) -> Result<()> {
    // Normalize deprecated auth choices
    let auth_choice = normalize_auth_choice(opts.auth_choice.as_deref());

    let mut normalized_opts = opts.clone();
    if let Some(ref normalized) = auth_choice {
        normalized_opts.auth_choice = Some(normalized.clone());
    }

    // Non-interactive mode requires explicit risk acknowledgement
    if normalized_opts.non_interactive.unwrap_or(false)
        && !normalized_opts.accept_risk.unwrap_or(false)
    {
        anyhow::bail!(
            "Non-interactive onboarding requires explicit risk acknowledgement.\n\
             Read: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/general/security\n\
             Re-run with: openacosmi onboard --non-interactive --accept-risk ..."
        );
    }

    // Handle reset if requested
    if normalized_opts.reset.unwrap_or(false) {
        let snapshot = read_config_file_snapshot().await?;
        let base_config = if snapshot.valid {
            snapshot.config
        } else {
            OpenAcosmiConfig::default()
        };
        let workspace_default = normalized_opts
            .workspace
            .as_deref()
            .or(base_config
                .agents
                .as_ref()
                .and_then(|a| a.defaults.as_ref())
                .and_then(|d| d.workspace.as_deref()))
            .unwrap_or(DEFAULT_WORKSPACE);
        handle_reset(
            types::ResetScope::Full,
            &resolve_user_path(workspace_default),
        )
        .await?;
    }

    // Platform warning for Windows
    if cfg!(target_os = "windows") {
        warn!(
            "Windows detected -- OpenAcosmi runs great on WSL2!\n\
             Native Windows might be trickier.\n\
             Quick setup: wsl --install (one command, one reboot)\n\
             Guide: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/platforms/windows"
        );
    }

    // Dispatch to the appropriate flow
    if normalized_opts.non_interactive.unwrap_or(false) {
        non_interactive::run_non_interactive_onboarding(&normalized_opts).await
    } else {
        interactive::run_interactive_onboarding(&normalized_opts).await
    }
}

/// Normalize deprecated auth choice aliases to their canonical forms.
///
/// Source: `src/commands/onboard.ts` - auth choice normalization logic
fn normalize_auth_choice(choice: Option<&str>) -> Option<String> {
    match choice {
        Some("oauth") | Some("claude-cli") => Some("setup-token".to_string()),
        Some("codex-cli") => Some("openai-codex".to_string()),
        Some("manual") => Some("advanced".to_string()),
        Some(other) => Some(other.to_string()),
        None => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalize_oauth_to_setup_token() {
        assert_eq!(
            normalize_auth_choice(Some("oauth")),
            Some("setup-token".to_string())
        );
    }

    #[test]
    fn normalize_claude_cli_to_setup_token() {
        assert_eq!(
            normalize_auth_choice(Some("claude-cli")),
            Some("setup-token".to_string())
        );
    }

    #[test]
    fn normalize_codex_cli_to_openai_codex() {
        assert_eq!(
            normalize_auth_choice(Some("codex-cli")),
            Some("openai-codex".to_string())
        );
    }

    #[test]
    fn normalize_none_returns_none() {
        assert!(normalize_auth_choice(None).is_none());
    }

    #[test]
    fn normalize_other_passes_through() {
        assert_eq!(
            normalize_auth_choice(Some("token")),
            Some("token".to_string())
        );
        assert_eq!(
            normalize_auth_choice(Some("apiKey")),
            Some("apiKey".to_string())
        );
    }
}
