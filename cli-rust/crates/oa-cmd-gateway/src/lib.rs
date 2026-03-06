/// Gateway lifecycle commands for Claw Acosmi CLI.
///
/// Provides subcommands for managing the Gateway service: start, stop,
/// restart, status, install, uninstall, call, usage-cost, health, probe,
/// and discover.
///
/// Source: `src/cli/gateway-cli.ts`, `src/commands/gateway*.ts`

pub mod call;
pub mod discover;
pub mod health;
pub mod install;
pub mod probe;
pub mod run;
pub mod start;
pub mod status;
pub mod stop;
pub mod uninstall;
pub mod usage_cost;
