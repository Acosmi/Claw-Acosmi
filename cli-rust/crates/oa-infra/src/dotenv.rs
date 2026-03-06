/// Dotenv file loading utilities.
///
/// Loads `.env` files from the current working directory and from the
/// global Claw Acosmi config directory (`~/.openacosmi/.env`).
///
/// Source: `src/infra/dotenv.ts`

use std::path::Path;

use tracing::debug;

use crate::home_dir::resolve_config_dir;

/// Load `.env` files into the process environment.
///
/// This function loads environment variables from `.env` files in two locations:
///
/// 1. **Current working directory**: Loads `.env` from the process CWD first.
///    Variables loaded here take precedence over the global file.
/// 2. **Global config directory**: Loads `~/.openacosmi/.env` (or the directory
///    specified by `OPENACOSMI_STATE_DIR`). Variables from this file will NOT
///    override any already-set environment variables.
///
/// Both files are optional; missing files are silently ignored.
///
/// # Safety
///
/// This function modifies environment variables (via the `dotenvy` crate).
/// It should be called during single-threaded startup before spawning threads.
pub fn load_dotenv() {
    // 1. Load from CWD first (dotenvy default behavior).
    //    dotenvy::dotenv() looks for `.env` in the current directory.
    //    It does NOT override existing env vars by default.
    match dotenvy::dotenv() {
        Ok(path) => {
            debug!("Loaded .env from {}", path.display());
        }
        Err(dotenvy::Error::Io(ref e)) if e.kind() == std::io::ErrorKind::NotFound => {
            // No .env file in CWD, that's fine.
        }
        Err(e) => {
            debug!("Failed to load .env from CWD: {e}");
        }
    }

    // 2. Load global fallback: ~/.openacosmi/.env
    let config_dir = resolve_config_dir();
    let global_env_path = Path::new(&config_dir).join(".env");

    if !global_env_path.exists() {
        return;
    }

    match dotenvy::from_path(&global_env_path) {
        Ok(()) => {
            debug!("Loaded global .env from {}", global_env_path.display());
        }
        Err(e) => {
            debug!("Failed to load global .env from {}: {e}", global_env_path.display());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_load_dotenv_no_crash() {
        // Ensure load_dotenv doesn't panic even without any .env files
        load_dotenv();
    }

    #[test]
    fn test_load_dotenv_with_temp_dir() {
        // Create a temporary .env file and verify it loads
        let temp_dir = std::env::temp_dir().join("oa-infra-dotenv-test");
        let _ = std::fs::create_dir_all(&temp_dir);
        let env_file = temp_dir.join(".env");
        std::fs::write(&env_file, "OA_TEST_DOTENV_VAR=hello_from_dotenv\n")
            .expect("Failed to write temp .env");

        // Save original CWD and change to temp dir
        let original_cwd = std::env::current_dir().expect("Failed to get CWD");
        std::env::set_current_dir(&temp_dir).expect("Failed to change CWD");

        // SAFETY: Test environment, single test thread assumed.
        unsafe {
            std::env::remove_var("OA_TEST_DOTENV_VAR");
        }

        load_dotenv();

        // Restore CWD before assertions
        std::env::set_current_dir(original_cwd).expect("Failed to restore CWD");

        assert_eq!(
            std::env::var("OA_TEST_DOTENV_VAR").ok().as_deref(),
            Some("hello_from_dotenv")
        );

        // Cleanup
        // SAFETY: Test environment, single test thread assumed.
        unsafe {
            std::env::remove_var("OA_TEST_DOTENV_VAR");
        }
        let _ = std::fs::remove_dir_all(&temp_dir);
    }
}
