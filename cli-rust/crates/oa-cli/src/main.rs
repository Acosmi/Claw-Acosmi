/// Claw Acosmi CLI — Rust implementation.
///
/// Binary entry point with Clap-based command routing. Connects all command
/// crates through a `#[derive(Subcommand)]` enum and dispatches to the
/// appropriate entry function.
///
/// Source: `backend/cmd/openacosmi/main.go`, `src/cli/index.ts`
mod commands;

use std::process::ExitCode;

use clap::Parser;
use tracing::error;
use tracing_subscriber::EnvFilter;

use commands::Commands;

// ---------------------------------------------------------------------------
// Root CLI definition
// ---------------------------------------------------------------------------

/// Claw Acosmi CLI — orchestrate AI agents, channels, models, and more.
#[derive(Debug, Parser)]
#[command(
    name = "openacosmi",
    version = env!("CARGO_PKG_VERSION"),
    about = "Claw Acosmi CLI — orchestrate AI agents, channels, models, and more",
    long_about = None,
    propagate_version = true,
    subcommand_required = true,
    arg_required_else_help = true,
)]
pub struct Cli {
    #[command(subcommand)]
    command: Commands,

    /// Use the dev profile (isolate state under ~/.openacosmi-dev).
    #[arg(long, global = true)]
    dev: bool,

    /// Use a named profile (isolate state under ~/.openacosmi-<name>).
    #[arg(long, global = true)]
    profile: Option<String>,

    /// Enable verbose output.
    #[arg(short, long, global = true)]
    verbose: bool,

    /// Output as JSON where supported.
    #[arg(long, global = true)]
    json: bool,

    /// Disable ANSI colors.
    #[arg(long, global = true)]
    no_color: bool,

    /// UI language override (e.g. zh-CN, en-US).
    #[arg(long, global = true)]
    lang: Option<String>,
}

// ---------------------------------------------------------------------------
// Pre-run setup
// ---------------------------------------------------------------------------

/// Initialize the tracing subscriber with default filter from the
/// `OPENACOSMI_LOG` environment variable (or `warn` if unset).
fn init_tracing(verbose: bool) {
    let default_level = if verbose { "debug" } else { "warn" };
    let filter =
        EnvFilter::try_from_env("OPENACOSMI_LOG").unwrap_or_else(|_| EnvFilter::new(default_level));

    tracing_subscriber::fmt()
        .with_env_filter(filter)
        .with_target(false)
        .without_time()
        .with_writer(std::io::stderr)
        .init();
}

/// Apply global settings (profile, dev mode, color) based on root flags.
///
/// Uses `unsafe` because `std::env::set_var` is unsafe in Rust 2024 edition.
/// This is safe because it runs before the tokio runtime spawns worker threads.
fn apply_global_flags(cli: &Cli) {
    // SAFETY: called before any threads are spawned (pre-tokio-main).
    unsafe {
        if cli.dev {
            std::env::set_var("OPENACOSMI_PROFILE", "dev");
        } else if let Some(ref profile) = cli.profile {
            std::env::set_var("OPENACOSMI_PROFILE", profile);
        }

        if cli.no_color {
            std::env::set_var("NO_COLOR", "1");
        }

        if let Some(ref lang) = cli.lang {
            std::env::set_var("OPENACOSMI_LANG", lang);
        }
    }
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

fn main() -> ExitCode {
    let cli = Cli::parse();

    init_tracing(cli.verbose);

    // Apply env vars before the async runtime starts any threads.
    apply_global_flags(&cli);

    // Build the tokio runtime manually so env vars are set first.
    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build();

    let rt = match rt {
        Ok(rt) => rt,
        Err(e) => {
            eprintln!("Error: failed to start async runtime: {e}");
            return ExitCode::FAILURE;
        }
    };

    rt.block_on(async {
        match commands::dispatch(cli.command, cli.json, cli.verbose).await {
            Ok(()) => ExitCode::SUCCESS,
            Err(e) => {
                error!("{e:#}");
                eprintln!("Error: {e:#}");
                ExitCode::FAILURE
            }
        }
    })
}
