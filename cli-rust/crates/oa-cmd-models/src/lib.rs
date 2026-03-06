/// Model management commands for Claw Acosmi CLI.
///
/// Provides subcommands for listing, setting, aliasing, and managing
/// model fallbacks. Mirrors the TypeScript commands in
/// `src/commands/models.ts` and `src/commands/models/*.ts`.
///
/// Source: `src/commands/models.ts`, `src/commands/models/*.ts`

pub mod shared;
pub mod aliases;
pub mod fallbacks;
pub mod image_fallbacks;
pub mod list_types;
pub mod list_configured;
pub mod list_format;
pub mod set;
pub mod set_image;
