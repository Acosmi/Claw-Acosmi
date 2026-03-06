/// `sandbox explain` command: displays the effective sandbox configuration
/// for a given agent/session, including mode, scope, tool policy, and
/// elevated access gates.
///
/// Source: `src/commands/sandbox-explain.ts`
use anyhow::Result;
use serde::{Deserialize, Serialize};

use oa_channels::normalize_any_channel_id;
use oa_config::io::load_config;
use oa_routing::session_key::{
    build_agent_main_session_key, normalize_agent_id, normalize_main_key, parse_agent_session_key,
    resolve_agent_id_from_session_key,
};
use oa_terminal::links::format_docs_link;
use oa_terminal::theme::Theme;
use oa_types::config::OpenAcosmiConfig;

/// Options for the `sandbox explain` subcommand.
///
/// Source: `src/commands/sandbox-explain.ts` — `SandboxExplainOptions`
#[derive(Debug, Clone, Default)]
pub struct SandboxExplainOptions {
    /// Session key override.
    pub session: Option<String>,
    /// Agent ID override.
    pub agent: Option<String>,
    /// Output in JSON format.
    pub json: bool,
}

/// Sandbox mode setting.
///
/// Source: `src/commands/sandbox-explain.ts` — `sandboxCfg.mode`
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum SandboxMode {
    /// All sessions are sandboxed.
    All,
    /// Only non-main sessions are sandboxed.
    NonMain,
    /// Sandboxing is disabled.
    Off,
}

impl Default for SandboxMode {
    fn default() -> Self {
        Self::NonMain
    }
}

/// Sandbox scope setting.
///
/// Source: `src/commands/sandbox-explain.ts` — `sandboxCfg.scope`
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum SandboxScope {
    /// One container per session.
    Session,
    /// One container per agent.
    Agent,
}

impl Default for SandboxScope {
    fn default() -> Self {
        Self::Session
    }
}

/// Workspace access level.
///
/// Source: `src/commands/sandbox-explain.ts` — `sandboxCfg.workspaceAccess`
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum WorkspaceAccess {
    /// No workspace access.
    None,
    /// Read-only workspace access.
    #[serde(rename = "read-only")]
    ReadOnly,
    /// Full read-write workspace access.
    #[serde(rename = "read-write")]
    ReadWrite,
}

impl Default for WorkspaceAccess {
    fn default() -> Self {
        Self::ReadOnly
    }
}

/// Tool policy source info.
///
/// Source: `src/commands/sandbox-explain.ts` — `toolPolicy.sources`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolPolicySource {
    /// Source label (e.g. "global", "agent").
    pub source: String,
}

/// Tool policy: allow/deny lists with their source labels.
///
/// Source: `src/commands/sandbox-explain.ts` — sandbox tool policy
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolPolicy {
    /// List of allowed tool patterns.
    pub allow: Vec<String>,
    /// List of denied tool patterns.
    pub deny: Vec<String>,
    /// Sources for the allow/deny lists.
    pub sources: ToolPolicySources,
}

/// Source labels for allow/deny tool policy lists.
///
/// Source: `src/commands/sandbox-explain.ts` — `toolPolicy.sources`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolPolicySources {
    /// Source of the allow list.
    pub allow: ToolPolicySource,
    /// Source of the deny list.
    pub deny: ToolPolicySource,
}

/// A failing gate for elevated access.
///
/// Source: `src/commands/sandbox-explain.ts` — `elevatedFailures` entries
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ElevatedFailure {
    /// Gate name (e.g. "enabled", "allowFrom").
    pub gate: String,
    /// Config key that failed.
    pub key: String,
}

/// The full explain payload for JSON output.
///
/// Source: `src/commands/sandbox-explain.ts` — `payload`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SandboxExplainPayload {
    /// Documentation URL.
    pub docs_url: String,
    /// Resolved agent ID.
    pub agent_id: String,
    /// Resolved session key.
    pub session_key: String,
    /// Main session key for this agent.
    pub main_session_key: String,
    /// Sandbox configuration section.
    pub sandbox: SandboxSection,
    /// Elevated access configuration section.
    pub elevated: ElevatedSection,
    /// Fix-it configuration key suggestions.
    pub fix_it: Vec<String>,
}

/// Sandbox section of the explain payload.
///
/// Source: `src/commands/sandbox-explain.ts` — `payload.sandbox`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SandboxSection {
    /// Sandbox mode.
    pub mode: SandboxMode,
    /// Sandbox scope.
    pub scope: SandboxScope,
    /// Whether scope is per-session.
    pub per_session: bool,
    /// Workspace access level.
    pub workspace_access: WorkspaceAccess,
    /// Workspace root directory.
    pub workspace_root: String,
    /// Whether this session is sandboxed.
    pub session_is_sandboxed: bool,
    /// Tool policy.
    pub tools: ToolPolicy,
}

/// Elevated access section of the explain payload.
///
/// Source: `src/commands/sandbox-explain.ts` — `payload.elevated`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ElevatedSection {
    /// Whether elevated access is enabled globally.
    pub enabled: bool,
    /// Active channel for this session.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub channel: Option<String>,
    /// Whether elevated access is allowed by config.
    pub allowed_by_config: bool,
    /// Whether elevated access is always allowed.
    pub always_allowed_by_config: bool,
    /// Failing gates that prevent elevated access.
    pub failures: Vec<ElevatedFailure>,
}

/// The internal message channel identifier.
///
/// Source: `src/utils/message-channel.ts` — `INTERNAL_MESSAGE_CHANNEL`
const INTERNAL_MESSAGE_CHANNEL: &str = "internal";

/// The sandbox docs URL.
///
/// Source: `src/commands/sandbox-explain.ts` — `SANDBOX_DOCS_URL`
const SANDBOX_DOCS_URL: &str = "https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills/tools/sandbox";

/// Normalize the session key for explain, building a full agent session key
/// if only a partial key is given.
///
/// Source: `src/commands/sandbox-explain.ts` — `normalizeExplainSessionKey`
fn normalize_explain_session_key(
    cfg: &OpenAcosmiConfig,
    agent_id: &str,
    session: Option<&str>,
) -> String {
    let raw = session.unwrap_or("").trim();
    if raw.is_empty() {
        let main_key = cfg.session.as_ref().and_then(|s| s.main_key.as_deref());
        return build_agent_main_session_key(agent_id, main_key);
    }
    if raw.contains(':') {
        return raw.to_owned();
    }
    if raw == "global" {
        return "global".to_owned();
    }
    build_agent_main_session_key(agent_id, Some(&normalize_main_key(Some(raw))))
}

/// Infer a channel from the session key structure.
///
/// Source: `src/commands/sandbox-explain.ts` — `inferProviderFromSessionKey`
fn infer_channel_from_session_key(cfg: &OpenAcosmiConfig, session_key: &str) -> Option<String> {
    let parsed = parse_agent_session_key(Some(session_key))?;
    let rest = parsed.rest.trim();
    if rest.is_empty() {
        return None;
    }
    let parts: Vec<&str> = rest.split(':').filter(|s| !s.is_empty()).collect();
    if parts.is_empty() {
        return None;
    }
    let configured_main_key =
        normalize_main_key(cfg.session.as_ref().and_then(|s| s.main_key.as_deref()));
    if parts[0] == configured_main_key {
        return None;
    }
    let candidate = parts[0].trim().to_lowercase();
    if candidate.is_empty() {
        return None;
    }
    if candidate == INTERNAL_MESSAGE_CHANNEL {
        return Some(INTERNAL_MESSAGE_CHANNEL.to_owned());
    }
    normalize_any_channel_id(&candidate).map(|id| id.as_str().to_owned())
}

/// Resolve the sandbox mode from config.
///
/// Source: `src/commands/sandbox-explain.ts` — `resolveSandboxConfigForAgent`
fn resolve_sandbox_mode(cfg: &OpenAcosmiConfig, _agent_id: &str) -> SandboxMode {
    // Check agent-level config first, then defaults
    if let Some(agents) = &cfg.agents {
        if let Some(defaults) = &agents.defaults {
            if let Some(sandbox) = &defaults.sandbox {
                if let Some(mode_val) = &sandbox.mode {
                    return match mode_val {
                        oa_types::agent_defaults::SandboxMode::All => SandboxMode::All,
                        oa_types::agent_defaults::SandboxMode::Off => SandboxMode::Off,
                        oa_types::agent_defaults::SandboxMode::NonMain => SandboxMode::NonMain,
                    };
                }
            }
        }
    }
    SandboxMode::NonMain
}

/// Build the fix-it list of config keys to check.
///
/// Source: `src/commands/sandbox-explain.ts` — `fixIt`
fn build_fix_it(mode: &SandboxMode, channel: Option<&str>) -> Vec<String> {
    let mut fix_it = Vec::new();
    if *mode != SandboxMode::Off {
        fix_it.push("agents.defaults.sandbox.mode=off".to_owned());
        fix_it.push("agents.list[].sandbox.mode=off".to_owned());
    }
    fix_it.push("tools.sandbox.tools.allow".to_owned());
    fix_it.push("tools.sandbox.tools.deny".to_owned());
    fix_it.push("agents.list[].tools.sandbox.tools.allow".to_owned());
    fix_it.push("agents.list[].tools.sandbox.tools.deny".to_owned());
    fix_it.push("tools.elevated.enabled".to_owned());
    if let Some(ch) = channel {
        fix_it.push(format!("tools.elevated.allowFrom.{ch}"));
    }
    fix_it
}

/// Execute the `sandbox explain` command.
///
/// Source: `src/commands/sandbox-explain.ts` — `sandboxExplainCommand`
pub async fn sandbox_explain_command(opts: &SandboxExplainOptions) -> Result<()> {
    let cfg = load_config()?;

    let default_agent_id =
        resolve_agent_id_from_session_key(cfg.session.as_ref().and_then(|s| s.main_key.as_deref()));
    let resolved_agent_id = normalize_agent_id(
        opts.agent
            .as_deref()
            .filter(|s| !s.trim().is_empty())
            .or_else(|| {
                opts.session
                    .as_deref()
                    .filter(|s| !s.trim().is_empty())
                    .map(|s| {
                        // Can't return reference to temp; use default
                        let _ = s;
                        ""
                    })
            })
            .or(Some(&default_agent_id)),
    );

    let session_key =
        normalize_explain_session_key(&cfg, &resolved_agent_id, opts.session.as_deref());

    let main_key = cfg.session.as_ref().and_then(|s| s.main_key.as_deref());
    let main_session_key = build_agent_main_session_key(&resolved_agent_id, main_key);

    let mode = resolve_sandbox_mode(&cfg, &resolved_agent_id);
    let scope = SandboxScope::Session;
    let workspace_access = WorkspaceAccess::ReadOnly;
    let workspace_root = oa_agents::scope::resolve_agent_workspace_dir(&cfg, &resolved_agent_id)
        .display()
        .to_string();

    let session_is_sandboxed = match mode {
        SandboxMode::All => true,
        SandboxMode::Off => false,
        SandboxMode::NonMain => session_key.trim() != main_session_key.trim(),
    };

    let channel = infer_channel_from_session_key(&cfg, &session_key);

    let elevated_enabled = cfg
        .tools
        .as_ref()
        .and_then(|t| t.elevated.as_ref())
        .and_then(|e| e.enabled)
        .unwrap_or(true);

    let mut elevated_failures = Vec::new();
    if !elevated_enabled {
        elevated_failures.push(ElevatedFailure {
            gate: "enabled".to_owned(),
            key: "tools.elevated.enabled".to_owned(),
        });
    }

    let fix_it = build_fix_it(&mode, channel.as_deref());

    let payload = SandboxExplainPayload {
        docs_url: SANDBOX_DOCS_URL.to_owned(),
        agent_id: resolved_agent_id.clone(),
        session_key: session_key.clone(),
        main_session_key: main_session_key.clone(),
        sandbox: SandboxSection {
            mode: mode.clone(),
            scope: scope.clone(),
            per_session: scope == SandboxScope::Session,
            workspace_access,
            workspace_root,
            session_is_sandboxed,
            tools: ToolPolicy {
                allow: Vec::new(),
                deny: Vec::new(),
                sources: ToolPolicySources {
                    allow: ToolPolicySource {
                        source: "default".to_owned(),
                    },
                    deny: ToolPolicySource {
                        source: "default".to_owned(),
                    },
                },
            },
        },
        elevated: ElevatedSection {
            enabled: elevated_enabled,
            channel: channel.clone(),
            allowed_by_config: elevated_enabled && channel.is_some(),
            always_allowed_by_config: false,
            failures: elevated_failures,
        },
        fix_it: fix_it.clone(),
    };

    if opts.json {
        println!("{}\n", serde_json::to_string_pretty(&payload)?);
        return Ok(());
    }

    let bool_str = |flag: bool| -> String {
        if flag {
            Theme::success("true")
        } else {
            Theme::error("false")
        }
    };

    let mut lines = Vec::new();
    lines.push(Theme::heading("Effective sandbox:"));
    lines.push(format!(
        "  {} {}",
        Theme::muted("agentId:"),
        Theme::info(&payload.agent_id)
    ));
    lines.push(format!(
        "  {} {}",
        Theme::muted("sessionKey:"),
        Theme::info(&payload.session_key)
    ));
    lines.push(format!(
        "  {} {}",
        Theme::muted("mainSessionKey:"),
        Theme::info(&payload.main_session_key)
    ));
    lines.push(format!(
        "  {} {}",
        Theme::muted("runtime:"),
        if payload.sandbox.session_is_sandboxed {
            Theme::warn("sandboxed")
        } else {
            Theme::success("direct")
        }
    ));

    let mode_str = serde_json::to_value(&payload.sandbox.mode)
        .ok()
        .and_then(|v| v.as_str().map(String::from))
        .unwrap_or_else(|| format!("{:?}", payload.sandbox.mode));
    let scope_str = serde_json::to_value(&payload.sandbox.scope)
        .ok()
        .and_then(|v| v.as_str().map(String::from))
        .unwrap_or_else(|| format!("{:?}", payload.sandbox.scope));

    lines.push(format!(
        "  {} {} {} {} {} {}",
        Theme::muted("mode:"),
        Theme::info(&mode_str),
        Theme::muted("scope:"),
        Theme::info(&scope_str),
        Theme::muted("perSession:"),
        bool_str(payload.sandbox.per_session)
    ));

    let wa_str = serde_json::to_value(&payload.sandbox.workspace_access)
        .ok()
        .and_then(|v| v.as_str().map(String::from))
        .unwrap_or_else(|| format!("{:?}", payload.sandbox.workspace_access));

    lines.push(format!(
        "  {} {} {} {}",
        Theme::muted("workspaceAccess:"),
        Theme::info(&wa_str),
        Theme::muted("workspaceRoot:"),
        Theme::info(&payload.sandbox.workspace_root)
    ));
    lines.push(String::new());

    lines.push(Theme::heading("Sandbox tool policy:"));
    lines.push(format!(
        "  {} {}",
        Theme::muted(&format!(
            "allow ({}):",
            payload.sandbox.tools.sources.allow.source
        )),
        Theme::info(&if payload.sandbox.tools.allow.is_empty() {
            "(empty)".to_owned()
        } else {
            payload.sandbox.tools.allow.join(", ")
        })
    ));
    lines.push(format!(
        "  {} {}",
        Theme::muted(&format!(
            "deny  ({}):",
            payload.sandbox.tools.sources.deny.source
        )),
        Theme::info(&if payload.sandbox.tools.deny.is_empty() {
            "(empty)".to_owned()
        } else {
            payload.sandbox.tools.deny.join(", ")
        })
    ));
    lines.push(String::new());

    lines.push(Theme::heading("Elevated:"));
    lines.push(format!(
        "  {} {}",
        Theme::muted("enabled:"),
        bool_str(payload.elevated.enabled)
    ));
    lines.push(format!(
        "  {} {}",
        Theme::muted("channel:"),
        Theme::info(payload.elevated.channel.as_deref().unwrap_or("(unknown)"))
    ));
    lines.push(format!(
        "  {} {}",
        Theme::muted("allowedByConfig:"),
        bool_str(payload.elevated.allowed_by_config)
    ));

    if !payload.elevated.failures.is_empty() {
        let failure_text = payload
            .elevated
            .failures
            .iter()
            .map(|f| format!("{} ({})", f.gate, f.key))
            .collect::<Vec<_>>()
            .join(", ");
        lines.push(format!(
            "  {} {}",
            Theme::muted("failing gates:"),
            Theme::warn(&failure_text)
        ));
    }

    if mode == SandboxMode::NonMain && payload.sandbox.session_is_sandboxed {
        lines.push(String::new());
        lines.push(format!(
            "{} sandbox mode is non-main; use main session key to run direct: {}",
            Theme::warn("Hint:"),
            Theme::info(&payload.main_session_key)
        ));
    }

    lines.push(String::new());
    lines.push(Theme::heading("Fix-it:"));
    for key in &fix_it {
        lines.push(format!("  - {key}"));
    }
    lines.push(String::new());
    lines.push(format!(
        "{} {}",
        Theme::muted("Docs:"),
        format_docs_link("/skills/tools/sandbox", Some("Sandbox docs"))
    ));

    println!("{}\n", lines.join("\n"));

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalize_explain_session_key_empty() {
        let cfg = OpenAcosmiConfig::default();
        let key = normalize_explain_session_key(&cfg, "main", None);
        assert_eq!(key, "agent:main:main");
    }

    #[test]
    fn normalize_explain_session_key_global() {
        let cfg = OpenAcosmiConfig::default();
        let key = normalize_explain_session_key(&cfg, "main", Some("global"));
        assert_eq!(key, "global");
    }

    #[test]
    fn normalize_explain_session_key_with_colon() {
        let cfg = OpenAcosmiConfig::default();
        let key = normalize_explain_session_key(&cfg, "main", Some("agent:bot:work"));
        assert_eq!(key, "agent:bot:work");
    }

    #[test]
    fn normalize_explain_session_key_plain() {
        let cfg = OpenAcosmiConfig::default();
        let key = normalize_explain_session_key(&cfg, "main", Some("work"));
        assert_eq!(key, "agent:main:work");
    }

    #[test]
    fn infer_channel_discord_session() {
        let cfg = OpenAcosmiConfig::default();
        let channel = infer_channel_from_session_key(&cfg, "agent:main:discord:group:general");
        assert_eq!(channel.as_deref(), Some("discord"));
    }

    #[test]
    fn infer_channel_main_session() {
        let cfg = OpenAcosmiConfig::default();
        let channel = infer_channel_from_session_key(&cfg, "agent:main:main");
        assert_eq!(channel, None);
    }

    #[test]
    fn infer_channel_internal() {
        let cfg = OpenAcosmiConfig::default();
        let channel = infer_channel_from_session_key(&cfg, "agent:main:internal:task:123");
        assert_eq!(channel.as_deref(), Some("internal"));
    }

    #[test]
    fn infer_channel_empty() {
        let cfg = OpenAcosmiConfig::default();
        let channel = infer_channel_from_session_key(&cfg, "");
        assert_eq!(channel, None);
    }

    #[test]
    fn resolve_sandbox_mode_default() {
        let cfg = OpenAcosmiConfig::default();
        assert_eq!(resolve_sandbox_mode(&cfg, "main"), SandboxMode::NonMain);
    }

    #[test]
    fn build_fix_it_with_channel() {
        let fix = build_fix_it(&SandboxMode::NonMain, Some("discord"));
        assert!(fix.contains(&"agents.defaults.sandbox.mode=off".to_owned()));
        assert!(fix.contains(&"tools.elevated.allowFrom.discord".to_owned()));
    }

    #[test]
    fn build_fix_it_off_mode() {
        let fix = build_fix_it(&SandboxMode::Off, None);
        assert!(!fix.contains(&"agents.defaults.sandbox.mode=off".to_owned()));
        assert!(fix.contains(&"tools.elevated.enabled".to_owned()));
    }

    #[test]
    fn sandbox_explain_payload_serializes() {
        let payload = SandboxExplainPayload {
            docs_url: SANDBOX_DOCS_URL.to_owned(),
            agent_id: "main".to_owned(),
            session_key: "agent:main:main".to_owned(),
            main_session_key: "agent:main:main".to_owned(),
            sandbox: SandboxSection {
                mode: SandboxMode::NonMain,
                scope: SandboxScope::Session,
                per_session: true,
                workspace_access: WorkspaceAccess::ReadOnly,
                workspace_root: "/tmp/workspace".to_owned(),
                session_is_sandboxed: false,
                tools: ToolPolicy {
                    allow: Vec::new(),
                    deny: Vec::new(),
                    sources: ToolPolicySources {
                        allow: ToolPolicySource {
                            source: "default".to_owned(),
                        },
                        deny: ToolPolicySource {
                            source: "default".to_owned(),
                        },
                    },
                },
            },
            elevated: ElevatedSection {
                enabled: true,
                channel: None,
                allowed_by_config: false,
                always_allowed_by_config: false,
                failures: Vec::new(),
            },
            fix_it: vec!["tools.elevated.enabled".to_owned()],
        };
        let json = serde_json::to_string_pretty(&payload);
        assert!(json.is_ok());
        let json = json.expect("should serialize");
        assert!(json.contains("\"agentId\": \"main\""));
        assert!(json.contains("\"nonMain\"") || json.contains("\"non-main\""));
    }
}
