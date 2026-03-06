/// Approvals get — calls `exec.approvals.get` via Gateway RPC.
use std::collections::HashMap;

use anyhow::{Result, bail};
use serde_json::Value;

use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

/// Get current exec approvals configuration from the Gateway.
pub async fn approvals_get_command(
    gateway: bool,
    node: Option<&str>,
    json_out: bool,
) -> Result<()> {
    if node.is_some() {
        bail!(
            "--node is not supported for approvals get; manage ~/.openacosmi/exec-approvals.json on the node host directly"
        );
    }

    let resp: HashMap<String, Value> = call_gateway(CallGatewayOptions {
        method: "exec.approvals.get".to_string(),
        params: None,
        ..Default::default()
    })
    .await?;

    if json_out {
        println!("{}", serde_json::to_string_pretty(&resp)?);
        return Ok(());
    }

    // Human-readable output
    let path = resp.get("path").and_then(|v| v.as_str()).unwrap_or("?");
    let exists = resp
        .get("exists")
        .and_then(|v| v.as_bool())
        .unwrap_or(false);
    let hash = resp.get("hash").and_then(|v| v.as_str()).unwrap_or("?");

    println!("📋 Exec Approvals:");
    println!("   Path:   {path}");
    println!("   Exists: {exists}");
    println!("   Hash:   {hash}");

    if let Some(file) = resp.get("file") {
        if let Some(defaults) = file.get("defaults") {
            let security = defaults
                .get("security")
                .and_then(|v| v.as_str())
                .unwrap_or("?");
            let ask = defaults.get("ask").and_then(|v| v.as_str()).unwrap_or("?");
            println!("   Defaults: security={security}, ask={ask}");
        }
    }

    let _ = gateway; // used for target selection
    Ok(())
}
