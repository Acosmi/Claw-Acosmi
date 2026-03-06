/// Setup command implementation.
///
/// Initializes or updates the workspace, configuration file, and session
/// directories for the Claw Acosmi CLI.
///
/// Source: `src/commands/setup.ts`

use std::path::PathBuf;

use anyhow::{Context, Result};
use tracing::info;

use oa_config::io::write_config_file;
use oa_config::paths::resolve_config_path;
use oa_config::sessions::paths::resolve_session_transcripts_dir;
use oa_types::agent_defaults::AgentDefaultsConfig;
use oa_types::agents::AgentsConfig;
use oa_types::config::OpenAcosmiConfig;

use crate::cleanup_utils::shorten_home_path;

/// The default workspace directory name under the state directory.
///
/// Source: `src/commands/setup.ts` - `DEFAULT_AGENT_WORKSPACE_DIR`
const DEFAULT_AGENT_WORKSPACE_DIR: &str = "~/openacosmi-workspace";

/// Options for the setup command.
///
/// Source: `src/commands/setup.ts` - `opts` parameter
#[derive(Debug, Clone, Default)]
pub struct SetupOptions {
    /// Optional workspace directory path override.
    pub workspace: Option<String>,
}

/// Read the config file as raw JSON5 and parse it.
///
/// Returns the parsed config and whether the file existed on disk.
///
/// Source: `src/commands/setup.ts` - `readConfigFileRaw`
async fn read_config_file_raw(config_path: &std::path::Path) -> (bool, OpenAcosmiConfig) {
    match tokio::fs::read_to_string(config_path).await {
        Ok(raw) => {
            let parsed: OpenAcosmiConfig = json5::from_str(&raw).unwrap_or_default();
            (true, parsed)
        }
        Err(_) => (false, OpenAcosmiConfig::default()),
    }
}

/// Resolve the workspace path, expanding `~` to the home directory.
fn resolve_workspace_path(input: &str) -> PathBuf {
    let trimmed = input.trim();
    if let Some(rest) = trimmed.strip_prefix('~') {
        let home = dirs::home_dir().unwrap_or_else(|| PathBuf::from("."));
        let rest = rest.strip_prefix('/').unwrap_or(rest);
        home.join(rest)
    } else {
        PathBuf::from(trimmed)
    }
}

/// Run the setup command.
///
/// Ensures the config file exists with a workspace path, creates the
/// workspace directory, and ensures the sessions directory exists.
///
/// Source: `src/commands/setup.ts` - `setupCommand`
pub async fn setup_command(opts: SetupOptions) -> Result<()> {
    let desired_workspace = opts
        .workspace
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty());

    let config_path = resolve_config_path();
    let (exists, cfg) = read_config_file_raw(&config_path).await;

    let defaults = cfg
        .agents
        .as_ref()
        .and_then(|a| a.defaults.as_ref())
        .cloned()
        .unwrap_or_default();

    let workspace = desired_workspace
        .map(String::from)
        .or(defaults.workspace.clone())
        .unwrap_or_else(|| DEFAULT_AGENT_WORKSPACE_DIR.to_owned());

    let next = OpenAcosmiConfig {
        agents: Some(AgentsConfig {
            defaults: Some(AgentDefaultsConfig {
                workspace: Some(workspace.clone()),
                ..defaults
            }),
            ..cfg.agents.unwrap_or_default()
        }),
        ..cfg
    };

    let config_path_display = shorten_home_path(&config_path.display().to_string());

    if !exists || defaults.workspace.as_deref() != Some(&workspace) {
        write_config_file(&next)
            .await
            .context("Failed to write config file")?;

        if !exists {
            info!("Wrote {config_path_display}");
        } else {
            info!("Config updated: {config_path_display} (set agents.defaults.workspace)");
        }
    } else {
        info!("Config OK: {config_path_display}");
    }

    // Ensure workspace directory exists.
    let ws_path = resolve_workspace_path(&workspace);
    tokio::fs::create_dir_all(&ws_path)
        .await
        .with_context(|| format!("Failed to create workspace: {}", ws_path.display()))?;
    info!("Workspace OK: {}", shorten_home_path(&ws_path.display().to_string()));

    // Ensure sessions directory exists.
    let sessions_dir = resolve_session_transcripts_dir();
    tokio::fs::create_dir_all(&sessions_dir)
        .await
        .with_context(|| format!("Failed to create sessions dir: {}", sessions_dir.display()))?;
    info!(
        "Sessions OK: {}",
        shorten_home_path(&sessions_dir.display().to_string())
    );

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_workspace_dir_value() {
        assert_eq!(DEFAULT_AGENT_WORKSPACE_DIR, "~/openacosmi-workspace");
    }

    #[test]
    fn resolve_workspace_path_tilde() {
        let path = resolve_workspace_path("~/my-workspace");
        let path_str = path.display().to_string();
        assert!(path_str.contains("my-workspace"));
        assert!(!path_str.starts_with('~'));
    }

    #[test]
    fn resolve_workspace_path_absolute() {
        let path = resolve_workspace_path("/tmp/workspace");
        assert_eq!(path.display().to_string(), "/tmp/workspace");
    }

    #[test]
    fn resolve_workspace_path_trims() {
        let path = resolve_workspace_path("  /tmp/test  ");
        assert_eq!(path.display().to_string(), "/tmp/test");
    }

    #[test]
    fn default_options() {
        let opts = SetupOptions::default();
        assert!(opts.workspace.is_none());
    }
}
