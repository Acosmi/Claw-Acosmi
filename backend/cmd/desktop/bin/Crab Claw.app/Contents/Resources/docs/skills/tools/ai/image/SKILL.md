---
name: image
description: "Analyze images using configured vision model with provider auto-selection"
tools: image
metadata:
  tree_id: "ai/image"
  tree_group: "ai"
  min_tier: "task_multimodal"
  approval_type: "none"
---

# Image — Vision Analysis

## Usage Guide

- Analyze images using the configured image/vision model
- Supports multiple providers (OpenAI, Ollama, etc.)
- No approval required

## Configuration Flow

1. `image.config.get` — check current provider/model
2. `image.config.set` — minimal configuration change
3. `image.test` — E2E validation with test samples
4. `image.models` / `image.ollama.models` — verify available models

## Rules

- Model switch → must run test samples for comparison
- Live failure → verify provider online before config change
