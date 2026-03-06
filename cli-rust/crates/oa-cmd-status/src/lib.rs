/// Status, status-all, and gateway-status commands for Claw Acosmi CLI.
///
/// Source: `src/commands/status.ts`, `src/commands/status-all.ts`,
///         `src/commands/gateway-status.ts`

pub mod types;
pub mod format;
pub mod gateway_probe;
pub mod agent_local;
pub mod daemon;
pub mod summary;
pub mod scan;
pub mod update;
pub mod status_all;
pub mod gateway_status;
pub mod status_command;

// Re-export the main command entry points.
pub use gateway_status::gateway_status_command;
pub use status_command::status_command;
pub use summary::get_status_summary;
pub use types::{HeartbeatStatus, SessionStatus, StatusSummary};
