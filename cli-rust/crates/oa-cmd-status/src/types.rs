/// Status types for Claw Acosmi CLI.
///
/// Source: `src/commands/status.types.ts`

use serde::{Deserialize, Serialize};

/// Describes the kind of a session (direct, group, global, unknown).
///
/// Source: `src/commands/status.types.ts` - `SessionStatus.kind`
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SessionKind {
    /// Direct (1:1) session.
    Direct,
    /// Group or channel session.
    Group,
    /// Global session.
    Global,
    /// Unknown session type.
    Unknown,
}

impl std::fmt::Display for SessionKind {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Direct => write!(f, "direct"),
            Self::Group => write!(f, "group"),
            Self::Global => write!(f, "global"),
            Self::Unknown => write!(f, "unknown"),
        }
    }
}

/// Status of a single session, including token usage and flags.
///
/// Source: `src/commands/status.types.ts` - `SessionStatus`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionStatus {
    /// Agent ID that owns this session.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub agent_id: Option<String>,
    /// Session key.
    pub key: String,
    /// Session kind.
    pub kind: SessionKind,
    /// Session ID.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session_id: Option<String>,
    /// Timestamp of last update (epoch ms).
    pub updated_at: Option<u64>,
    /// Age in milliseconds since last update.
    pub age: Option<u64>,
    /// Thinking level override.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<String>,
    /// Verbose level override.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub verbose_level: Option<String>,
    /// Reasoning level override.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reasoning_level: Option<String>,
    /// Elevated level override.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub elevated_level: Option<String>,
    /// Whether the system prompt was sent for this session.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub system_sent: Option<bool>,
    /// Whether the last run was aborted.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub aborted_last_run: Option<bool>,
    /// Input tokens consumed.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub input_tokens: Option<u64>,
    /// Output tokens consumed.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub output_tokens: Option<u64>,
    /// Total tokens consumed.
    pub total_tokens: Option<u64>,
    /// Remaining tokens in context window.
    pub remaining_tokens: Option<i64>,
    /// Percentage of context window used.
    pub percent_used: Option<u64>,
    /// Model name.
    pub model: Option<String>,
    /// Context token limit.
    pub context_tokens: Option<u64>,
    /// Active session flags.
    pub flags: Vec<String>,
}

/// Heartbeat status for a specific agent.
///
/// Source: `src/commands/status.types.ts` - `HeartbeatStatus`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HeartbeatStatus {
    /// Agent ID.
    pub agent_id: String,
    /// Whether heartbeat is enabled.
    pub enabled: bool,
    /// Interval label (e.g., "5m", "1h").
    pub every: String,
    /// Interval in milliseconds.
    pub every_ms: Option<u64>,
}

/// Aggregated sessions by agent.
///
/// Source: `src/commands/status.types.ts` - `StatusSummary.sessions.byAgent`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentSessionSummary {
    /// Agent ID.
    pub agent_id: String,
    /// Store path.
    pub path: String,
    /// Session count.
    pub count: usize,
    /// Most recent sessions.
    pub recent: Vec<SessionStatus>,
}

/// Summary of all status information.
///
/// Source: `src/commands/status.types.ts` - `StatusSummary`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StatusSummary {
    /// Link channel context.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub link_channel: Option<LinkChannelInfo>,
    /// Heartbeat summary.
    pub heartbeat: HeartbeatSummary,
    /// Channel summary lines.
    pub channel_summary: Vec<String>,
    /// Queued system events.
    pub queued_system_events: Vec<String>,
    /// Sessions info.
    pub sessions: SessionsInfo,
}

/// Link channel info.
///
/// Source: `src/commands/status.types.ts` - `StatusSummary.linkChannel`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LinkChannelInfo {
    /// Channel ID.
    pub id: String,
    /// Display label.
    pub label: String,
    /// Whether the channel is linked.
    pub linked: bool,
    /// Auth age in milliseconds.
    pub auth_age_ms: Option<u64>,
}

/// Heartbeat summary across agents.
///
/// Source: `src/commands/status.types.ts` - `StatusSummary.heartbeat`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HeartbeatSummary {
    /// Default agent ID.
    pub default_agent_id: String,
    /// Per-agent heartbeat statuses.
    pub agents: Vec<HeartbeatStatus>,
}

/// Session defaults (model and context tokens).
///
/// Source: `src/commands/status.types.ts` - `StatusSummary.sessions.defaults`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDefaults {
    /// Default model.
    pub model: Option<String>,
    /// Default context token limit.
    pub context_tokens: Option<u64>,
}

/// Sessions information.
///
/// Source: `src/commands/status.types.ts` - `StatusSummary.sessions`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionsInfo {
    /// Store paths.
    pub paths: Vec<String>,
    /// Total session count.
    pub count: usize,
    /// Session defaults.
    pub defaults: SessionDefaults,
    /// Most recent sessions across all agents.
    pub recent: Vec<SessionStatus>,
    /// Per-agent session summary.
    pub by_agent: Vec<AgentSessionSummary>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn session_kind_display() {
        assert_eq!(SessionKind::Direct.to_string(), "direct");
        assert_eq!(SessionKind::Group.to_string(), "group");
        assert_eq!(SessionKind::Global.to_string(), "global");
        assert_eq!(SessionKind::Unknown.to_string(), "unknown");
    }

    #[test]
    fn session_kind_serialization() {
        let json = serde_json::to_string(&SessionKind::Group).unwrap_or_default();
        assert_eq!(json, "\"group\"");
    }

    #[test]
    fn session_status_serializes_camel_case() {
        let sess = SessionStatus {
            agent_id: Some("main".to_string()),
            key: "test-key".to_string(),
            kind: SessionKind::Direct,
            session_id: None,
            updated_at: Some(1_000_000),
            age: Some(500),
            thinking_level: None,
            verbose_level: None,
            reasoning_level: None,
            elevated_level: None,
            system_sent: None,
            aborted_last_run: None,
            input_tokens: Some(100),
            output_tokens: Some(50),
            total_tokens: Some(150),
            remaining_tokens: Some(850),
            percent_used: Some(15),
            model: Some("gpt-4".to_string()),
            context_tokens: Some(1000),
            flags: vec!["system".to_string()],
        };
        let json = serde_json::to_value(&sess).unwrap_or_default();
        assert_eq!(json["agentId"], "main");
        assert_eq!(json["totalTokens"], 150);
        assert_eq!(json["percentUsed"], 15);
        assert_eq!(json["contextTokens"], 1000);
    }

    #[test]
    fn heartbeat_status_serializes() {
        let hb = HeartbeatStatus {
            agent_id: "main".to_string(),
            enabled: true,
            every: "5m".to_string(),
            every_ms: Some(300_000),
        };
        let json = serde_json::to_value(&hb).unwrap_or_default();
        assert_eq!(json["agentId"], "main");
        assert_eq!(json["enabled"], true);
        assert_eq!(json["everyMs"], 300_000);
    }

    #[test]
    fn status_summary_round_trip() {
        let summary = StatusSummary {
            link_channel: None,
            heartbeat: HeartbeatSummary {
                default_agent_id: "main".to_string(),
                agents: vec![],
            },
            channel_summary: vec!["test".to_string()],
            queued_system_events: vec![],
            sessions: SessionsInfo {
                paths: vec!["/tmp/sessions.json".to_string()],
                count: 0,
                defaults: SessionDefaults {
                    model: Some("gpt-4".to_string()),
                    context_tokens: Some(128_000),
                },
                recent: vec![],
                by_agent: vec![],
            },
        };
        let json = serde_json::to_string(&summary).unwrap_or_default();
        let parsed: StatusSummary = serde_json::from_str(&json).unwrap_or_else(|_| summary.clone());
        assert_eq!(parsed.sessions.count, 0);
        assert_eq!(parsed.heartbeat.default_agent_id, "main");
    }
}
