---
name: web-search
description: "Search the web via Brave/Perplexity API for real-time information retrieval"
tools: web_search
metadata:
  tree_id: "web/web_search"
  tree_group: "web"
  min_tier: "task_light"
  approval_type: "none"
---

# Web Search

## Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `query` | yes | — | Search query string |
| `count` | no | 5 | Number of results (1–10) |
| `country` | no | — | Country code filter |
| `search_lang` | no | — | Search language |
| `ui_lang` | no | — | UI language |
| `freshness` | no | — | Brave only: `pd`/`pw`/`pm`/`py` or date range |

## Providers

| Provider | Config Key | API Key |
|----------|-----------|---------|
| Brave (default) | `tools.web.search.provider: "brave"` | `BRAVE_API_KEY` or `tools.web.search.apiKey` |
| Perplexity | `tools.web.search.provider: "perplexity"` | `PERPLEXITY_API_KEY` or `tools.web.search.perplexity.apiKey` |

## When to Use

- **web_search**: API-based, fast, structured results — use for factual queries, current events
- **web_fetch**: extract readable content from a known URL
- **browser**: DOM interaction, JS-heavy SPAs, login-required pages

## Notes

- Results cached for 15 minutes (configurable)
- No approval required; read-only operation
