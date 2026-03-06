/// Daemon commands for Claw Acosmi CLI (legacy alias for gateway service).
///
/// Provides subcommands that mirror the gateway service lifecycle:
/// status, start, stop, restart, install, uninstall.
/// These are legacy aliases — new code should use `gateway` subcommands.
///
/// Source: `src/cli/daemon-cli.ts`

pub mod commands;
