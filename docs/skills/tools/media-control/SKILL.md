---
name: media-control
description: "媒体运营控制台：media.* 热点、草稿、发布与配置治理"
---

# 媒体运营控制台

用于媒体子系统的热点抓取、草稿生命周期、发布记录和工具开关管理。

## 覆盖方法
- `media.trending.fetch`
- `media.trending.sources`
- `media.trending.health`
- `media.drafts.list`
- `media.drafts.get`
- `media.drafts.update`
- `media.drafts.approve`
- `media.drafts.delete`
- `media.publish.list`
- `media.publish.get`
- `media.config.get`
- `media.config.update`
- `media.tools.list`
- `media.tools.toggle`
- `media.sources.toggle`

## 推荐流程
1. 先看 `media.trending.health` 与 `media.trending.sources`。
2. 拉取热点后用 `media.drafts.*` 管理内容草稿。
3. 发布前做 `media.drafts.approve`，发布后核对 `media.publish.*`。
4. 用 `media.tools.toggle` 和 `media.sources.toggle` 控制能力范围。
5. 用 `media.config.get/update` 维护子智能体配置。

## 风险控制
- 生产发布前保留草稿快照。
- 不在同一窗口同时切换多项工具和数据源。

## 成功判定
- 热点抓取稳定、草稿流转正常、发布记录可追踪。
