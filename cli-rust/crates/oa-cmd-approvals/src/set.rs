/// Approvals set — calls `exec.approvals.set` via Gateway RPC.
use anyhow::{Context, Result, bail};
use serde_json::Value;

use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

/// Set exec approvals from a file, with optimistic concurrency control.
pub async fn approvals_set_command(file: &str, gateway: bool, node: Option<&str>) -> Result<()> {
    if node.is_some() {
        bail!(
            "--node is not supported for approvals set; update ~/.openacosmi/exec-approvals.json on the node host directly"
        );
    }

    // Read the approvals file
    let content = std::fs::read_to_string(file)
        .with_context(|| format!("Failed to read approvals file: {file}"))?;
    let file_value: Value = serde_json::from_str(&content)
        .with_context(|| format!("Failed to parse approvals file as JSON: {file}"))?;

    // First, get current hash for optimistic concurrency
    let current: std::collections::HashMap<String, Value> = call_gateway(CallGatewayOptions {
        method: "exec.approvals.get".to_string(),
        params: None,
        ..Default::default()
    })
    .await?;

    let base_hash = current.get("hash").and_then(|v| v.as_str()).unwrap_or("");

    // Now set with the base hash
    let set_params = serde_json::json!({
        "file": file_value,
        "baseHash": base_hash,
    });

    let _resp: std::collections::HashMap<String, Value> = call_gateway(CallGatewayOptions {
        method: "exec.approvals.set".to_string(),
        params: Some(set_params),
        ..Default::default()
    })
    .await?;

    println!("✅ Exec approvals updated from {file}");
    let _ = gateway;
    Ok(())
}
