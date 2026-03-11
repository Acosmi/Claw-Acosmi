---
name: cron
description: "Manage scheduled tasks and wake events with explicit delivery targeting"
tools: cron
metadata:
  tree_id: "system/cron"
  tree_group: "system"
  min_tier: "task_multimodal"
  approval_type: "plan_confirm"
---

# Cron — Scheduled Task Management

## Workflow

1. `cron.list` → check for conflicts with existing schedules
2. `cron.add` → create task (minimal, auditable)
3. `cron.status` → verify scheduler health
4. `cron.run` → manual trigger to verify output + delivery
5. `cron.runs` → audit history, errors, delivery results
6. `cron.update` / `cron.remove` → maintain or retire

## Key Principles

- **Low-frequency-first**: avoid burst scheduling
- **Explicit delivery**: always set `channel` + `to`, never rely on vague `last`
- **Auditable**: task name, purpose, output path must be traceable
- `wake` wakes existing queues — does not replace scheduling design

## Common Pitfalls

- `status` healthy ≠ business logic correct → always check `cron.runs`
- Manual run works ≠ unattended works → often missing delivery target
- File outputs need separate `send_media` delivery design
