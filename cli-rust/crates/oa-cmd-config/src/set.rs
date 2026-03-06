/// Config set command.
///
/// Sets a value in the Claw Acosmi configuration at a dot-separated path
/// and writes the updated config to disk.
///
/// Source: `src/commands/config-set.ts`

use anyhow::{Context, Result};

use oa_config::io::{load_config, write_config_file};
use oa_types::config::OpenAcosmiConfig;

/// Set a configuration value at the given dot-separated path.
///
/// The `value` string is first parsed as JSON; if that fails it is
/// stored as a plain string. The config is then serialized back and
/// written atomically.
///
/// Source: `src/commands/config-set.ts` - `configSetCommand`
pub async fn config_set_command(path: &str, value: &str) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let mut serialized = serde_json::to_value(&cfg)?;

    let parsed_value: serde_json::Value = serde_json::from_str(value)
        .unwrap_or_else(|_| serde_json::Value::String(value.to_string()));

    let keys: Vec<&str> = path.split('.').collect();
    let mut target = &mut serialized;
    for (i, key) in keys.iter().enumerate() {
        if i == keys.len() - 1 {
            target[key] = parsed_value.clone();
        } else {
            if !target.get(key).is_some_and(|v| v.is_object()) {
                target[key] = serde_json::json!({});
            }
            target = &mut target[key];
        }
    }

    let updated: OpenAcosmiConfig =
        serde_json::from_value(serialized).context("Failed to deserialize updated config")?;
    write_config_file(&updated)
        .await
        .context("Failed to save config")?;
    println!("Set {path} = {value}");

    Ok(())
}
