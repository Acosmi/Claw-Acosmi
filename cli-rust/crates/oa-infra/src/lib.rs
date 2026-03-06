/// Infrastructure utilities for Claw Acosmi CLI.
///
/// Provides environment variable access, error formatting,
/// time utilities, device identification, dotenv loading,
/// and home directory resolution.
///
/// Source: `src/infra/*.ts`

/// Home directory resolution and path expansion.
pub mod home_dir;

/// Environment variable utilities (truthy checks, normalization).
pub mod env;

/// Dotenv file loading from CWD and global config directory.
pub mod dotenv;

/// Error formatting utilities.
pub mod errors;

/// Device identity generation and persistence.
pub mod device;

/// Heartbeat service (placeholder).
pub mod heartbeat;

/// Time utilities (placeholder).
pub mod time;
