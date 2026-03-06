/// Config get command.
///
/// Reads a value from the Claw Acosmi configuration by dot-separated path.
///
/// Source: `src/commands/config-get.ts`

use anyhow::{Context, Result};

use oa_config::io::load_config;

/// Read a configuration value by dot-separated path.
///
/// Traverses the serialized config object using the dot-separated
/// `path` (e.g., `"gateway.port"`) and prints the value found.
///
/// Source: `src/commands/config-get.ts` - `configGetCommand`
pub fn config_get_command(path: &str, json: bool) -> Result<()> {
    let cfg = load_config().context("Failed to load config")?;
    let serialized = serde_json::to_value(&cfg)?;

    let value = path
        .split('.')
        .fold(Some(&serialized), |acc, key| acc.and_then(|v| v.get(key)));

    match value {
        Some(v) => {
            if json {
                println!("{}", serde_json::to_string_pretty(v)?);
            } else {
                match v {
                    serde_json::Value::String(s) => println!("{s}"),
                    other => println!("{}", serde_json::to_string_pretty(other)?),
                }
            }
        }
        None => {
            anyhow::bail!("Config path \"{path}\" not found");
        }
    }

    Ok(())
}
