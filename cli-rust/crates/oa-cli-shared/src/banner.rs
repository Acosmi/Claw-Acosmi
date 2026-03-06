/// CLI banner display.
///
/// Renders the Claw Acosmi ASCII art banner and single-line version info
/// to the terminal. Tracks whether the banner has already been emitted
/// to avoid duplicate output.
///
/// Source: `src/cli/banner.ts`

use std::sync::atomic::{AtomicBool, Ordering};

use oa_terminal::ansi::visible_width;
use oa_terminal::theme::{is_rich, Theme};

/// Tracks whether the banner has already been emitted this process.
///
/// Source: `src/cli/banner.ts` – `bannerEmitted`
static EMITTED: AtomicBool = AtomicBool::new(false);

/// ASCII art block for the banner.
///
/// Source: `src/cli/banner.ts` – `LOBSTER_ASCII`
const LOBSTER_ASCII: &[&str] = &[
    "\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}\u{2584}",
    "\u{2588}\u{2588}\u{2591}\u{2584}\u{2584}\u{2584}\u{2591}\u{2588}\u{2588}\u{2591}\u{2584}\u{2584}\u{2591}\u{2588}\u{2588}\u{2591}\u{2584}\u{2584}\u{2584}\u{2588}\u{2588}\u{2591}\u{2580}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2591}\u{2584}\u{2584}\u{2580}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2588}\u{2588}\u{2591}\u{2584}\u{2584}\u{2580}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}",
    "\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2591}\u{2580}\u{2580}\u{2591}\u{2588}\u{2588}\u{2591}\u{2584}\u{2584}\u{2584}\u{2588}\u{2588}\u{2591}\u{2588}\u{2591}\u{2588}\u{2591}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2588}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2588}\u{2588}\u{2591}\u{2580}\u{2580}\u{2591}\u{2588}\u{2588}\u{2591}\u{2588}\u{2591}\u{2588}\u{2591}\u{2588}\u{2588}",
    "\u{2588}\u{2588}\u{2591}\u{2580}\u{2580}\u{2580}\u{2591}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2588}\u{2588}\u{2588}\u{2591}\u{2580}\u{2580}\u{2580}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2584}\u{2591}\u{2588}\u{2588}\u{2591}\u{2580}\u{2580}\u{2584}\u{2588}\u{2588}\u{2591}\u{2580}\u{2580}\u{2591}\u{2588}\u{2591}\u{2588}\u{2588}\u{2591}\u{2588}\u{2588}\u{2584}\u{2580}\u{2584}\u{2580}\u{2584}\u{2588}\u{2588}",
    "\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}\u{2580}",
    "                  \u{1F99C} OPENACOSMI \u{1F99C}                    ",
    " ",
];

/// Check whether `--json` or `--json=...` is present in the argument list.
///
/// Source: `src/cli/banner.ts` – `hasJsonFlag`
fn has_json_flag(argv: &[String]) -> bool {
    argv.iter()
        .any(|arg| arg == "--json" || arg.starts_with("--json="))
}

/// Check whether `--version`, `-V`, or `-v` is present in the argument list.
///
/// Source: `src/cli/banner.ts` – `hasVersionFlag`
fn has_version_flag(argv: &[String]) -> bool {
    argv.iter()
        .any(|arg| arg == "--version" || arg == "-V" || arg == "-v")
}

/// Format a single-line CLI banner showing the version string.
///
/// Returns a themed string when the terminal supports rich output,
/// or a plain-text string otherwise. If the full banner line exceeds
/// the terminal width it wraps onto two lines with the tagline indented.
///
/// Source: `src/cli/banner.ts` – `formatCliBannerLine`
pub fn format_cli_banner_line(version: &str) -> String {
    let rich = is_rich();
    let title = "\u{1F99C} OpenAcosmi";
    let prefix_width = 3; // emoji + space visual width
    let columns = terminal_columns();

    let plain_full = format!("{title} {version}");
    let fits = visible_width(&plain_full) <= columns;

    if rich {
        if fits {
            return format!(
                "{} {}",
                Theme::heading(title),
                Theme::info(version),
            );
        }
        let line1 = format!("{} {}", Theme::heading(title), Theme::info(version));
        let line2 = format!("{}{}", " ".repeat(prefix_width), Theme::muted(""));
        return format!("{line1}\n{line2}");
    }

    if fits {
        return plain_full;
    }
    let line1 = format!("{title} {version}");
    let line2 = " ".repeat(prefix_width);
    format!("{line1}\n{line2}")
}

/// Format the ASCII art banner with theme colors.
///
/// Source: `src/cli/banner.ts` – `formatCliBannerArt`
pub fn format_cli_banner_art() -> String {
    let rich = is_rich();
    if !rich {
        return LOBSTER_ASCII.join("\n");
    }

    let colored: Vec<String> = LOBSTER_ASCII
        .iter()
        .map(|line| {
            if line.contains("OPENACOSMI") {
                return format!(
                    "{}{}{}{}",
                    Theme::muted("              "),
                    Theme::accent("\u{1F99C}"),
                    Theme::info(" OPENACOSMI "),
                    Theme::accent("\u{1F99C}"),
                );
            }
            line.chars()
                .map(|ch| match ch {
                    '\u{2588}' => Theme::accent_bright(&ch.to_string()),
                    '\u{2591}' => Theme::accent_dim(&ch.to_string()),
                    '\u{2580}' => Theme::accent(&ch.to_string()),
                    _ => Theme::muted(&ch.to_string()),
                })
                .collect::<String>()
        })
        .collect();

    colored.join("\n")
}

/// Emit the CLI banner to stdout.
///
/// Suppresses the banner when any of these conditions are true:
/// - The banner has already been emitted
/// - stdout is not a TTY
/// - `--json` or `--json=` flags are present
/// - `--version`, `-V`, or `-v` flags are present
/// - `json_mode` is `true`
/// - `version_mode` is `true`
///
/// Source: `src/cli/banner.ts` – `emitCliBanner`
pub fn emit_cli_banner(version: &str, json_mode: bool, version_mode: bool) {
    if EMITTED.load(Ordering::Relaxed) {
        return;
    }
    if !std::io::IsTerminal::is_terminal(&std::io::stdout()) {
        return;
    }
    if json_mode || version_mode {
        EMITTED.store(true, Ordering::Relaxed);
        return;
    }

    let line = format_cli_banner_line(version);
    // Use eprintln for stderr-like behavior consistent with TS writing to stdout
    // but we match the TS pattern of process.stdout.write
    print!("\n{line}\n\n");
    EMITTED.store(true, Ordering::Relaxed);
}

/// Emit the CLI banner, checking the provided argv for suppression flags.
///
/// This mirrors the TS `emitCliBanner` overload that accepts `options.argv`.
///
/// Source: `src/cli/banner.ts` – `emitCliBanner`
pub fn emit_cli_banner_with_argv(version: &str, argv: &[String]) {
    if EMITTED.load(Ordering::Relaxed) {
        return;
    }
    if !std::io::IsTerminal::is_terminal(&std::io::stdout()) {
        return;
    }
    if has_json_flag(argv) {
        return;
    }
    if has_version_flag(argv) {
        return;
    }

    let line = format_cli_banner_line(version);
    print!("\n{line}\n\n");
    EMITTED.store(true, Ordering::Relaxed);
}

/// Check whether the CLI banner has already been emitted.
///
/// Source: `src/cli/banner.ts` – `hasEmittedCliBanner`
pub fn has_emitted_cli_banner() -> bool {
    EMITTED.load(Ordering::Relaxed)
}

/// Reset the emitted flag. Intended only for testing.
#[cfg(test)]
pub(crate) fn reset_emitted() {
    EMITTED.store(false, Ordering::Relaxed);
}

/// Determine the terminal column width, defaulting to 120.
fn terminal_columns() -> usize {
    console::Term::stdout()
        .size_checked()
        .map(|(_rows, cols)| cols as usize)
        .unwrap_or(120)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn banner_line_contains_version() {
        let line = format_cli_banner_line("1.2.3");
        assert!(line.contains("1.2.3"));
    }

    #[test]
    fn banner_line_contains_openacosmi() {
        let line = format_cli_banner_line("0.0.1");
        assert!(line.contains("OpenAcosmi") || line.contains("OPENACOSMI"));
    }

    #[test]
    fn has_json_flag_detects_json() {
        let argv = vec!["openacosmi".to_string(), "--json".to_string()];
        assert!(has_json_flag(&argv));
    }

    #[test]
    fn has_json_flag_detects_json_equals() {
        let argv = vec!["openacosmi".to_string(), "--json=true".to_string()];
        assert!(has_json_flag(&argv));
    }

    #[test]
    fn has_json_flag_negative() {
        let argv = vec!["openacosmi".to_string(), "status".to_string()];
        assert!(!has_json_flag(&argv));
    }

    #[test]
    fn has_version_flag_detects_flags() {
        for flag in &["--version", "-V", "-v"] {
            let argv = vec!["openacosmi".to_string(), (*flag).to_string()];
            assert!(has_version_flag(&argv), "failed for {flag}");
        }
    }

    #[test]
    fn has_version_flag_negative() {
        let argv = vec!["openacosmi".to_string(), "status".to_string()];
        assert!(!has_version_flag(&argv));
    }

    #[test]
    fn emitted_flag_is_tracked() {
        reset_emitted();
        assert!(!has_emitted_cli_banner());
    }

    #[test]
    fn banner_art_produces_output() {
        let art = format_cli_banner_art();
        assert!(!art.is_empty());
        assert!(art.contains("OPENACOSMI"));
    }
}
