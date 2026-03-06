/// Supporting commands for Claw Acosmi CLI.
///
/// This crate implements ancillary commands that are not part of the core
/// agent/model/channel pipelines: dashboard, docs search, reset, setup,
/// uninstall, message delivery, cleanup utilities, daemon install helpers,
/// signal-cli installation, and systemd linger management.
///
/// Source: `src/commands/dashboard.ts`, `src/commands/docs.ts`,
///         `src/commands/reset.ts`, `src/commands/setup.ts`,
///         `src/commands/uninstall.ts`, `src/commands/message.ts`,
///         `src/commands/message-format.ts`, `src/commands/cleanup-utils.ts`,
///         `src/commands/daemon-install-helpers.ts`, `src/commands/daemon-runtime.ts`,
///         `src/commands/signal-install.ts`, `src/commands/systemd-linger.ts`

pub mod cleanup_utils;
pub mod daemon_install_helpers;
pub mod daemon_runtime;
pub mod dashboard;
pub mod docs;
pub mod message;
pub mod message_format;
pub mod reset;
pub mod setup;
pub mod signal_install;
pub mod systemd_linger;
pub mod uninstall;
