/// Daemon directory path resolution for Claw Acosmi services.
///
/// Resolves state directories for the gateway and node services,
/// supporting profile-based suffixes and environment variable overrides.
///
/// Source: `src/daemon/paths.ts`

use std::path::{Path, PathBuf};

use crate::constants::resolve_gateway_profile_suffix;

/// Resolve the gateway state directory.
///
/// Uses the `OPENACOSMI_STATE_DIR` environment variable if set (supports `~` expansion),
/// otherwise falls back to `~/.openacosmi{profile_suffix}` where the suffix is derived
/// from `OPENACOSMI_PROFILE`.
///
/// The `env` parameter is a closure that looks up environment variable values,
/// allowing tests to provide custom environments.
///
/// Source: `src/daemon/paths.ts` - `resolveGatewayStateDir`
pub fn resolve_gateway_state_dir<F>(env: F) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    // Check for override
    if let Some(override_dir) = env("OPENACOSMI_STATE_DIR") {
        let trimmed = override_dir.trim().to_string();
        if !trimmed.is_empty() {
            return resolve_user_path(&trimmed, &env);
        }
    }

    let home = resolve_home_dir(&env)?;
    let suffix = resolve_gateway_profile_suffix(env("OPENACOSMI_PROFILE").as_deref());
    Ok(home.join(format!(".openacosmi{suffix}")))
}

/// Resolve the node service state directory relative to a base directory.
///
/// Returns `{base_dir}/node` for storing node-specific runtime state.
///
/// Source: `src/daemon/paths.ts` (inferred from `resolveNodeStateDir`)
pub fn resolve_node_state_dir(base_dir: &Path) -> PathBuf {
    base_dir.join("node")
}

/// Resolve the user data directory for daemon state.
///
/// On macOS, returns `~/Library/Application Support/openacosmi`.
/// On Linux, follows XDG conventions (`$XDG_DATA_HOME/openacosmi` or `~/.local/share/openacosmi`).
/// On other platforms, falls back to `dirs::data_dir()`.
///
/// Source: `src/daemon/paths.ts` (inferred from user-data patterns)
pub fn resolve_user_data_dir() -> anyhow::Result<PathBuf> {
    let data_dir =
        dirs::data_dir().ok_or_else(|| anyhow::anyhow!("Could not determine user data directory"))?;
    Ok(data_dir.join("openacosmi"))
}

/// Resolve the home directory from environment variables.
///
/// Checks `HOME` first (Unix convention), then `USERPROFILE` (Windows convention).
///
/// Source: `src/daemon/paths.ts` - `resolveHomeDir`
pub fn resolve_home_dir<F>(env: &F) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    if let Some(home) = env("HOME") {
        let trimmed = home.trim();
        if !trimmed.is_empty() {
            return Ok(PathBuf::from(trimmed));
        }
    }
    if let Some(userprofile) = env("USERPROFILE") {
        let trimmed = userprofile.trim();
        if !trimmed.is_empty() {
            return Ok(PathBuf::from(trimmed));
        }
    }
    Err(anyhow::anyhow!("Missing HOME"))
}

/// Resolve a user-provided path, expanding `~` to the home directory.
///
/// Source: `src/daemon/paths.ts` - `resolveUserPathWithHome`
fn resolve_user_path<F>(input: &str, env: &F) -> anyhow::Result<PathBuf>
where
    F: Fn(&str) -> Option<String>,
{
    let trimmed = input.trim();
    if trimmed.is_empty() {
        return Ok(PathBuf::from(trimmed));
    }

    if trimmed.starts_with('~') {
        let home = resolve_home_dir(env)?;
        let rest = trimmed.strip_prefix('~').unwrap_or("");
        let rest = rest.strip_prefix('/').or_else(|| rest.strip_prefix('\\')).unwrap_or(rest);
        if rest.is_empty() {
            return Ok(home);
        }
        return Ok(home.join(rest));
    }

    // Check for Windows absolute paths (e.g., C:\... or \\...)
    if is_windows_absolute_path(trimmed) {
        return Ok(PathBuf::from(trimmed));
    }

    // Resolve relative paths against current directory
    Ok(std::path::absolute(PathBuf::from(trimmed))
        .unwrap_or_else(|_| PathBuf::from(trimmed)))
}

/// Check if a path looks like a Windows absolute path.
fn is_windows_absolute_path(path: &str) -> bool {
    let bytes = path.as_bytes();
    // C:\... or C:/...
    if bytes.len() >= 3 && bytes[0].is_ascii_alphabetic() && bytes[1] == b':' && (bytes[2] == b'\\' || bytes[2] == b'/') {
        return true;
    }
    // UNC path: \\...
    if bytes.len() >= 2 && bytes[0] == b'\\' && bytes[1] == b'\\' {
        return true;
    }
    false
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn make_env(entries: &[(&str, &str)]) -> impl Fn(&str) -> Option<String> {
        let map: HashMap<String, String> = entries
            .iter()
            .map(|(k, v)| (k.to_string(), v.to_string()))
            .collect();
        move |key: &str| map.get(key).cloned()
    }

    #[test]
    fn default_state_dir_with_home() {
        let env = make_env(&[("HOME", "/Users/test")]);
        let result = resolve_gateway_state_dir(env).expect("should resolve");
        assert_eq!(result, PathBuf::from("/Users/test/.openacosmi"));
    }

    #[test]
    fn state_dir_with_profile_suffix() {
        let env = make_env(&[("HOME", "/Users/test"), ("OPENACOSMI_PROFILE", "rescue")]);
        let result = resolve_gateway_state_dir(env).expect("should resolve");
        assert_eq!(result, PathBuf::from("/Users/test/.openacosmi-rescue"));
    }

    #[test]
    fn state_dir_treats_default_profile_as_base() {
        let env = make_env(&[("HOME", "/Users/test"), ("OPENACOSMI_PROFILE", "Default")]);
        let result = resolve_gateway_state_dir(env).expect("should resolve");
        assert_eq!(result, PathBuf::from("/Users/test/.openacosmi"));
    }

    #[test]
    fn state_dir_with_override() {
        let env = make_env(&[
            ("HOME", "/Users/test"),
            ("OPENACOSMI_STATE_DIR", "/var/lib/openacosmi"),
        ]);
        let result = resolve_gateway_state_dir(env).expect("should resolve");
        assert_eq!(result, PathBuf::from("/var/lib/openacosmi"));
    }

    #[test]
    fn state_dir_expands_tilde() {
        let env = make_env(&[
            ("HOME", "/Users/test"),
            ("OPENACOSMI_STATE_DIR", "~/openacosmi-state"),
        ]);
        let result = resolve_gateway_state_dir(env).expect("should resolve");
        assert_eq!(result, PathBuf::from("/Users/test/openacosmi-state"));
    }

    #[test]
    fn state_dir_preserves_windows_absolute_paths() {
        let env = make_env(&[("OPENACOSMI_STATE_DIR", "C:\\State\\openacosmi")]);
        let result = resolve_gateway_state_dir(env).expect("should resolve");
        assert_eq!(result, PathBuf::from("C:\\State\\openacosmi"));
    }

    #[test]
    fn node_state_dir_appends_node() {
        let base = PathBuf::from("/tmp/state");
        assert_eq!(resolve_node_state_dir(&base), PathBuf::from("/tmp/state/node"));
    }

    #[test]
    fn user_data_dir_resolves() {
        // This test just ensures the function does not panic on the host system.
        let result = resolve_user_data_dir();
        assert!(result.is_ok());
        let path = result.expect("should resolve");
        assert!(path.to_string_lossy().contains("openacosmi"));
    }

    #[test]
    fn resolve_home_dir_prefers_home() {
        let env = make_env(&[("HOME", "/home/user"), ("USERPROFILE", "C:\\Users\\user")]);
        let result = resolve_home_dir(&env).expect("should resolve");
        assert_eq!(result, PathBuf::from("/home/user"));
    }

    #[test]
    fn resolve_home_dir_falls_back_to_userprofile() {
        let env = make_env(&[("USERPROFILE", "C:\\Users\\user")]);
        let result = resolve_home_dir(&env).expect("should resolve");
        assert_eq!(result, PathBuf::from("C:\\Users\\user"));
    }

    #[test]
    fn resolve_home_dir_error_when_missing() {
        let env = make_env(&[]);
        let result = resolve_home_dir(&env);
        assert!(result.is_err());
    }
}
