/// Configuration guard for ensuring valid config before command execution.
///
/// Validates that the Claw Acosmi configuration file exists and is parseable
/// before allowing a command to proceed. Certain safe commands (e.g.,
/// `doctor`, `health`, `status`) bypass validation.
///
/// Source: `src/cli/program/config-guard.ts`

use anyhow::{bail, Result};
use oa_config::paths::resolve_config_path;
use oa_terminal::theme::{is_rich, Theme};

use crate::command_format::format_cli_command;

/// Commands that are allowed to run even when the config is invalid or absent.
///
/// Source: `src/cli/program/config-guard.ts` – `ALLOWED_INVALID_COMMANDS`
const SAFE_COMMANDS: &[&str] = &["doctor", "logs", "health", "help", "status"];

/// Gateway subcommands that are allowed to run even when the config is invalid.
///
/// Source: `src/cli/program/config-guard.ts` – `ALLOWED_INVALID_GATEWAY_SUBCOMMANDS`
const SAFE_GATEWAY_SUBCOMMANDS: &[&str] = &[
    "status", "probe", "health", "discover", "call", "install", "uninstall", "start", "stop",
    "restart",
];

/// Check whether a command name is in the safe list (bypasses config validation).
///
/// Source: `src/cli/program/config-guard.ts` – `ALLOWED_INVALID_COMMANDS`
pub fn is_safe_command(cmd: &str) -> bool {
    SAFE_COMMANDS.contains(&cmd)
}

/// Check whether a gateway subcommand is in the safe list.
///
/// Source: `src/cli/program/config-guard.ts` – `ALLOWED_INVALID_GATEWAY_SUBCOMMANDS`
pub fn is_safe_gateway_subcommand(sub: &str) -> bool {
    SAFE_GATEWAY_SUBCOMMANDS.contains(&sub)
}

/// Determine whether the given command path should skip config validation.
///
/// A command path is `&["gateway", "status"]` for `openacosmi gateway status`.
///
/// Source: `src/cli/program/config-guard.ts` – `allowInvalid` logic
pub fn should_skip_validation(command_path: &[&str]) -> bool {
    let primary = match command_path.first() {
        Some(cmd) => *cmd,
        None => return false,
    };

    if is_safe_command(primary) {
        return true;
    }

    if primary == "gateway" {
        if let Some(sub) = command_path.get(1) {
            return is_safe_gateway_subcommand(sub);
        }
    }

    false
}

/// Ensure the configuration is ready for a command to execute.
///
/// Checks that the config file exists on disk. If it does not exist
/// and the command is not in the safe list, returns an error with
/// guidance to run `openacosmi doctor --fix`.
///
/// For the full validation path (parsing, schema checks) the caller
/// should use `oa_config::io::load_config` or
/// `oa_config::io::read_config_file_snapshot`. This guard performs a
/// lightweight existence check to fail fast with a helpful message.
///
/// Source: `src/cli/program/config-guard.ts` – `ensureConfigReady`
pub fn ensure_config_ready(command_path: &[&str]) -> Result<()> {
    let config_path = resolve_config_path();

    // If the config file exists, this guard passes.
    // Full validation is deferred to config loading.
    if config_path.exists() {
        return Ok(());
    }

    // Safe commands can run without config
    if should_skip_validation(command_path) {
        return Ok(());
    }

    let rich = is_rich();
    let path_display = config_path.display().to_string();

    let heading = if rich {
        Theme::heading("Config not found")
    } else {
        "Config not found".to_string()
    };
    let file_label = if rich {
        format!("{} {}", Theme::muted("File:"), Theme::muted(&path_display))
    } else {
        format!("File: {path_display}")
    };
    let fix_cmd = format_cli_command("openacosmi doctor --fix");
    let run_label = if rich {
        format!("{} {}", Theme::muted("Run:"), Theme::command(&fix_cmd))
    } else {
        format!("Run: {fix_cmd}")
    };

    eprintln!("{heading}");
    eprintln!("{file_label}");
    eprintln!();
    eprintln!("{run_label}");

    bail!(
        "Config file not found at {}. Run `openacosmi doctor --fix` to create one.",
        path_display
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn safe_commands_recognized() {
        assert!(is_safe_command("doctor"));
        assert!(is_safe_command("health"));
        assert!(is_safe_command("status"));
        assert!(is_safe_command("logs"));
        assert!(is_safe_command("help"));
    }

    #[test]
    fn unsafe_commands_recognized() {
        assert!(!is_safe_command("deploy"));
        assert!(!is_safe_command("configure"));
        assert!(!is_safe_command("sessions"));
    }

    #[test]
    fn safe_gateway_subcommands_recognized() {
        assert!(is_safe_gateway_subcommand("status"));
        assert!(is_safe_gateway_subcommand("probe"));
        assert!(is_safe_gateway_subcommand("health"));
        assert!(is_safe_gateway_subcommand("install"));
        assert!(is_safe_gateway_subcommand("start"));
        assert!(is_safe_gateway_subcommand("stop"));
        assert!(is_safe_gateway_subcommand("restart"));
    }

    #[test]
    fn unsafe_gateway_subcommands_recognized() {
        assert!(!is_safe_gateway_subcommand("configure"));
        assert!(!is_safe_gateway_subcommand("deploy"));
    }

    #[test]
    fn skip_validation_for_safe_commands() {
        assert!(should_skip_validation(&["doctor"]));
        assert!(should_skip_validation(&["health"]));
        assert!(should_skip_validation(&["status"]));
        assert!(should_skip_validation(&["logs"]));
        assert!(should_skip_validation(&["help"]));
    }

    #[test]
    fn skip_validation_for_safe_gateway_subcommands() {
        assert!(should_skip_validation(&["gateway", "status"]));
        assert!(should_skip_validation(&["gateway", "health"]));
        assert!(should_skip_validation(&["gateway", "install"]));
    }

    #[test]
    fn no_skip_for_unsafe_commands() {
        assert!(!should_skip_validation(&["deploy"]));
        assert!(!should_skip_validation(&["configure"]));
        assert!(!should_skip_validation(&["gateway", "configure"]));
    }

    #[test]
    fn no_skip_for_empty_path() {
        assert!(!should_skip_validation(&[]));
    }
}
