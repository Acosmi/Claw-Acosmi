/// CLI shared utilities for Claw Acosmi CLI.
///
/// Provides common CLI infrastructure: banner display, global state,
/// progress indicators, config guards, command formatting, and argument
/// parsing utilities.
///
/// Source: `src/globals.ts`, `src/cli/banner.ts`, `src/cli/progress.ts`,
/// `src/cli/command-format.ts`, `src/cli/program/config-guard.ts`, `src/cli/argv.ts`

pub mod argv;
pub mod banner;
pub mod command_format;
pub mod config_guard;
pub mod globals;
pub mod progress;
