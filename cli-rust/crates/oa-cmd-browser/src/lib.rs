/// Browser automation commands for Claw Acosmi CLI.
///
/// All commands delegate to the Gateway via `browser.request` RPC, which
/// proxies to the local browser control HTTP service or a remote node.
///
/// RPC method: `browser.request`
/// Params: `{ method: "GET"|"POST"|"DELETE", path: "/...", query: {...}, body: {...} }`

use anyhow::Result;
use clap::Parser;

use oa_cli_shared::progress::with_progress;
use oa_config::io::load_config;
use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

// ---------------------------------------------------------------------------
// Helper: browser.request RPC wrapper
// ---------------------------------------------------------------------------

/// Send a `browser.request` RPC to the Gateway.
///
/// This is the **single RPC method** for all browser operations. The Gateway
/// routes it to the local browser control HTTP service (or proxies to a node).
async fn browser_rpc(
    http_method: &str,
    path: &str,
    query: Option<serde_json::Value>,
    body: Option<serde_json::Value>,
    profile: Option<&str>,
    progress_msg: &str,
    show_progress: bool,
) -> Result<serde_json::Value> {
    let cfg = match load_config() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("warning: failed to load config, using defaults: {e}");
            Default::default()
        }
    };

    let mut params = serde_json::json!({
        "method": http_method,
        "path": path,
    });

    // Merge query with profile
    let mut q = query.unwrap_or_else(|| serde_json::json!({}));
    if let Some(p) = profile {
        q["profile"] = serde_json::json!(p);
    }
    if q.as_object().map_or(false, |o| !o.is_empty()) {
        params["query"] = q;
    }
    if let Some(b) = body {
        params["body"] = b;
    }

    let opts = CallGatewayOptions {
        method: "browser.request".to_string(),
        config: Some(cfg),
        params: Some(params),
        ..Default::default()
    };

    if show_progress {
        with_progress(progress_msg, call_gateway(opts)).await
    } else {
        call_gateway(opts).await
    }
}

/// Print result as pretty JSON.
fn print_result(result: &serde_json::Value) {
    match serde_json::to_string_pretty(result) {
        Ok(s) => println!("{s}"),
        Err(e) => eprintln!("error: failed to serialize result: {e}"),
    }
}

// ===========================================================================
// Manage: status / start / stop / tabs / open / focus / close / profiles
// ===========================================================================

/// CLI arguments for `browser status`.
#[derive(Debug, Parser)]
pub struct BrowserStatusArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser status`.
pub async fn browser_status_command(args: &BrowserStatusArgs) -> Result<()> {
    let result = browser_rpc(
        "GET", "/",
        None, None,
        args.browser_profile.as_deref(),
        "Checking browser status\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser start`.
#[derive(Debug, Parser)]
pub struct BrowserStartArgs {
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
    /// Run headless.
    #[arg(long)]
    pub headless: bool,
}

/// Execute `browser start`.
pub async fn browser_start_command(args: &BrowserStartArgs) -> Result<()> {
    let mut body = serde_json::json!({});
    if args.headless {
        body["headless"] = serde_json::json!(true);
    }
    let result = browser_rpc(
        "POST", "/start",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Starting browser\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser stop`.
#[derive(Debug, Parser)]
pub struct BrowserStopArgs {
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser stop`.
pub async fn browser_stop_command(args: &BrowserStopArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/stop",
        None, None,
        args.browser_profile.as_deref(),
        "Stopping browser\u{2026}",
        true,
    ).await?;
    println!("Browser stopped.");
    Ok(())
}

/// CLI arguments for `browser tabs`.
#[derive(Debug, Parser)]
pub struct BrowserTabsArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser tabs`.
pub async fn browser_tabs_command(args: &BrowserTabsArgs) -> Result<()> {
    let result = browser_rpc(
        "GET", "/tabs",
        None, None,
        args.browser_profile.as_deref(),
        "Loading tabs\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser open`.
#[derive(Debug, Parser)]
pub struct BrowserOpenArgs {
    /// URL to open.
    pub url: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser open`.
pub async fn browser_open_command(args: &BrowserOpenArgs) -> Result<()> {
    let result = browser_rpc(
        "POST", "/tabs/open",
        None, Some(serde_json::json!({ "url": args.url })),
        args.browser_profile.as_deref(),
        "Opening URL\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser focus`.
#[derive(Debug, Parser)]
pub struct BrowserFocusArgs {
    /// Target ID of the tab to focus.
    pub target_id: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser focus`.
pub async fn browser_focus_command(args: &BrowserFocusArgs) -> Result<()> {
    let result = browser_rpc(
        "POST", "/tabs/focus",
        None, Some(serde_json::json!({ "targetId": args.target_id })),
        args.browser_profile.as_deref(),
        "Focusing tab\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser close`.
#[derive(Debug, Parser)]
pub struct BrowserCloseArgs {
    /// Target ID of the tab to close (optional; closes active tab if omitted).
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser close`.
pub async fn browser_close_command(args: &BrowserCloseArgs) -> Result<()> {
    let path = if let Some(ref tid) = args.target_id {
        format!("/tabs/{}", tid)
    } else {
        "/tabs/active".to_string()
    };
    let _: serde_json::Value = browser_rpc(
        "DELETE", &path,
        None, None,
        args.browser_profile.as_deref(),
        "Closing tab\u{2026}",
        true,
    ).await?;
    println!("Tab closed.");
    Ok(())
}

/// CLI arguments for `browser screenshot`.
#[derive(Debug, Parser)]
pub struct BrowserScreenshotArgs {
    /// Output file path (optional; server decides filename if omitted).
    pub file: Option<String>,
    /// Full page screenshot.
    #[arg(long)]
    pub full_page: bool,
    /// Element ref for element screenshot.
    #[arg(long)]
    pub r#ref: Option<String>,
    /// CSS selector for element screenshot.
    #[arg(long)]
    pub element: Option<String>,
    /// Image format (png or jpeg).
    #[arg(long, name = "type")]
    pub format: Option<String>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser screenshot`.
pub async fn browser_screenshot_command(args: &BrowserScreenshotArgs) -> Result<()> {
    let mut body = serde_json::json!({});
    if let Some(ref file) = args.file {
        body["file"] = serde_json::json!(file);
    }
    if args.full_page {
        body["fullPage"] = serde_json::json!(true);
    }
    if let Some(ref r) = args.r#ref {
        body["ref"] = serde_json::json!(r);
    }
    if let Some(ref el) = args.element {
        body["element"] = serde_json::json!(el);
    }
    if let Some(ref fmt) = args.format {
        body["type"] = serde_json::json!(fmt);
    }
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/screenshot",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Taking screenshot\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser profiles`.
#[derive(Debug, Parser)]
pub struct BrowserProfilesArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// Execute `browser profiles`.
pub async fn browser_profiles_command(args: &BrowserProfilesArgs) -> Result<()> {
    let result = browser_rpc(
        "GET", "/profiles",
        None, None,
        None,
        "Loading profiles\u{2026}",
        !args.json,
    ).await?;

    if args.json {
        print_result(&result);
    } else if let Some(profiles) = result.as_array() {
        if profiles.is_empty() {
            println!("No browser profiles.");
        } else {
            for profile in profiles {
                let name = profile
                    .get("name")
                    .and_then(|n| n.as_str())
                    .unwrap_or("(unnamed)");
                println!("  {name}");
            }
        }
    } else {
        print_result(&result);
    }
    Ok(())
}

/// CLI arguments for `browser create-profile`.
#[derive(Debug, Parser)]
pub struct BrowserCreateProfileArgs {
    /// Profile name.
    pub name: String,
    /// Profile color (hex).
    #[arg(long)]
    pub color: Option<String>,
    /// CDP URL (for remote profiles).
    #[arg(long)]
    pub cdp_url: Option<String>,
    /// Driver type (managed, extension, remote).
    #[arg(long)]
    pub driver: Option<String>,
}

/// Execute `browser create-profile`.
pub async fn browser_create_profile_command(args: &BrowserCreateProfileArgs) -> Result<()> {
    let mut body = serde_json::json!({ "name": args.name });
    if let Some(ref color) = args.color {
        body["color"] = serde_json::json!(color);
    }
    if let Some(ref cdp_url) = args.cdp_url {
        body["cdpUrl"] = serde_json::json!(cdp_url);
    }
    if let Some(ref driver) = args.driver {
        body["driver"] = serde_json::json!(driver);
    }
    let _: serde_json::Value = browser_rpc(
        "POST", "/profiles/create",
        None, Some(body),
        None,
        "Creating profile\u{2026}",
        true,
    ).await?;
    println!("Profile '{}' created.", args.name);
    Ok(())
}

/// CLI arguments for `browser delete-profile`.
#[derive(Debug, Parser)]
pub struct BrowserDeleteProfileArgs {
    /// Profile name to delete.
    #[arg(long)]
    pub name: String,
}

/// Execute `browser delete-profile`.
pub async fn browser_delete_profile_command(args: &BrowserDeleteProfileArgs) -> Result<()> {
    let path = format!("/profiles/{}", args.name);
    let _: serde_json::Value = browser_rpc(
        "DELETE", &path,
        None, None,
        None,
        "Deleting profile\u{2026}",
        true,
    ).await?;
    println!("Profile '{}' deleted.", args.name);
    Ok(())
}

/// CLI arguments for `browser reset-profile`.
#[derive(Debug, Parser)]
pub struct BrowserResetProfileArgs {
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser reset-profile`.
pub async fn browser_reset_profile_command(args: &BrowserResetProfileArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/profiles/reset",
        None, None,
        args.browser_profile.as_deref(),
        "Resetting profile\u{2026}",
        true,
    ).await?;
    println!("Browser profile reset.");
    Ok(())
}

// ===========================================================================
// Inspect: snapshot / console / pdf / errors / requests / responsebody
// ===========================================================================

/// CLI arguments for `browser snapshot`.
#[derive(Debug, Parser)]
pub struct BrowserSnapshotArgs {
    /// Snapshot format: ai (default) or aria.
    #[arg(long, default_value = "ai")]
    pub format: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Max snapshot lines/chars.
    #[arg(long)]
    pub limit: Option<u32>,
    /// Interactive-only elements.
    #[arg(long)]
    pub interactive: bool,
    /// Compact output.
    #[arg(long)]
    pub compact: bool,
    /// Max tree depth.
    #[arg(long)]
    pub depth: Option<u32>,
    /// CSS selector to scope snapshot.
    #[arg(long)]
    pub selector: Option<String>,
    /// Frame selector for iframe scoping.
    #[arg(long)]
    pub frame: Option<String>,
    /// Efficient preset (interactive + compact + depth 6).
    #[arg(long)]
    pub efficient: bool,
    /// Overlay ref labels on screenshot.
    #[arg(long)]
    pub labels: bool,
    /// Output file path.
    #[arg(long)]
    pub out: Option<String>,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser snapshot`.
pub async fn browser_snapshot_command(args: &BrowserSnapshotArgs) -> Result<()> {
    let mut query = serde_json::json!({});
    query["format"] = serde_json::json!(args.format);
    if let Some(ref tid) = args.target_id {
        query["targetId"] = serde_json::json!(tid);
    }
    if let Some(limit) = args.limit {
        query["limit"] = serde_json::json!(limit);
    }
    if args.interactive {
        query["interactive"] = serde_json::json!(true);
    }
    if args.compact {
        query["compact"] = serde_json::json!(true);
    }
    if let Some(depth) = args.depth {
        query["depth"] = serde_json::json!(depth);
    }
    if let Some(ref sel) = args.selector {
        query["selector"] = serde_json::json!(sel);
    }
    if let Some(ref frame) = args.frame {
        query["frame"] = serde_json::json!(frame);
    }
    if args.efficient {
        query["mode"] = serde_json::json!("efficient");
    }
    if args.labels {
        query["labels"] = serde_json::json!(true);
    }
    if let Some(ref out) = args.out {
        query["out"] = serde_json::json!(out);
    }
    let result = browser_rpc(
        "GET", "/snapshot",
        Some(query), None,
        args.browser_profile.as_deref(),
        "Taking snapshot\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser console`.
#[derive(Debug, Parser)]
pub struct BrowserConsoleArgs {
    /// Filter by log level: error, warn, info.
    #[arg(long)]
    pub level: Option<String>,
    /// Clear console after reading.
    #[arg(long)]
    pub clear: bool,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser console`.
pub async fn browser_console_command(args: &BrowserConsoleArgs) -> Result<()> {
    let mut query = serde_json::json!({});
    if let Some(ref level) = args.level {
        query["level"] = serde_json::json!(level);
    }
    if args.clear {
        query["clear"] = serde_json::json!(true);
    }
    if let Some(ref tid) = args.target_id {
        query["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "GET", "/console",
        Some(query), None,
        args.browser_profile.as_deref(),
        "Reading console\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser errors`.
#[derive(Debug, Parser)]
pub struct BrowserErrorsArgs {
    /// Clear errors after reading.
    #[arg(long)]
    pub clear: bool,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser errors`.
pub async fn browser_errors_command(args: &BrowserErrorsArgs) -> Result<()> {
    let mut query = serde_json::json!({});
    if args.clear {
        query["clear"] = serde_json::json!(true);
    }
    if let Some(ref tid) = args.target_id {
        query["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "GET", "/errors",
        Some(query), None,
        args.browser_profile.as_deref(),
        "Reading errors\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser requests`.
#[derive(Debug, Parser)]
pub struct BrowserRequestsArgs {
    /// URL filter pattern.
    #[arg(long)]
    pub filter: Option<String>,
    /// Clear requests after reading.
    #[arg(long)]
    pub clear: bool,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser requests`.
pub async fn browser_requests_command(args: &BrowserRequestsArgs) -> Result<()> {
    let mut query = serde_json::json!({});
    if let Some(ref f) = args.filter {
        query["filter"] = serde_json::json!(f);
    }
    if args.clear {
        query["clear"] = serde_json::json!(true);
    }
    if let Some(ref tid) = args.target_id {
        query["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "GET", "/requests",
        Some(query), None,
        args.browser_profile.as_deref(),
        "Reading requests\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser responsebody`.
#[derive(Debug, Parser)]
pub struct BrowserResponseBodyArgs {
    /// URL pattern to match.
    pub pattern: String,
    /// Max chars to return.
    #[arg(long, default_value = "10000")]
    pub max_chars: u32,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser responsebody`.
pub async fn browser_responsebody_command(args: &BrowserResponseBodyArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "pattern": args.pattern,
        "maxChars": args.max_chars,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/response/body",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Reading response body\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser pdf`.
#[derive(Debug, Parser)]
pub struct BrowserPdfArgs {
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser pdf`.
pub async fn browser_pdf_command(args: &BrowserPdfArgs) -> Result<()> {
    let mut body = serde_json::json!({});
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/pdf",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Generating PDF\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

// ===========================================================================
// Actions: navigate / resize / click / type / press / hover / drag / select
//          upload / fill / dialog / wait / evaluate / highlight
// ===========================================================================

/// CLI arguments for `browser navigate`.
#[derive(Debug, Parser)]
pub struct BrowserNavigateArgs {
    /// URL to navigate to.
    pub url: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser navigate`.
pub async fn browser_navigate_command(args: &BrowserNavigateArgs) -> Result<()> {
    let mut body = serde_json::json!({ "url": args.url });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/navigate",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Navigating\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser resize`.
#[derive(Debug, Parser)]
pub struct BrowserResizeArgs {
    /// Width in pixels.
    pub width: u32,
    /// Height in pixels.
    pub height: u32,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser resize`.
pub async fn browser_resize_command(args: &BrowserResizeArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "resize",
        "width": args.width,
        "height": args.height,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Resizing viewport\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser click`.
#[derive(Debug, Parser)]
pub struct BrowserClickArgs {
    /// Element ref (numeric or role ref like e12).
    pub r#ref: String,
    /// Double-click.
    #[arg(long)]
    pub double: bool,
    /// Mouse button: left, right, middle.
    #[arg(long, default_value = "left")]
    pub button: String,
    /// Modifier keys (comma-separated: ctrl,shift,alt,meta).
    #[arg(long)]
    pub modifiers: Option<String>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser click`.
pub async fn browser_click_command(args: &BrowserClickArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "click",
        "ref": args.r#ref,
    });
    if args.double {
        body["doubleClick"] = serde_json::json!(true);
    }
    if args.button != "left" {
        body["button"] = serde_json::json!(args.button);
    }
    if let Some(ref m) = args.modifiers {
        body["modifiers"] = serde_json::json!(m);
    }
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Clicking\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser type` (text input).
#[derive(Debug, Parser)]
pub struct BrowserTypeArgs {
    /// Element ref (numeric or role ref like e12).
    pub r#ref: String,
    /// Text to type.
    pub text: String,
    /// Press Enter after typing.
    #[arg(long)]
    pub submit: bool,
    /// Type slowly (character by character).
    #[arg(long)]
    pub slowly: bool,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser type`.
pub async fn browser_type_command(args: &BrowserTypeArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "type",
        "ref": args.r#ref,
        "value": args.text,
    });
    if args.submit {
        body["submit"] = serde_json::json!(true);
    }
    if args.slowly {
        body["slowly"] = serde_json::json!(true);
    }
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Typing\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser press`.
#[derive(Debug, Parser)]
pub struct BrowserPressArgs {
    /// Key to press (e.g. Enter, Tab, Escape, ArrowDown).
    pub key: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser press`.
pub async fn browser_press_command(args: &BrowserPressArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "press",
        "key": args.key,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Pressing key\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser hover`.
#[derive(Debug, Parser)]
pub struct BrowserHoverArgs {
    /// Element ref.
    pub r#ref: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser hover`.
pub async fn browser_hover_command(args: &BrowserHoverArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "hover",
        "ref": args.r#ref,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Hovering\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser scrollintoview`.
#[derive(Debug, Parser)]
pub struct BrowserScrollIntoViewArgs {
    /// Element ref.
    pub r#ref: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser scrollintoview`.
pub async fn browser_scrollintoview_command(args: &BrowserScrollIntoViewArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "scrollintoview",
        "ref": args.r#ref,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Scrolling into view\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser drag`.
#[derive(Debug, Parser)]
pub struct BrowserDragArgs {
    /// Start element ref.
    pub start_ref: String,
    /// End element ref.
    pub end_ref: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser drag`.
pub async fn browser_drag_command(args: &BrowserDragArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "drag",
        "startRef": args.start_ref,
        "endRef": args.end_ref,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Dragging\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser select`.
#[derive(Debug, Parser)]
pub struct BrowserSelectArgs {
    /// Element ref of the select element.
    pub r#ref: String,
    /// Option values to select.
    pub values: Vec<String>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser select`.
pub async fn browser_select_command(args: &BrowserSelectArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "kind": "select",
        "ref": args.r#ref,
        "values": args.values,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/act",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Selecting\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser download`.
#[derive(Debug, Parser)]
pub struct BrowserDownloadArgs {
    /// Element ref to click for download.
    pub r#ref: String,
    /// Save path for the downloaded file.
    pub save_path: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser download`.
pub async fn browser_download_command(args: &BrowserDownloadArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "ref": args.r#ref,
        "savePath": args.save_path,
    });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/download",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Downloading\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser waitfordownload`.
#[derive(Debug, Parser)]
pub struct BrowserWaitForDownloadArgs {
    /// Save path for the downloaded file.
    pub save_path: String,
    /// Timeout in milliseconds.
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser waitfordownload`.
pub async fn browser_waitfordownload_command(args: &BrowserWaitForDownloadArgs) -> Result<()> {
    let mut body = serde_json::json!({ "savePath": args.save_path });
    if let Some(t) = args.timeout_ms {
        body["timeoutMs"] = serde_json::json!(t);
    }
    let result = browser_rpc(
        "POST", "/wait/download",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Waiting for download\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser upload`.
#[derive(Debug, Parser)]
pub struct BrowserUploadArgs {
    /// File paths to upload.
    pub paths: Vec<String>,
    /// Arm file chooser by ref.
    #[arg(long)]
    pub r#ref: Option<String>,
    /// Input element ref (direct set).
    #[arg(long)]
    pub input_ref: Option<String>,
    /// CSS selector for the input element.
    #[arg(long)]
    pub element: Option<String>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Timeout in milliseconds.
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser upload`.
pub async fn browser_upload_command(args: &BrowserUploadArgs) -> Result<()> {
    let mut body = serde_json::json!({ "paths": args.paths });
    if let Some(ref r) = args.r#ref {
        body["ref"] = serde_json::json!(r);
    }
    if let Some(ref ir) = args.input_ref {
        body["inputRef"] = serde_json::json!(ir);
    }
    if let Some(ref el) = args.element {
        body["element"] = serde_json::json!(el);
    }
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    if let Some(t) = args.timeout_ms {
        body["timeoutMs"] = serde_json::json!(t);
    }
    let result = browser_rpc(
        "POST", "/hooks/file-chooser",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Uploading\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser fill`.
#[derive(Debug, Parser)]
pub struct BrowserFillArgs {
    /// JSON array of field descriptors: [{"ref":"1","type":"text","value":"Ada"}].
    #[arg(long)]
    pub fields: Option<String>,
    /// Path to a JSON file with field descriptors.
    #[arg(long)]
    pub fields_file: Option<String>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser fill`.
pub async fn browser_fill_command(args: &BrowserFillArgs) -> Result<()> {
    let fields: serde_json::Value = if let Some(ref ff) = args.fields_file {
        let data = std::fs::read_to_string(ff)?;
        serde_json::from_str(&data)?
    } else if let Some(ref f) = args.fields {
        serde_json::from_str(f)?
    } else {
        anyhow::bail!("--fields or --fields-file is required");
    };
    let mut body = serde_json::json!({ "fields": fields });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/fill",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Filling form\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser dialog`.
#[derive(Debug, Parser)]
pub struct BrowserDialogArgs {
    /// Accept the dialog.
    #[arg(long)]
    pub accept: bool,
    /// Dismiss the dialog.
    #[arg(long)]
    pub dismiss: bool,
    /// Prompt text (for prompt dialogs).
    #[arg(long)]
    pub prompt: Option<String>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Timeout in milliseconds.
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser dialog`.
pub async fn browser_dialog_command(args: &BrowserDialogArgs) -> Result<()> {
    let mut body = serde_json::json!({});
    if args.accept {
        body["action"] = serde_json::json!("accept");
    } else if args.dismiss {
        body["action"] = serde_json::json!("dismiss");
    } else {
        body["action"] = serde_json::json!("accept");
    }
    if let Some(ref p) = args.prompt {
        body["promptText"] = serde_json::json!(p);
    }
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    if let Some(t) = args.timeout_ms {
        body["timeoutMs"] = serde_json::json!(t);
    }
    let result = browser_rpc(
        "POST", "/hooks/dialog",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Handling dialog\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser wait`.
#[derive(Debug, Parser)]
pub struct BrowserWaitArgs {
    /// CSS selector to wait for.
    pub selector: Option<String>,
    /// Wait for URL pattern (glob).
    #[arg(long)]
    pub url: Option<String>,
    /// Wait for load state (networkidle, load, domcontentloaded).
    #[arg(long)]
    pub load: Option<String>,
    /// Wait for JS predicate to return truthy.
    #[arg(long, name = "fn")]
    pub js_fn: Option<String>,
    /// Wait for text to appear.
    #[arg(long)]
    pub text: Option<String>,
    /// Wait for text to disappear.
    #[arg(long)]
    pub text_gone: Option<String>,
    /// Wait time in milliseconds.
    #[arg(long)]
    pub time: Option<u32>,
    /// Timeout in milliseconds.
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser wait`.
pub async fn browser_wait_command(args: &BrowserWaitArgs) -> Result<()> {
    let mut body = serde_json::json!({});
    if let Some(ref sel) = args.selector {
        body["selector"] = serde_json::json!(sel);
    }
    if let Some(ref u) = args.url {
        body["url"] = serde_json::json!(u);
    }
    if let Some(ref l) = args.load {
        body["load"] = serde_json::json!(l);
    }
    if let Some(ref f) = args.js_fn {
        body["fn"] = serde_json::json!(f);
    }
    if let Some(ref t) = args.text {
        body["text"] = serde_json::json!(t);
    }
    if let Some(ref tg) = args.text_gone {
        body["textGone"] = serde_json::json!(tg);
    }
    if let Some(t) = args.time {
        body["time"] = serde_json::json!(t);
    }
    if let Some(t) = args.timeout_ms {
        body["timeoutMs"] = serde_json::json!(t);
    }
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/wait",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Waiting\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser evaluate`.
#[derive(Debug, Parser)]
pub struct BrowserEvaluateArgs {
    /// JavaScript code to evaluate.
    #[arg(long, name = "fn")]
    pub js_fn: String,
    /// Element ref for evaluation context.
    #[arg(long)]
    pub r#ref: Option<String>,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser evaluate`.
pub async fn browser_evaluate_command(args: &BrowserEvaluateArgs) -> Result<()> {
    let mut body = serde_json::json!({ "fn": args.js_fn });
    if let Some(ref r) = args.r#ref {
        body["ref"] = serde_json::json!(r);
    }
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/evaluate",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Evaluating\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser highlight`.
#[derive(Debug, Parser)]
pub struct BrowserHighlightArgs {
    /// Element ref to highlight.
    pub r#ref: String,
    /// Target tab ID.
    #[arg(long)]
    pub target_id: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser highlight`.
pub async fn browser_highlight_command(args: &BrowserHighlightArgs) -> Result<()> {
    let mut body = serde_json::json!({ "ref": args.r#ref });
    if let Some(ref tid) = args.target_id {
        body["targetId"] = serde_json::json!(tid);
    }
    let result = browser_rpc(
        "POST", "/highlight",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Highlighting\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

// ===========================================================================
// Debug: trace start / trace stop
// ===========================================================================

/// CLI arguments for `browser trace`.
#[derive(Debug, Parser)]
pub struct BrowserTraceStartArgs {
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser trace start`.
pub async fn browser_trace_start_command(args: &BrowserTraceStartArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/trace/start",
        None, None,
        args.browser_profile.as_deref(),
        "Starting trace\u{2026}",
        true,
    ).await?;
    println!("Trace recording started.");
    Ok(())
}

/// CLI arguments for `browser trace stop`.
#[derive(Debug, Parser)]
pub struct BrowserTraceStopArgs {
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser trace stop`.
pub async fn browser_trace_stop_command(args: &BrowserTraceStopArgs) -> Result<()> {
    let result = browser_rpc(
        "POST", "/trace/stop",
        None, None,
        args.browser_profile.as_deref(),
        "Stopping trace\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

// ===========================================================================
// State: cookies / storage / set (offline/headers/credentials/geo/media/tz/locale/device)
// ===========================================================================

/// CLI arguments for `browser cookies`.
#[derive(Debug, Parser)]
pub struct BrowserCookiesArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser cookies`.
pub async fn browser_cookies_command(args: &BrowserCookiesArgs) -> Result<()> {
    let result = browser_rpc(
        "GET", "/cookies",
        None, None,
        args.browser_profile.as_deref(),
        "Reading cookies\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser cookies set`.
#[derive(Debug, Parser)]
pub struct BrowserCookiesSetArgs {
    /// Cookie name.
    pub name: String,
    /// Cookie value.
    pub value: String,
    /// Cookie URL/domain.
    #[arg(long)]
    pub url: Option<String>,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser cookies set`.
pub async fn browser_cookies_set_command(args: &BrowserCookiesSetArgs) -> Result<()> {
    let mut body = serde_json::json!({
        "name": args.name,
        "value": args.value,
    });
    if let Some(ref u) = args.url {
        body["url"] = serde_json::json!(u);
    }
    let _: serde_json::Value = browser_rpc(
        "POST", "/cookies/set",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Setting cookie\u{2026}",
        true,
    ).await?;
    println!("Cookie set.");
    Ok(())
}

/// CLI arguments for `browser cookies clear`.
#[derive(Debug, Parser)]
pub struct BrowserCookiesClearArgs {
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser cookies clear`.
pub async fn browser_cookies_clear_command(args: &BrowserCookiesClearArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/cookies/clear",
        None, None,
        args.browser_profile.as_deref(),
        "Clearing cookies\u{2026}",
        true,
    ).await?;
    println!("Cookies cleared.");
    Ok(())
}

/// CLI arguments for `browser storage`.
#[derive(Debug, Parser)]
pub struct BrowserStorageGetArgs {
    /// Storage kind: local or session.
    pub kind: String,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser storage <kind> get`.
pub async fn browser_storage_get_command(args: &BrowserStorageGetArgs) -> Result<()> {
    let path = format!("/storage/{}", args.kind);
    let result = browser_rpc(
        "GET", &path,
        None, None,
        args.browser_profile.as_deref(),
        "Reading storage\u{2026}",
        !args.json,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser storage set`.
#[derive(Debug, Parser)]
pub struct BrowserStorageSetArgs {
    /// Storage kind: local or session.
    pub kind: String,
    /// Key.
    pub key: String,
    /// Value.
    pub value: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser storage <kind> set`.
pub async fn browser_storage_set_command(args: &BrowserStorageSetArgs) -> Result<()> {
    let path = format!("/storage/{}/set", args.kind);
    let result = browser_rpc(
        "POST", &path,
        None, Some(serde_json::json!({ "key": args.key, "value": args.value })),
        args.browser_profile.as_deref(),
        "Setting storage\u{2026}",
        true,
    ).await?;
    print_result(&result);
    Ok(())
}

/// CLI arguments for `browser storage clear`.
#[derive(Debug, Parser)]
pub struct BrowserStorageClearArgs {
    /// Storage kind: local or session.
    pub kind: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser storage <kind> clear`.
pub async fn browser_storage_clear_command(args: &BrowserStorageClearArgs) -> Result<()> {
    let path = format!("/storage/{}/clear", args.kind);
    let _: serde_json::Value = browser_rpc(
        "POST", &path,
        None, None,
        args.browser_profile.as_deref(),
        "Clearing storage\u{2026}",
        true,
    ).await?;
    println!("Storage cleared.");
    Ok(())
}

/// CLI arguments for `browser set offline`.
#[derive(Debug, Parser)]
pub struct BrowserSetOfflineArgs {
    /// on or off.
    pub state: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set offline`.
pub async fn browser_set_offline_command(args: &BrowserSetOfflineArgs) -> Result<()> {
    let offline = args.state == "on" || args.state == "true";
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/offline",
        None, Some(serde_json::json!({ "offline": offline })),
        args.browser_profile.as_deref(),
        "Setting offline mode\u{2026}",
        true,
    ).await?;
    println!("Offline mode: {}.", if offline { "on" } else { "off" });
    Ok(())
}

/// CLI arguments for `browser set headers`.
#[derive(Debug, Parser)]
pub struct BrowserSetHeadersArgs {
    /// JSON object of headers, or --clear.
    #[arg(long)]
    pub json: Option<String>,
    /// Clear custom headers.
    #[arg(long)]
    pub clear: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set headers`.
pub async fn browser_set_headers_command(args: &BrowserSetHeadersArgs) -> Result<()> {
    let body = if args.clear {
        serde_json::json!({ "clear": true })
    } else if let Some(ref j) = args.json {
        let headers: serde_json::Value = serde_json::from_str(j)?;
        serde_json::json!({ "headers": headers })
    } else {
        anyhow::bail!("--json '<headers>' or --clear is required");
    };
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/headers",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Setting headers\u{2026}",
        true,
    ).await?;
    println!("Headers updated.");
    Ok(())
}

/// CLI arguments for `browser set credentials`.
#[derive(Debug, Parser)]
pub struct BrowserSetCredentialsArgs {
    /// Username.
    pub username: Option<String>,
    /// Password.
    pub password: Option<String>,
    /// Clear credentials.
    #[arg(long)]
    pub clear: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set credentials`.
pub async fn browser_set_credentials_command(args: &BrowserSetCredentialsArgs) -> Result<()> {
    let body = if args.clear {
        serde_json::json!({ "clear": true })
    } else if args.username.is_none() || args.password.is_none() {
        anyhow::bail!("both <USERNAME> and <PASSWORD> are required (or use --clear)");
    } else {
        serde_json::json!({
            "username": args.username,
            "password": args.password,
        })
    };
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/credentials",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Setting credentials\u{2026}",
        true,
    ).await?;
    if args.clear {
        println!("Credentials cleared.");
    } else {
        println!("Credentials set.");
    }
    Ok(())
}

/// CLI arguments for `browser set geo`.
#[derive(Debug, Parser)]
pub struct BrowserSetGeoArgs {
    /// Latitude.
    pub latitude: Option<f64>,
    /// Longitude.
    pub longitude: Option<f64>,
    /// Origin URL for geolocation permission.
    #[arg(long)]
    pub origin: Option<String>,
    /// Clear geolocation override.
    #[arg(long)]
    pub clear: bool,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set geo`.
pub async fn browser_set_geo_command(args: &BrowserSetGeoArgs) -> Result<()> {
    let body = if args.clear {
        serde_json::json!({ "clear": true })
    } else if args.latitude.is_none() || args.longitude.is_none() {
        anyhow::bail!("both <LATITUDE> and <LONGITUDE> are required (or use --clear)");
    } else {
        let mut b = serde_json::json!({
            "latitude": args.latitude,
            "longitude": args.longitude,
        });
        if let Some(ref o) = args.origin {
            b["origin"] = serde_json::json!(o);
        }
        b
    };
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/geolocation",
        None, Some(body),
        args.browser_profile.as_deref(),
        "Setting geolocation\u{2026}",
        true,
    ).await?;
    if args.clear {
        println!("Geolocation override cleared.");
    } else {
        println!("Geolocation set.");
    }
    Ok(())
}

/// CLI arguments for `browser set media`.
#[derive(Debug, Parser)]
pub struct BrowserSetMediaArgs {
    /// Media preference: dark, light, no-preference, none.
    pub scheme: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set media`.
pub async fn browser_set_media_command(args: &BrowserSetMediaArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/media",
        None, Some(serde_json::json!({ "colorScheme": args.scheme })),
        args.browser_profile.as_deref(),
        "Setting media preference\u{2026}",
        true,
    ).await?;
    println!("Media preference set to: {}.", args.scheme);
    Ok(())
}

/// CLI arguments for `browser set timezone`.
#[derive(Debug, Parser)]
pub struct BrowserSetTimezoneArgs {
    /// Timezone ID (e.g. America/New_York).
    pub timezone: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set timezone`.
pub async fn browser_set_timezone_command(args: &BrowserSetTimezoneArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/timezone",
        None, Some(serde_json::json!({ "timezoneId": args.timezone })),
        args.browser_profile.as_deref(),
        "Setting timezone\u{2026}",
        true,
    ).await?;
    println!("Timezone set to: {}.", args.timezone);
    Ok(())
}

/// CLI arguments for `browser set locale`.
#[derive(Debug, Parser)]
pub struct BrowserSetLocaleArgs {
    /// Locale (e.g. en-US, zh-CN).
    pub locale: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set locale`.
pub async fn browser_set_locale_command(args: &BrowserSetLocaleArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/locale",
        None, Some(serde_json::json!({ "locale": args.locale })),
        args.browser_profile.as_deref(),
        "Setting locale\u{2026}",
        true,
    ).await?;
    println!("Locale set to: {}.", args.locale);
    Ok(())
}

/// CLI arguments for `browser set device`.
#[derive(Debug, Parser)]
pub struct BrowserSetDeviceArgs {
    /// Playwright device preset name (e.g. "iPhone 14").
    pub device: String,
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

/// Execute `browser set device`.
pub async fn browser_set_device_command(args: &BrowserSetDeviceArgs) -> Result<()> {
    let _: serde_json::Value = browser_rpc(
        "POST", "/set/device",
        None, Some(serde_json::json!({ "device": args.device })),
        args.browser_profile.as_deref(),
        "Setting device\u{2026}",
        true,
    ).await?;
    println!("Device set to: {}.", args.device);
    Ok(())
}

// ===========================================================================
// Tests
// ===========================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn browser_status_args_defaults() {
        let args = BrowserStatusArgs {
            json: false,
            browser_profile: None,
        };
        assert!(!args.json);
        assert!(args.browser_profile.is_none());
    }

    #[test]
    fn browser_start_args_headless() {
        let args = BrowserStartArgs {
            browser_profile: Some("test".to_string()),
            headless: true,
        };
        assert!(args.headless);
        assert_eq!(args.browser_profile.as_deref(), Some("test"));
    }

    #[test]
    fn browser_open_args_url() {
        let args = BrowserOpenArgs {
            url: "https://example.com".to_string(),
            browser_profile: None,
        };
        assert_eq!(args.url, "https://example.com");
    }

    #[test]
    fn browser_screenshot_args_default_file() {
        let args = BrowserScreenshotArgs {
            file: None,
            full_page: false,
            r#ref: None,
            element: None,
            format: None,
            target_id: None,
            browser_profile: None,
        };
        assert!(args.file.is_none());
    }

    #[test]
    fn browser_create_profile_args() {
        let args = BrowserCreateProfileArgs {
            name: "dev".to_string(),
            color: None,
            cdp_url: None,
            driver: None,
        };
        assert_eq!(args.name, "dev");
    }

    #[test]
    fn browser_click_args() {
        let args = BrowserClickArgs {
            r#ref: "e12".to_string(),
            double: true,
            button: "left".to_string(),
            modifiers: None,
            target_id: None,
            browser_profile: None,
        };
        assert_eq!(args.r#ref, "e12");
        assert!(args.double);
    }

    #[test]
    fn browser_snapshot_args_efficient() {
        let args = BrowserSnapshotArgs {
            format: "ai".to_string(),
            target_id: None,
            limit: None,
            interactive: false,
            compact: false,
            depth: None,
            selector: None,
            frame: None,
            efficient: true,
            labels: false,
            out: None,
            json: false,
            browser_profile: None,
        };
        assert!(args.efficient);
    }

    #[test]
    fn browser_wait_args() {
        let args = BrowserWaitArgs {
            selector: Some("#main".to_string()),
            url: Some("**/dash".to_string()),
            load: Some("networkidle".to_string()),
            js_fn: None,
            text: None,
            text_gone: None,
            time: None,
            timeout_ms: Some(15000),
            target_id: None,
            browser_profile: None,
        };
        assert_eq!(args.selector.as_deref(), Some("#main"));
        assert_eq!(args.timeout_ms, Some(15000));
    }
}
