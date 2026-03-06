/// Home directory resolution utilities.
///
/// Resolves the effective home directory from environment variables and
/// system defaults, following the precedence chain:
/// `OPENACOSMI_HOME` > `HOME` > `USERPROFILE` > `dirs::home_dir()`.
///
/// Source: `src/infra/home-dir.ts`

use std::path::{Path, PathBuf};

/// Trim a string and return `None` if the result is empty.
fn normalize(value: Option<String>) -> Option<String> {
    let trimmed = value?.trim().to_owned();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed)
    }
}

/// Attempt to get the home directory from the `dirs` crate, returning `None` on failure.
fn dirs_home_dir_str() -> Option<String> {
    dirs::home_dir().and_then(|p| p.to_str().map(String::from))
}

/// Resolve the raw (un-canonicalized) home directory from environment variables.
///
/// Precedence:
/// 1. `OPENACOSMI_HOME` (with `~` expansion using fallback sources)
/// 2. `HOME`
/// 3. `USERPROFILE`
/// 4. `dirs::home_dir()`
fn resolve_raw_home_dir() -> Option<String> {
    // 1. Check OPENACOSMI_HOME
    let explicit_home = normalize(std::env::var("OPENACOSMI_HOME").ok());
    if let Some(ref explicit) = explicit_home {
        if explicit == "~" || explicit.starts_with("~/") || explicit.starts_with("~\\") {
            // Need a fallback home to expand the tilde
            let fallback_home = normalize(std::env::var("HOME").ok())
                .or_else(|| normalize(std::env::var("USERPROFILE").ok()))
                .or_else(dirs_home_dir_str);
            if let Some(fallback) = fallback_home {
                return Some(replace_tilde_prefix(explicit, &fallback));
            }
            return None;
        }
        return explicit_home;
    }

    // 2. Check HOME
    let env_home = normalize(std::env::var("HOME").ok());
    if env_home.is_some() {
        return env_home;
    }

    // 3. Check USERPROFILE
    let user_profile = normalize(std::env::var("USERPROFILE").ok());
    if user_profile.is_some() {
        return user_profile;
    }

    // 4. Fall back to dirs::home_dir()
    dirs_home_dir_str()
}

/// Replace a leading `~` in a path string with the given home directory.
fn replace_tilde_prefix(input: &str, home: &str) -> String {
    if input == "~" {
        home.to_owned()
    } else if input.starts_with("~/") || input.starts_with("~\\") {
        format!("{}{}", home, &input[1..])
    } else {
        input.to_owned()
    }
}

/// Resolve the effective home directory.
///
/// Returns the resolved (absolute) home directory path by checking
/// environment variables in order of precedence:
/// `OPENACOSMI_HOME` > `HOME` > `USERPROFILE` > `dirs::home_dir()`.
///
/// Returns `None` if no home directory can be determined.
pub fn resolve_effective_home_dir() -> Option<String> {
    resolve_raw_home_dir().map(|raw| {
        let path = PathBuf::from(&raw);
        if path.is_absolute() {
            raw
        } else {
            std::env::current_dir()
                .ok()
                .map(|cwd| cwd.join(&path).to_string_lossy().to_string())
                .unwrap_or(raw)
        }
    })
}

/// Resolve the home directory, falling back to the current working directory.
///
/// This function always returns a valid path string. If no home directory
/// can be determined from environment variables or system defaults, it
/// returns the current working directory.
pub fn resolve_required_home_dir() -> String {
    resolve_effective_home_dir().unwrap_or_else(|| {
        std::env::current_dir()
            .unwrap_or_else(|_| PathBuf::from("."))
            .to_string_lossy()
            .to_string()
    })
}

/// Expand a leading `~` prefix in a path string to the user's home directory.
///
/// If the input does not start with `~`, it is returned unchanged.
/// If the home directory cannot be determined, the input is returned unchanged.
///
/// # Examples
///
/// ```
/// // With HOME set to /home/user:
/// // expand_home_prefix("~/Documents") -> "/home/user/Documents"
/// // expand_home_prefix("/absolute/path") -> "/absolute/path"
/// ```
pub fn expand_home_prefix(input: &str) -> String {
    if !input.starts_with('~') {
        return input.to_owned();
    }
    match resolve_effective_home_dir() {
        Some(home) => replace_tilde_prefix(input, &home),
        None => input.to_owned(),
    }
}

/// Resolve the Claw Acosmi state directory.
///
/// Checks `OPENACOSMI_STATE_DIR` and `CLAWDBOT_STATE_DIR` environment variables
/// first, then falls back to `~/.openacosmi`.
pub fn resolve_state_dir() -> String {
    let state_override = normalize(std::env::var("OPENACOSMI_STATE_DIR").ok())
        .or_else(|| normalize(std::env::var("CLAWDBOT_STATE_DIR").ok()));

    if let Some(dir) = state_override {
        return expand_home_prefix(&dir);
    }

    let home = resolve_required_home_dir();
    Path::new(&home).join(".openacosmi").to_string_lossy().to_string()
}

/// Resolve the Claw Acosmi config directory.
///
/// This is the same as the state directory - used for loading config files
/// and `.env` files from `~/.openacosmi/`.
pub fn resolve_config_dir() -> String {
    resolve_state_dir()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_normalize_empty() {
        assert_eq!(normalize(None), None);
        assert_eq!(normalize(Some(String::new())), None);
        assert_eq!(normalize(Some("  ".to_owned())), None);
    }

    #[test]
    fn test_normalize_non_empty() {
        assert_eq!(normalize(Some("hello".to_owned())), Some("hello".to_owned()));
        assert_eq!(normalize(Some("  hello  ".to_owned())), Some("hello".to_owned()));
    }

    #[test]
    fn test_replace_tilde_prefix() {
        assert_eq!(replace_tilde_prefix("~", "/home/user"), "/home/user");
        assert_eq!(
            replace_tilde_prefix("~/Documents", "/home/user"),
            "/home/user/Documents"
        );
        assert_eq!(
            replace_tilde_prefix("/absolute/path", "/home/user"),
            "/absolute/path"
        );
    }

    #[test]
    fn test_expand_home_prefix_no_tilde() {
        assert_eq!(expand_home_prefix("/usr/bin"), "/usr/bin");
        assert_eq!(expand_home_prefix("relative/path"), "relative/path");
    }

    #[test]
    fn test_resolve_required_home_dir_returns_something() {
        let result = resolve_required_home_dir();
        assert!(!result.is_empty());
    }

    #[test]
    fn test_resolve_effective_home_dir_returns_some() {
        // In most test environments, there should be a HOME set
        let result = resolve_effective_home_dir();
        // We can't guarantee it in all environments, but it should
        // generally be Some on Unix/macOS
        if std::env::var("HOME").is_ok() || dirs::home_dir().is_some() {
            assert!(result.is_some());
        }
    }

    #[test]
    fn test_resolve_state_dir_default() {
        // Without OPENACOSMI_STATE_DIR set, should end with .openacosmi
        let saved = std::env::var("OPENACOSMI_STATE_DIR").ok();
        let saved2 = std::env::var("CLAWDBOT_STATE_DIR").ok();

        // SAFETY: Tests run single-threaded with --test-threads=1 or
        // this test is isolated enough that env mutation is acceptable.
        unsafe {
            std::env::remove_var("OPENACOSMI_STATE_DIR");
            std::env::remove_var("CLAWDBOT_STATE_DIR");
        }

        let result = resolve_state_dir();
        assert!(result.ends_with(".openacosmi"), "Expected .openacosmi suffix, got: {result}");

        // Restore
        unsafe {
            if let Some(v) = saved {
                std::env::set_var("OPENACOSMI_STATE_DIR", v);
            }
            if let Some(v) = saved2 {
                std::env::set_var("CLAWDBOT_STATE_DIR", v);
            }
        }
    }
}
