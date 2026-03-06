/// Agent management commands (plural) for Claw Acosmi CLI.
///
/// Provides subcommands for listing, adding, deleting agents, and
/// managing agent identity and bindings. Mirrors the TypeScript commands
/// in `src/commands/agents*.ts`.
///
/// Source: `src/commands/agents.ts`, `src/commands/agents.*.ts`

pub mod add;
pub mod bindings;
pub mod command_shared;
pub mod config;
pub mod delete;
pub mod list;
pub mod set_identity;
