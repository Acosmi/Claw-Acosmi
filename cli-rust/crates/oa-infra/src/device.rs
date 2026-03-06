/// Device identity management.
///
/// Generates and persists a unique device identity based on a SHA-256 hash
/// of system information (hostname, OS, etc.) combined with a UUID salt.
/// The identity is stored as a JSON file in the Claw Acosmi state directory.
///
/// Source: `src/infra/device-identity.ts`

use std::fs;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use tracing::{debug, warn};

use crate::home_dir::resolve_state_dir;

/// A device identity containing a unique device ID.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DeviceIdentity {
    /// The unique device identifier (SHA-256 hex string).
    pub device_id: String,
}

/// Stored identity format on disk.
#[derive(Debug, Clone, Serialize, Deserialize)]
struct StoredIdentity {
    /// Format version (always 1).
    version: u32,
    /// The unique device ID.
    device_id: String,
    /// Timestamp of creation in milliseconds since epoch.
    created_at_ms: u64,
}

/// Get the default identity directory path.
fn default_identity_dir() -> PathBuf {
    Path::new(&resolve_state_dir()).join("identity")
}

/// Get the default identity file path.
fn default_identity_file() -> PathBuf {
    default_identity_dir().join("device.json")
}

/// Generate a new device ID based on system characteristics and a random salt.
///
/// The device ID is a SHA-256 hash of the hostname, OS, architecture, and
/// a random UUID salt, producing a deterministic-looking but unique identifier.
fn generate_device_id() -> String {
    let mut hasher = Sha256::new();

    // Include hostname
    let hostname = sysinfo::System::host_name().unwrap_or_default();
    hasher.update(hostname.as_bytes());

    // Include OS
    hasher.update(std::env::consts::OS.as_bytes());

    // Include architecture
    hasher.update(std::env::consts::ARCH.as_bytes());

    // Include a random UUID as salt (makes each generation unique)
    let salt = uuid::Uuid::new_v4().to_string();
    hasher.update(salt.as_bytes());

    // Produce hex-encoded SHA-256 hash
    let result = hasher.finalize();
    hex_encode(&result)
}

/// Encode bytes as a lowercase hex string.
fn hex_encode(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// Ensure the parent directory of a file path exists.
fn ensure_parent_dir(file_path: &Path) -> std::io::Result<()> {
    if let Some(parent) = file_path.parent() {
        fs::create_dir_all(parent)?;
    }
    Ok(())
}

/// Set file permissions to owner-only read/write (0o600) on Unix systems.
#[cfg(unix)]
fn set_file_permissions(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    if let Err(e) = fs::set_permissions(path, fs::Permissions::from_mode(0o600)) {
        debug!("Failed to set permissions on {}: {e}", path.display());
    }
}

/// Set file permissions (no-op on non-Unix systems).
#[cfg(not(unix))]
fn set_file_permissions(_path: &Path) {
    // No-op on non-Unix platforms
}

/// Write a stored identity to disk.
fn write_identity(file_path: &Path, stored: &StoredIdentity) -> anyhow::Result<()> {
    ensure_parent_dir(file_path)?;
    let json = serde_json::to_string_pretty(stored)?;
    fs::write(file_path, format!("{json}\n"))?;
    set_file_permissions(file_path);
    Ok(())
}

/// Load an existing device identity from disk, or create a new one.
///
/// If a valid identity file exists at the given path (or the default path),
/// it is loaded and returned. Otherwise, a new identity is generated,
/// persisted to disk, and returned.
///
/// # Arguments
///
/// * `file_path` - Optional custom path for the identity file. If `None`,
///   uses `~/.openacosmi/identity/device.json`.
///
/// # Returns
///
/// The loaded or newly generated [`DeviceIdentity`].
pub fn load_or_create_device_identity(file_path: Option<&Path>) -> DeviceIdentity {
    let path = file_path
        .map(PathBuf::from)
        .unwrap_or_else(default_identity_file);

    // Try to load existing identity
    if path.exists() {
        match load_identity_from_file(&path) {
            Ok(identity) => return identity,
            Err(e) => {
                warn!("Failed to load device identity from {}: {e}. Regenerating.", path.display());
            }
        }
    }

    // Generate new identity
    let device_id = generate_device_id();
    let now = chrono::Utc::now().timestamp_millis();
    #[allow(clippy::cast_sign_loss)]
    let stored = StoredIdentity {
        version: 1,
        device_id: device_id.clone(),
        created_at_ms: now as u64,
    };

    if let Err(e) = write_identity(&path, &stored) {
        warn!("Failed to persist device identity to {}: {e}", path.display());
    }

    DeviceIdentity { device_id }
}

/// Load a device identity from a JSON file.
fn load_identity_from_file(path: &Path) -> anyhow::Result<DeviceIdentity> {
    let raw = fs::read_to_string(path)?;
    let parsed: StoredIdentity = serde_json::from_str(&raw)?;

    if parsed.version != 1 {
        anyhow::bail!("Unsupported identity version: {}", parsed.version);
    }

    if parsed.device_id.is_empty() {
        anyhow::bail!("Empty device_id in stored identity");
    }

    debug!("Loaded device identity from {}", path.display());
    Ok(DeviceIdentity {
        device_id: parsed.device_id,
    })
}

/// Get the device ID, loading or creating the identity as needed.
///
/// This is a convenience wrapper around [`load_or_create_device_identity`]
/// using the default file path.
pub fn get_device_id() -> String {
    load_or_create_device_identity(None).device_id
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_device_id() {
        let id = generate_device_id();
        // SHA-256 hex is 64 characters
        assert_eq!(id.len(), 64);
        // Should be all hex chars
        assert!(id.chars().all(|c| c.is_ascii_hexdigit()));
    }

    #[test]
    fn test_generate_device_id_unique() {
        let id1 = generate_device_id();
        let id2 = generate_device_id();
        // Each generation includes a random UUID salt, so they should differ
        assert_ne!(id1, id2);
    }

    #[test]
    fn test_hex_encode() {
        assert_eq!(hex_encode(&[0x00, 0xff, 0xab]), "00ffab");
        assert_eq!(hex_encode(&[]), "");
    }

    #[test]
    fn test_load_or_create_device_identity_new() {
        let temp_dir = std::env::temp_dir().join("oa-infra-device-test");
        let _ = fs::remove_dir_all(&temp_dir);
        fs::create_dir_all(&temp_dir).expect("Failed to create temp dir");

        let file_path = temp_dir.join("device.json");
        let identity = load_or_create_device_identity(Some(&file_path));

        assert_eq!(identity.device_id.len(), 64);
        assert!(file_path.exists(), "Identity file should have been created");

        // Clean up
        let _ = fs::remove_dir_all(&temp_dir);
    }

    #[test]
    fn test_load_or_create_device_identity_existing() {
        let temp_dir = std::env::temp_dir().join("oa-infra-device-test-existing");
        let _ = fs::remove_dir_all(&temp_dir);
        fs::create_dir_all(&temp_dir).expect("Failed to create temp dir");

        let file_path = temp_dir.join("device.json");

        // Create an identity
        let first = load_or_create_device_identity(Some(&file_path));

        // Load the same identity
        let second = load_or_create_device_identity(Some(&file_path));

        assert_eq!(first.device_id, second.device_id, "Should load the same identity");

        // Clean up
        let _ = fs::remove_dir_all(&temp_dir);
    }

    #[test]
    fn test_load_or_create_device_identity_corrupt_file() {
        let temp_dir = std::env::temp_dir().join("oa-infra-device-test-corrupt");
        let _ = fs::remove_dir_all(&temp_dir);
        fs::create_dir_all(&temp_dir).expect("Failed to create temp dir");

        let file_path = temp_dir.join("device.json");
        fs::write(&file_path, "not valid json").expect("Failed to write");

        // Should regenerate
        let identity = load_or_create_device_identity(Some(&file_path));
        assert_eq!(identity.device_id.len(), 64);

        // Clean up
        let _ = fs::remove_dir_all(&temp_dir);
    }
}
