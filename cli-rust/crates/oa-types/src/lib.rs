/// Core types for Claw Acosmi CLI.
///
/// This crate contains all shared type definitions used across the CLI,
/// including configuration types, session types, health/status types,
/// and common enums/structs.
///
/// Source: `src/config/types.*.ts`, `src/config/sessions/types.ts`

// ── Foundational modules (no internal deps or only depend on common) ──
pub mod common;
pub mod tts;
pub mod queue;
pub mod sandbox;

// ── Base types (depends on common) ──
pub mod base;
pub mod auth;

// ── Domain modules (depend on base/common/sandbox) ──
pub mod models;
pub mod gateway;
pub mod tools;
pub mod memory;
pub mod plugins;
pub mod skills;
pub mod browser;
pub mod cron;
pub mod node_host;
pub mod approvals;
pub mod hooks;

// ── Higher-level modules (depend on domain modules) ──
pub mod agent_defaults;
pub mod messages;
pub mod agents;
pub mod channels;

// ── Top-level config (depends on everything) ──
pub mod config;

// ── Runtime types ──
pub mod session;
pub mod health;
pub mod status;

// ── Convenience re-exports ──
pub use config::OpenAcosmiConfig;
pub use config::ConfigFileSnapshot;
pub use config::ConfigValidationIssue;
pub use config::LegacyConfigIssue;
pub use common::ChatType;
pub use common::JsonOutput;
pub use session::SessionEntry;
pub use health::HealthResult;
pub use status::SystemStatus;
