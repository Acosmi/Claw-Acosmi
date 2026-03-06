/// Runtime environment abstraction for Claw Acosmi CLI.
///
/// Provides a trait-based abstraction for runtime environment access,
/// enabling testing with mock environments.
///
/// Source: `src/runtime.ts`

use async_trait::async_trait;

/// Runtime environment trait.
///
/// Abstracts access to the runtime environment (filesystem, env vars, etc.)
/// to enable testing with mock implementations.
#[async_trait]
pub trait RuntimeEnv: Send + Sync {
    /// Get an environment variable value.
    fn env_var(&self, key: &str) -> Option<String>;

    /// Get the current working directory.
    fn cwd(&self) -> std::path::PathBuf;

    /// Get the home directory.
    fn home_dir(&self) -> Option<std::path::PathBuf>;

    /// Check if running in a TTY.
    fn is_tty(&self) -> bool;

    /// Get the current platform.
    fn platform(&self) -> &str;
}

/// Default runtime environment using real system calls.
pub struct DefaultRuntime;

#[async_trait]
impl RuntimeEnv for DefaultRuntime {
    fn env_var(&self, key: &str) -> Option<String> {
        std::env::var(key).ok()
    }

    fn cwd(&self) -> std::path::PathBuf {
        std::env::current_dir().unwrap_or_default()
    }

    fn home_dir(&self) -> Option<std::path::PathBuf> {
        dirs::home_dir()
    }

    fn is_tty(&self) -> bool {
        std::io::IsTerminal::is_terminal(&std::io::stdin())
    }

    fn platform(&self) -> &str {
        std::env::consts::OS
    }
}
