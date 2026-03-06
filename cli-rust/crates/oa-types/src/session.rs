/// Session types for Claw Acosmi CLI.
///
/// Source: `src/config/sessions/types.ts`

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::common::ChatType;
use crate::tts::TtsAutoMode;

/// Session chat type alias.
pub type SessionChatType = ChatType;

/// Thread ID can be a string or number.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum SessionThreadId {
    String(String),
    Number(i64),
}

/// Session origin information.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct SessionOrigin {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub label: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub surface: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub chat_type: Option<SessionChatType>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub from: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub to: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub account_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub thread_id: Option<SessionThreadId>,
}

/// Delivery context (simplified for Rust; full schema is internal).
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DeliveryContext {
    #[serde(flatten)]
    pub extra: HashMap<String, serde_json::Value>,
}

/// Injected workspace file report entry.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InjectedWorkspaceFileEntry {
    pub name: String,
    pub path: String,
    pub missing: bool,
    pub raw_chars: u64,
    pub injected_chars: u64,
    pub truncated: bool,
}

/// Skill entry in system prompt report.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillReportEntry {
    pub name: String,
    pub block_chars: u64,
}

/// Tool entry in system prompt report.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolReportEntry {
    pub name: String,
    pub summary_chars: u64,
    pub schema_chars: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub properties_count: Option<u64>,
}

/// System prompt report sandbox info.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct SystemPromptReportSandbox {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mode: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sandboxed: Option<bool>,
}

/// System prompt char counts.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SystemPromptChars {
    pub chars: u64,
    pub project_context_chars: u64,
    pub non_project_context_chars: u64,
}

/// Skills section of system prompt report.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SystemPromptReportSkills {
    pub prompt_chars: u64,
    pub entries: Vec<SkillReportEntry>,
}

/// Tools section of system prompt report.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SystemPromptReportTools {
    pub list_chars: u64,
    pub schema_chars: u64,
    pub entries: Vec<ToolReportEntry>,
}

/// Source of the system prompt report.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum SystemPromptReportSource {
    Run,
    Estimate,
}

/// System prompt report for a session.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionSystemPromptReport {
    pub source: SystemPromptReportSource,
    pub generated_at: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session_key: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub workspace_dir: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bootstrap_max_chars: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sandbox: Option<SystemPromptReportSandbox>,
    pub system_prompt: SystemPromptChars,
    pub injected_workspace_files: Vec<InjectedWorkspaceFileEntry>,
    pub skills: SystemPromptReportSkills,
    pub tools: SystemPromptReportTools,
}

/// Skill snapshot entry.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionSkillSnapshotEntry {
    pub name: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub primary_env: Option<String>,
}

/// Skills snapshot for a session.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionSkillSnapshot {
    pub prompt: String,
    pub skills: Vec<SessionSkillSnapshotEntry>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resolved_skills: Option<Vec<serde_json::Value>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub version: Option<u32>,
}

/// Session entry stored in the session store.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionEntry {
    pub session_id: String,
    pub updated_at: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_heartbeat_text: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_heartbeat_sent_at: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session_file: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub spawned_by: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub system_sent: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub aborted_last_run: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub chat_type: Option<SessionChatType>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub verbose_level: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reasoning_level: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub elevated_level: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tts_auto: Option<TtsAutoMode>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exec_host: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exec_security: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exec_ask: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exec_node: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub response_usage: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub provider_override: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model_override: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub auth_profile_override: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub auth_profile_override_source: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub auth_profile_override_compaction_count: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub group_activation: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub group_activation_needs_system_intro: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub send_policy: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub queue_mode: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub queue_debounce_ms: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub queue_cap: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub queue_drop: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub input_tokens: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub output_tokens: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub total_tokens: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model_provider: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub context_tokens: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub compaction_count: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub memory_flush_at: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub memory_flush_compaction_count: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cli_session_ids: Option<HashMap<String, String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub claude_cli_session_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub label: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub display_name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub channel: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub group_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub subject: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub group_channel: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub space: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub origin: Option<SessionOrigin>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub delivery_context: Option<DeliveryContext>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_channel: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_to: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_account_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_thread_id: Option<SessionThreadId>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skills_snapshot: Option<SessionSkillSnapshot>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub system_prompt_report: Option<SessionSystemPromptReport>,
}

/// Group key resolution.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GroupKeyResolution {
    pub key: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub channel: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub chat_type: Option<SessionChatType>,
}
