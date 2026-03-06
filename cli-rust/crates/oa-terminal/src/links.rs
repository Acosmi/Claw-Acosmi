/// Terminal hyperlink formatting.
///
/// Supports OSC-8 hyperlinks and plain-text fallbacks.
///
/// Source: `src/terminal/links.ts`, `src/utils.ts`

/// Root URL for Claw Acosmi documentation.
pub const DOCS_ROOT: &str = "https://github.com/Acosmi/Claw-Acosmi/tree/main/docs";

/// Format an OSC-8 terminal hyperlink.
///
/// If the terminal supports hyperlinks (TTY), wraps the label in an OSC-8 sequence.
/// Otherwise, returns a plain-text fallback.
pub fn format_terminal_link(label: &str, url: &str, force: Option<bool>) -> String {
    let allow = match force {
        Some(true) => true,
        Some(false) => false,
        None => std::io::IsTerminal::is_terminal(&std::io::stdout()),
    };

    if allow {
        format!("\x1b]8;;{url}\x1b\\{label}\x1b]8;;\x1b\\")
    } else {
        format!("{label} ({url})")
    }
}

/// Format a documentation link.
pub fn format_docs_link(path: &str, label: Option<&str>) -> String {
    let url = format!("{DOCS_ROOT}/{}", path.trim_start_matches('/'));
    let display = label.unwrap_or(path);
    format_terminal_link(display, &url, None)
}

/// Format a link to the documentation root.
pub fn format_docs_root_link(label: Option<&str>) -> String {
    let display = label.unwrap_or("OpenAcosmi Docs");
    format_terminal_link(display, DOCS_ROOT, None)
}

/// Shorten home directory references in a path string for display.
///
/// Replaces the home directory prefix with `~` for cleaner output.
pub fn display_path(input: &str) -> String {
    if let Some(home) = dirs::home_dir() {
        if let Some(home_str) = home.to_str() {
            if input.starts_with(home_str) {
                return format!("~{}", &input[home_str.len()..]);
            }
        }
    }
    input.to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_format_terminal_link_plain() {
        let result = format_terminal_link("click", "https://example.com", Some(false));
        assert_eq!(result, "click (https://example.com)");
    }

    #[test]
    fn test_format_terminal_link_osc8() {
        let result = format_terminal_link("click", "https://example.com", Some(true));
        assert!(result.contains("\x1b]8;;"));
        assert!(result.contains("https://example.com"));
        assert!(result.contains("click"));
    }
}
