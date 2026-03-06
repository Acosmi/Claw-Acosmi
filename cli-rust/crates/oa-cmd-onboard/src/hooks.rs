/// Hook setup wizard for onboarding.
///
/// Provides the interactive hook discovery and enablement flow during
/// onboarding. Hooks automate actions when agent commands are issued,
/// e.g., saving session context to memory on `/new`.
///
/// Source: `src/commands/onboard-hooks.ts`

use anyhow::Result;
use tracing::info;

use oa_types::config::OpenAcosmiConfig;
use oa_types::hooks::{HookConfig, InternalHooksConfig};

/// Information about an eligible hook for display during onboarding.
///
/// Source: `src/commands/onboard-hooks.ts` - hook discovery loop
#[derive(Debug, Clone)]
pub struct EligibleHookInfo {
    /// Hook name identifier.
    pub name: String,
    /// Optional description for display.
    pub description: Option<String>,
    /// Whether this hook is currently enabled.
    pub enabled: bool,
}

/// Enable a set of hooks in the config by name.
///
/// Creates or merges `hooks.internal.entries` with `enabled: true` for each
/// selected hook name, and sets `hooks.internal.enabled = true`.
///
/// Source: `src/commands/onboard-hooks.ts` - hook enable logic
pub fn enable_hooks(cfg: OpenAcosmiConfig, hook_names: &[String]) -> OpenAcosmiConfig {
    if hook_names.is_empty() {
        return cfg;
    }

    let existing_hooks = cfg.hooks.clone().unwrap_or_default();
    let existing_internal = existing_hooks.internal.clone().unwrap_or_default();
    let mut entries = existing_internal.entries.clone().unwrap_or_default();

    for name in hook_names {
        let existing_entry = entries.get(name).cloned().unwrap_or_default();
        let entry = HookConfig {
            enabled: Some(true),
            ..existing_entry
        };
        entries.insert(name.clone(), entry);
    }

    let internal = InternalHooksConfig {
        enabled: Some(true),
        entries: Some(entries),
        ..existing_internal
    };

    OpenAcosmiConfig {
        hooks: Some(oa_types::hooks::HooksConfig {
            internal: Some(internal),
            ..existing_hooks
        }),
        ..cfg
    }
}

/// Build the hooks primer text for display.
///
/// Source: `src/commands/onboard-hooks.ts` - primer note
pub fn build_hooks_primer_text() -> String {
    [
        "Hooks let you automate actions when agent commands are issued.",
        "Example: Save session context to memory when you issue /new.",
        "",
        "Learn more: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/cli/hooks",
    ]
    .join("\n")
}

/// Build the hooks configured summary text.
///
/// Source: `src/commands/onboard-hooks.ts` - configured note
pub fn build_hooks_configured_text(selected: &[String]) -> String {
    let count = selected.len();
    let plural = if count > 1 { "s" } else { "" };
    let names = selected.join(", ");
    [
        &format!("Enabled {count} hook{plural}: {names}"),
        "",
        "You can manage hooks later with:",
        "  openacosmi hooks list",
        "  openacosmi hooks enable <name>",
        "  openacosmi hooks disable <name>",
    ]
    .join("\n")
}

/// Run the internal hooks setup wizard (non-interactive stub).
///
/// In the full implementation, this discovers eligible hooks from the workspace
/// and prompts the user to enable them. Returns the updated config.
///
/// Source: `src/commands/onboard-hooks.ts` - `setupInternalHooks`
pub async fn setup_internal_hooks(cfg: OpenAcosmiConfig) -> Result<OpenAcosmiConfig> {
    info!("Hook configuration available via interactive mode.");
    Ok(cfg)
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;

    use super::*;

    #[test]
    fn enable_hooks_empty_list() {
        let cfg = OpenAcosmiConfig::default();
        let result = enable_hooks(cfg.clone(), &[]);
        // Should return config unchanged
        assert!(result.hooks.is_none());
    }

    #[test]
    fn enable_hooks_creates_entries() {
        let cfg = OpenAcosmiConfig::default();
        let names = vec!["memory-save".to_string(), "auto-tag".to_string()];
        let result = enable_hooks(cfg, &names);

        let internal = result
            .hooks
            .as_ref()
            .and_then(|h| h.internal.as_ref())
            .expect("internal hooks");
        assert_eq!(internal.enabled, Some(true));

        let entries = internal.entries.as_ref().expect("entries");
        assert_eq!(entries.len(), 2);
        assert_eq!(
            entries.get("memory-save").and_then(|e| e.enabled),
            Some(true)
        );
        assert_eq!(
            entries.get("auto-tag").and_then(|e| e.enabled),
            Some(true)
        );
    }

    #[test]
    fn enable_hooks_preserves_existing() {
        let mut cfg = OpenAcosmiConfig::default();
        let mut existing_entries = HashMap::new();
        existing_entries.insert(
            "existing-hook".to_string(),
            HookConfig {
                enabled: Some(true),
                ..Default::default()
            },
        );
        cfg.hooks = Some(oa_types::hooks::HooksConfig {
            internal: Some(InternalHooksConfig {
                enabled: Some(true),
                entries: Some(existing_entries),
                ..Default::default()
            }),
            ..Default::default()
        });

        let names = vec!["new-hook".to_string()];
        let result = enable_hooks(cfg, &names);

        let entries = result
            .hooks
            .as_ref()
            .and_then(|h| h.internal.as_ref())
            .and_then(|i| i.entries.as_ref())
            .expect("entries");
        assert_eq!(entries.len(), 2);
        assert!(entries.contains_key("existing-hook"));
        assert!(entries.contains_key("new-hook"));
    }

    #[test]
    fn hooks_primer_text_content() {
        let text = build_hooks_primer_text();
        assert!(text.contains("Hooks let you automate"));
        assert!(text.contains("github.com/Acosmi/Claw-Acosmi"));
    }

    #[test]
    fn hooks_configured_text_singular() {
        let text = build_hooks_configured_text(&["memory-save".to_string()]);
        assert!(text.contains("Enabled 1 hook:"));
        assert!(text.contains("memory-save"));
    }

    #[test]
    fn hooks_configured_text_plural() {
        let text = build_hooks_configured_text(&[
            "hook-a".to_string(),
            "hook-b".to_string(),
        ]);
        assert!(text.contains("Enabled 2 hooks:"));
        assert!(text.contains("hook-a, hook-b"));
    }

    #[test]
    fn eligible_hook_info_fields() {
        let hook = EligibleHookInfo {
            name: "test-hook".to_string(),
            description: Some("A test hook".to_string()),
            enabled: false,
        };
        assert_eq!(hook.name, "test-hook");
        assert!(!hook.enabled);
    }
}
