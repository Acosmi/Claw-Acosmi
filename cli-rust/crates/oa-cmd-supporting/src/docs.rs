/// Docs search command implementation.
///
/// Searches the Claw Acosmi documentation using the `mcporter` MCP tool
/// and formats the results for CLI output. Supports both rich (terminal)
/// and plain markdown output modes.
///
/// Source: `src/commands/docs.ts`

use std::process::Stdio;

use anyhow::{bail, Result};
use tracing::{error, info};

use oa_cli_shared::command_format::format_cli_command;

/// The MCP tool identifier for searching docs.
///
/// Source: `src/commands/docs.ts` - `SEARCH_TOOL`
const SEARCH_TOOL: &str = "https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/mcp.SearchOpenAcosmi";

/// Default timeout for doc search in milliseconds.
///
/// Source: `src/commands/docs.ts` - `SEARCH_TIMEOUT_MS`
const SEARCH_TIMEOUT_MS: u64 = 30_000;

/// Maximum snippet length for display.
///
/// Source: `src/commands/docs.ts` - `DEFAULT_SNIPPET_MAX`
const DEFAULT_SNIPPET_MAX: usize = 220;

/// A single documentation search result.
///
/// Source: `src/commands/docs.ts` - `DocResult`
#[derive(Debug, Clone)]
pub struct DocResult {
    /// The page title.
    pub title: String,
    /// The documentation link URL.
    pub link: String,
    /// Optional snippet of the page content.
    pub snippet: Option<String>,
}

/// Extract a value from a line starting with a given prefix.
///
/// Source: `src/commands/docs.ts` - `extractLine`
fn extract_line(lines: &[&str], prefix: &str) -> Option<String> {
    for line in lines {
        if let Some(rest) = line.strip_prefix(prefix) {
            let trimmed = rest.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_owned());
            }
        }
    }
    None
}

/// Normalize a snippet to a maximum length.
///
/// Collapses whitespace and truncates with an ellipsis if necessary.
///
/// Source: `src/commands/docs.ts` - `normalizeSnippet`
fn normalize_snippet(raw: Option<&str>, fallback: &str) -> String {
    let base = match raw.filter(|r| !r.trim().is_empty()) {
        Some(r) => r,
        None => fallback,
    };

    let cleaned: String = base.split_whitespace().collect::<Vec<_>>().join(" ");
    if cleaned.is_empty() {
        return String::new();
    }
    if cleaned.len() <= DEFAULT_SNIPPET_MAX {
        return cleaned;
    }
    let truncated: String = cleaned.chars().take(DEFAULT_SNIPPET_MAX - 3).collect();
    format!("{truncated}...")
}

/// Extract the first paragraph from text.
///
/// Source: `src/commands/docs.ts` - `firstParagraph`
fn first_paragraph(text: &str) -> String {
    for part in text.split("\n\n") {
        let trimmed = part.trim();
        if !trimmed.is_empty() {
            return trimmed.to_owned();
        }
    }
    String::new()
}

/// Parse raw search output into structured `DocResult` items.
///
/// Source: `src/commands/docs.ts` - `parseSearchOutput`
pub fn parse_search_output(raw: &str) -> Vec<DocResult> {
    let normalized = raw.replace('\r', "");
    let blocks: Vec<&str> = normalized
        .split("\nTitle: ")
        .collect();

    let mut results = Vec::new();

    for (i, block) in blocks.iter().enumerate() {
        let block = block.trim();
        if block.is_empty() {
            continue;
        }
        // The first block may not start with "Title: " since split removed it
        let full_block = if i == 0 && block.starts_with("Title: ") {
            block.strip_prefix("Title: ").unwrap_or(block).to_owned()
        } else if i == 0 {
            // First block before any "Title:" -- check if it starts with it
            if let Some(stripped) = block.strip_prefix("Title: ") {
                stripped.to_owned()
            } else {
                continue;
            }
        } else {
            block.to_owned()
        };

        let lines: Vec<&str> = full_block.lines().collect();
        // First line is the title (after "Title: " was stripped)
        let title = lines.first().map(|l| l.trim().to_owned()).unwrap_or_default();
        if title.is_empty() {
            continue;
        }

        let link = extract_line(&lines[1..], "Link:");
        let link = match link {
            Some(l) => l,
            None => continue,
        };

        let content = extract_line(&lines[1..], "Content:");
        let content_index = lines[1..]
            .iter()
            .position(|line| line.starts_with("Content:"));
        let body = content_index.map_or(String::new(), |idx| {
            let idx = idx + 2; // offset from the sliced lines
            if idx < lines.len() {
                lines[idx..].join("\n").trim().to_owned()
            } else {
                String::new()
            }
        });

        let snippet = normalize_snippet(content.as_deref(), &first_paragraph(&body));
        results.push(DocResult {
            title,
            link,
            snippet: if snippet.is_empty() {
                None
            } else {
                Some(snippet)
            },
        });
    }

    results
}

/// Escape characters that are special in Markdown.
///
/// Source: `src/commands/docs.ts` - `escapeMarkdown`
fn escape_markdown(text: &str) -> String {
    let mut result = String::with_capacity(text.len());
    for ch in text.chars() {
        if matches!(ch, '(' | ')' | '[' | ']') {
            result.push('\\');
        }
        result.push(ch);
    }
    result
}

/// Build a markdown document from search results.
///
/// Source: `src/commands/docs.ts` - `buildMarkdown`
pub fn build_markdown(query: &str, results: &[DocResult]) -> String {
    let mut lines = vec![format!("# Docs search: {}", escape_markdown(query)), String::new()];

    if results.is_empty() {
        lines.push("_No results._".to_owned());
        return lines.join("\n");
    }

    for item in results {
        let title = escape_markdown(&item.title);
        let suffix = item
            .snippet
            .as_ref()
            .map(|s| format!(" - {}", escape_markdown(s)))
            .unwrap_or_default();
        lines.push(format!("- [{title}]({}){}",item.link, suffix));
    }

    lines.join("\n")
}

/// Check whether a binary is available on PATH.
///
/// Source: `src/commands/docs.ts` - `hasBinary` (via agents/skills)
fn has_binary(name: &str) -> bool {
    which::which(name).is_ok()
}

/// Run the docs search command.
///
/// Searches the Claw Acosmi documentation and prints results. When called
/// without a query, prints links to the docs site and search usage.
///
/// Source: `src/commands/docs.ts` - `docsSearchCommand`
pub async fn docs_search_command(query_parts: &[String]) -> Result<()> {
    let query = query_parts
        .iter()
        .map(String::as_str)
        .collect::<Vec<_>>()
        .join(" ")
        .trim()
        .to_owned();

    if query.is_empty() {
        info!("Docs: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs");
        info!(
            "Search: {}",
            format_cli_command("openacosmi docs \"your query\"")
        );
        return Ok(());
    }

    // Run mcporter search tool
    let payload = serde_json::json!({ "query": query });
    let payload_str = payload.to_string();

    let (cmd, args) = if has_binary("mcporter") {
        (
            "mcporter".to_owned(),
            vec![
                "call".to_owned(),
                SEARCH_TOOL.to_owned(),
                "--args".to_owned(),
                payload_str,
                "--output".to_owned(),
                "text".to_owned(),
            ],
        )
    } else if has_binary("pnpm") {
        (
            "pnpm".to_owned(),
            vec![
                "dlx".to_owned(),
                "mcporter".to_owned(),
                "call".to_owned(),
                SEARCH_TOOL.to_owned(),
                "--args".to_owned(),
                payload_str,
                "--output".to_owned(),
                "text".to_owned(),
            ],
        )
    } else if has_binary("npx") {
        (
            "npx".to_owned(),
            vec![
                "-y".to_owned(),
                "mcporter".to_owned(),
                "call".to_owned(),
                SEARCH_TOOL.to_owned(),
                "--args".to_owned(),
                payload_str,
                "--output".to_owned(),
                "text".to_owned(),
            ],
        )
    } else {
        bail!("Missing pnpm or npx; install a Node package runner.");
    };

    let result = tokio::time::timeout(
        std::time::Duration::from_millis(SEARCH_TIMEOUT_MS),
        tokio::process::Command::new(&cmd)
            .args(&args)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .output(),
    )
    .await;

    let output = match result {
        Ok(Ok(output)) => output,
        Ok(Err(e)) => bail!("Docs search failed: {e}"),
        Err(_) => bail!("Docs search timed out after {SEARCH_TIMEOUT_MS}ms"),
    };

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        let stdout = String::from_utf8_lossy(&output.stdout);
        let err = if !stderr.trim().is_empty() {
            stderr.trim().to_owned()
        } else if !stdout.trim().is_empty() {
            stdout.trim().to_owned()
        } else {
            format!("exit {}", output.status.code().unwrap_or(-1))
        };
        error!("Docs search failed: {err}");
        bail!("Docs search failed: {err}");
    }

    let stdout = String::from_utf8_lossy(&output.stdout);
    let results = parse_search_output(&stdout);
    let markdown = build_markdown(&query, &results);
    info!("{}", markdown.trim_end());

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_empty_output() {
        let results = parse_search_output("");
        assert!(results.is_empty());
    }

    #[test]
    fn parse_single_result() {
        let raw = "Title: Getting Started\nLink: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/start\nContent: Quick start guide\n";
        let results = parse_search_output(raw);
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].title, "Getting Started");
        assert_eq!(results[0].link, "https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/start");
        assert_eq!(results[0].snippet, Some("Quick start guide".to_owned()));
    }

    #[test]
    fn parse_multiple_results() {
        let raw = "Title: First\nLink: https://example.com/1\nContent: Content one\n\nTitle: Second\nLink: https://example.com/2\nContent: Content two\n";
        let results = parse_search_output(raw);
        assert_eq!(results.len(), 2);
        assert_eq!(results[0].title, "First");
        assert_eq!(results[1].title, "Second");
    }

    #[test]
    fn normalize_snippet_short() {
        let result = normalize_snippet(Some("Short text"), "fallback");
        assert_eq!(result, "Short text");
    }

    #[test]
    fn normalize_snippet_fallback() {
        let result = normalize_snippet(None, "fallback text");
        assert_eq!(result, "fallback text");
    }

    #[test]
    fn normalize_snippet_empty() {
        let result = normalize_snippet(Some(""), "");
        assert_eq!(result, "");
    }

    #[test]
    fn normalize_snippet_truncation() {
        let long = "a".repeat(300);
        let result = normalize_snippet(Some(&long), "");
        assert!(result.len() <= DEFAULT_SNIPPET_MAX);
        assert!(result.ends_with("..."));
    }

    #[test]
    fn normalize_snippet_whitespace_collapse() {
        let result = normalize_snippet(Some("  hello   world  "), "");
        assert_eq!(result, "hello world");
    }

    #[test]
    fn first_paragraph_extraction() {
        let text = "First paragraph.\n\nSecond paragraph.";
        assert_eq!(first_paragraph(text), "First paragraph.");
    }

    #[test]
    fn first_paragraph_single() {
        assert_eq!(first_paragraph("Only one"), "Only one");
    }

    #[test]
    fn escape_markdown_brackets() {
        assert_eq!(escape_markdown("test(1)"), "test\\(1\\)");
        assert_eq!(escape_markdown("[link]"), "\\[link\\]");
    }

    #[test]
    fn build_markdown_empty_results() {
        let md = build_markdown("test query", &[]);
        assert!(md.contains("# Docs search: test query"));
        assert!(md.contains("_No results._"));
    }

    #[test]
    fn build_markdown_with_results() {
        let results = vec![DocResult {
            title: "Page Title".to_owned(),
            link: "https://example.com".to_owned(),
            snippet: Some("A snippet".to_owned()),
        }];
        let md = build_markdown("test", &results);
        assert!(md.contains("Page Title"));
        assert!(md.contains("https://example.com"));
        assert!(md.contains("A snippet"));
    }
}
