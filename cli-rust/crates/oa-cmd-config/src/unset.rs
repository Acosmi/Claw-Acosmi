/// Config unset command.
///
/// Removes a key from the Claw Acosmi configuration at a dot-separated
/// path and writes the updated config to disk.
///
/// Source: `src/commands/config-unset.ts`

use anyhow::{Context, Result};

use oa_config::io::{load_config, write_config_file};
use oa_types::config::OpenAcosmiConfig;

/// Remove a configuration key at the given dot-separated path.
///
/// Traverses the config object to the parent of the target key and
/// removes it. Writes the updated config atomically.
///
/// Source: `src/commands/config-unset.ts` - `configUnsetCommand`
pub async fn config_unset_command(path: &str) -> Result<()> {
    let cfg = load_config().context("Failed to load config")?;
    let mut serialized = serde_json::to_value(&cfg)?;

    let keys: Vec<&str> = path.split('.').collect();
    let removed = if keys.len() == 1 {
        serialized
            .as_object_mut()
            .and_then(|obj| obj.remove(keys[0]))
            .is_some()
    } else {
        let parent_keys = &keys[..keys.len() - 1];
        let last_key = keys[keys.len() - 1];
        let mut target = &mut serialized;
        for key in parent_keys {
            target = &mut target[key];
        }
        target
            .as_object_mut()
            .and_then(|obj| obj.remove(last_key))
            .is_some()
    };

    if removed {
        let updated: OpenAcosmiConfig =
            serde_json::from_value(serialized).context("Failed to deserialize")?;
        write_config_file(&updated)
            .await
            .context("Failed to save config")?;
        println!("Removed {path}");
    } else {
        println!("Key \"{path}\" not found in config.");
    }

    Ok(())
}
