/// Channel setup wizard for onboarding.
///
/// Provides the interactive channel selection and configuration flow during
/// onboarding. Channels include WhatsApp, Telegram, Discord, Slack, etc.
/// Handles both quickstart (single-channel) and advanced (multi-channel) flows.
///
/// Source: `src/commands/onboard-channels.ts`

use std::collections::HashMap;

use anyhow::Result;
use serde::{Deserialize, Serialize};
use tracing::info;

use oa_types::config::OpenAcosmiConfig;

/// Action to take on an already-configured channel.
///
/// Source: `src/commands/onboard-channels.ts` - `ConfiguredChannelAction`
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ConfiguredChannelAction {
    /// Modify existing settings.
    Update,
    /// Disable (keeps config).
    Disable,
    /// Delete config.
    Delete,
    /// Skip (leave as-is).
    Skip,
}

/// Channel status summary from discovery.
///
/// Source: `src/commands/onboard-channels.ts` - `ChannelStatusSummary`
#[derive(Debug, Clone, Default)]
pub struct ChannelStatusSummary {
    /// Status lines for display.
    pub status_lines: Vec<String>,
    /// Number of installed plugins.
    pub installed_plugin_count: usize,
    /// Number of catalog entries available.
    pub catalog_entry_count: usize,
}

/// DM (Direct Message) policy choice.
///
/// Source: `src/commands/onboard-channels.ts` - DM policy prompt
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DmPolicyChoice {
    /// Pairing mode (recommended).
    Pairing,
    /// Allowlist-based access.
    Allowlist,
    /// Open (public inbound DMs).
    Open,
    /// Disabled (ignore DMs).
    Disabled,
}

/// Options for the channel setup wizard.
///
/// Source: `src/commands/onboard-channels.ts` - `SetupChannelsOptions`
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SetupChannelsOptions {
    /// Skip the status note display.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skip_status_note: Option<bool>,
    /// Skip the initial confirm prompt.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skip_confirm: Option<bool>,
    /// Skip DM policy prompt.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skip_dm_policy_prompt: Option<bool>,
    /// Use quickstart defaults (single-channel selection).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub quickstart_defaults: Option<bool>,
    /// Allow disable/delete actions on configured channels.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub allow_disable: Option<bool>,
    /// Prompt for account IDs.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub prompt_account_ids: Option<bool>,
    /// Initial channel selection.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub initial_selection: Option<Vec<String>>,
    /// Channels that should force allow-from configuration.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub force_allow_from_channels: Option<Vec<String>>,
    /// Account ID overrides by channel.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub account_ids: Option<HashMap<String, String>>,
    /// WhatsApp account ID shortcut.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub whatsapp_account_id: Option<String>,
}

/// Default account ID constant.
///
/// Source: `src/routing/session-key.ts` - `DEFAULT_ACCOUNT_ID`
pub const DEFAULT_ACCOUNT_ID: &str = "default";

/// Format an account ID label for display.
///
/// Source: `src/commands/onboard-channels.ts` - `formatAccountLabel`
pub fn format_account_label(account_id: &str) -> String {
    if account_id == DEFAULT_ACCOUNT_ID {
        "default (primary)".to_string()
    } else {
        account_id.to_string()
    }
}

/// Normalize an account ID, returning `None` if empty.
///
/// Source: `src/routing/session-key.ts` - `normalizeAccountId`
pub fn normalize_account_id(value: &str) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

/// Collect channel status information from config.
///
/// In the full implementation, this queries plugin registries and onboarding
/// adapters. For now, returns a summary based on config state.
///
/// Source: `src/commands/onboard-channels.ts` - `collectChannelStatus`
pub fn collect_channel_status(cfg: &OpenAcosmiConfig) -> ChannelStatusSummary {
    let mut status_lines = Vec::new();

    // Check configured channels from config
    if let Some(ref channels) = cfg.channels {
        let json = serde_json::to_value(channels).unwrap_or_default();
        if let Some(obj) = json.as_object() {
            for (key, _value) in obj {
                status_lines.push(format!("{key}: configured"));
            }
        }
    }

    if status_lines.is_empty() {
        status_lines.push("No channels configured yet.".to_string());
    }

    ChannelStatusSummary {
        status_lines,
        installed_plugin_count: 0,
        catalog_entry_count: 0,
    }
}

/// Note channel status to the user.
///
/// Source: `src/commands/onboard-channels.ts` - `noteChannelStatus`
pub fn note_channel_status(cfg: &OpenAcosmiConfig) -> Vec<String> {
    let summary = collect_channel_status(cfg);
    summary.status_lines
}

/// Build the channel primer text explaining how channels work.
///
/// Source: `src/commands/onboard-channels.ts` - `noteChannelPrimer`
pub fn build_channel_primer_text() -> String {
    [
        "DM security: default is pairing; unknown DMs get a pairing code.",
        "Approve with: openacosmi pairing approve <channel> <code>",
        "Public DMs require dmPolicy=\"open\" + allowFrom=[\"*\"].",
        "Multi-user DMs: set session.dmScope=\"per-channel-peer\" (or \"per-account-channel-peer\" for multi-account channels) to isolate sessions.",
        "Docs: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/general/pairing",
    ]
    .join("\n")
}

/// Run the channel setup wizard (non-interactive stub).
///
/// In the full implementation, this drives the interactive channel selection,
/// plugin installation, and DM policy configuration. Returns the updated config.
///
/// Source: `src/commands/onboard-channels.ts` - `setupChannels`
pub async fn setup_channels(
    cfg: OpenAcosmiConfig,
    options: Option<&SetupChannelsOptions>,
) -> Result<OpenAcosmiConfig> {
    let status_lines = note_channel_status(&cfg);

    let skip_note = options
        .and_then(|o| o.skip_status_note)
        .unwrap_or(false);

    if !skip_note && !status_lines.is_empty() {
        for line in &status_lines {
            info!("{line}");
        }
    }

    // In non-interactive mode or when skipConfirm is set, just return config as-is
    let skip_confirm = options
        .and_then(|o| o.skip_confirm)
        .unwrap_or(false);

    if !skip_confirm {
        info!("Channel configuration available via interactive mode.");
    }

    Ok(cfg)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_default_account_label() {
        assert_eq!(format_account_label("default"), "default (primary)");
    }

    #[test]
    fn format_custom_account_label() {
        assert_eq!(format_account_label("my-account"), "my-account");
    }

    #[test]
    fn normalize_empty_account_id() {
        assert!(normalize_account_id("").is_none());
        assert!(normalize_account_id("  ").is_none());
    }

    #[test]
    fn normalize_valid_account_id() {
        assert_eq!(normalize_account_id("test"), Some("test".to_string()));
        assert_eq!(
            normalize_account_id("  trimmed  "),
            Some("trimmed".to_string())
        );
    }

    #[test]
    fn collect_status_empty_config() {
        let cfg = OpenAcosmiConfig::default();
        let summary = collect_channel_status(&cfg);
        assert!(!summary.status_lines.is_empty());
    }

    #[test]
    fn channel_primer_text_not_empty() {
        let text = build_channel_primer_text();
        assert!(text.contains("pairing"));
        assert!(text.contains("DM security"));
    }

    #[test]
    fn dm_policy_choice_serialize() {
        let json = serde_json::to_string(&DmPolicyChoice::Pairing).expect("serialize");
        assert_eq!(json, "\"pairing\"");
    }

    #[test]
    fn configured_action_serialize() {
        let json = serde_json::to_string(&ConfiguredChannelAction::Update).expect("serialize");
        assert_eq!(json, "\"update\"");
    }

    #[test]
    fn setup_channels_options_default() {
        let opts = SetupChannelsOptions::default();
        assert!(opts.skip_status_note.is_none());
        assert!(opts.quickstart_defaults.is_none());
        assert!(opts.initial_selection.is_none());
    }

    #[test]
    fn default_account_id_constant() {
        assert_eq!(DEFAULT_ACCOUNT_ID, "default");
    }
}
