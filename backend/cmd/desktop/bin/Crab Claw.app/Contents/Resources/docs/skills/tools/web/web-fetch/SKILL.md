---
name: web-fetch
description: "Fetch and extract readable content from URLs with Firecrawl fallback for anti-bot sites"
tools: web_fetch
metadata:
  tree_id: "web/web_fetch"
  tree_group: "web"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Web Fetch — URL Content Extraction

## Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `url` | yes | — | Target URL (http/https only) |
| `extractMode` | no | `markdown` | Output format: `markdown` \| `text` |
| `maxChars` | no | — | Truncate output to N characters |

## Extraction Pipeline

1. **Readability** (local, default) — fast HTML-to-markdown
2. **Firecrawl** (if configured) — handles anti-bot, caches results
3. **Basic HTML cleanup** (last resort)

## Firecrawl Config

```json5
{
  tools: {
    web: {
      fetch: {
        firecrawl: {
          apiKey: "KEY",
          onlyMainContent: true,
          maxAgeMs: 172800000,  // 2-day cache
          timeoutSeconds: 60
        }
      }
    }
  }
}
```

- Uses `proxy: "auto"` + `storeInCache: true`
- Auto-retries with stealth mode if basic request fails

## Security

- Blocks private/internal hostnames
- Re-checks redirects (configurable limit)
- Best-effort extraction; JS-heavy sites may need `browser` tool
