/// Status types for Claw Acosmi CLI.
///
/// Source: `src/commands/status/status.types.ts`

use serde::{Deserialize, Serialize};

/// System status information.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SystemStatus {
    /// Whether the system is running
    pub running: bool,
}
