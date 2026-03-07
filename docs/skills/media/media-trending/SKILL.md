---
name: media-trending
description: "热点追踪与选题分析，从微博/百度/知乎热搜中发掘创作方向"
metadata:
  openacosmi:
    agent_type: media
    phase: "5A"
user-invocable: true
disable-model-invocation: false
tools: trending_topics
---

# 热点追踪与选题分析

媒体运营子智能体专属技能。指导如何利用 `trending_topics` 工具从多平台热搜中发掘有价值的创作方向。

## 数据源

| 源 | API | 特点 |
|---|---|---|
| **weibo** | 微博实时热搜 | 娱乐/社会新闻为主，热度数值大 |
| **baidu** | 百度热搜榜 | 综合搜索热度，支持分类 tab |
| **zhihu** | 知乎热榜 | 深度讨论型话题，适合知识类内容 |

## 工作流程

### 1. 全源扫描

先用 `list_sources` 确认可用源，再 `fetch` 全量拉取：

```
trending_topics { action: "list_sources" }
trending_topics { action: "fetch", limit: 30 }
```

### 2. 按源定向

对特定平台内容，指定源拉取：

```
trending_topics { action: "fetch", source: "weibo", limit: 20 }
trending_topics { action: "fetch", source: "zhihu", limit: 20 }
```

### 3. 分类筛选

百度支持分类过滤：

| category | 对应 |
|---|---|
| tech / science | 科技 |
| finance | 财经 |
| entertainment | 娱乐 |
| sports | 体育 |
| game | 游戏 |

```
trending_topics { action: "fetch", source: "baidu", category: "tech" }
```

### 4. 热点分析

获取热点后用 `analyze` 提取关键信息：

```
trending_topics { action: "analyze", topic: "话题标题" }
```

## 选题策略

### 热度判断

- `heat_score >= 100万`：顶级热点，竞争激烈，需独特角度
- `heat_score 10万~100万`：中高热点，最佳创作窗口
- `heat_score 1万~10万`：新兴话题，抢占先机

### 平台匹配

| 话题类型 | 推荐平台 | 理由 |
|---|---|---|
| 热门事件深度解读 | 微信公众号 | 长篇深度分析 + HTML 排版 |
| 生活方式/种草/测评 | 小红书 | 图文并茂 + 标签传播 + 互动强 |
| 技术/知识科普 | 微信公众号 | 专业深度 + 订阅推送 |
| 娱乐八卦/潮流 | 小红书 | 口语化 + 快速传播 |
| 行业资讯/政策解读 | 微信公众号 | 权威感 + 结构化 |
| 好物推荐/合集 | 小红书 | 视觉优先 + 收藏率高 |

### 跨平台策略

同一热点可同时产出两个平台的内容，但**风格必须差异化**：

| | 小红书版 | 微信公众号版 |
|---|---|---|
| 标题 | ≤20字，口语化 | ≤64字，信息化 |
| 正文 | ≤1000字，分点 + emoji | 1500~3000字，结构化论述 |
| 风格 | casual | informative / professional |
| 标签 | 3~5个 `#话题#` | 不强制 |
| 图片 | 核心主角（≤9张） | 辅助说明 |

### 选题原则

1. **时效性**：热搜 2 小时内为黄金窗口
2. **关联性**：与账号定位/领域匹配
3. **差异化**：找独特切入角度，避免同质化
4. **可操作**：确保有足够素材和观点支撑
5. **合规性**：避免敏感话题、未经证实的消息

## 输出规范

选题分析结果应包含：

- 话题标题 + 热度
- 推荐创作角度（2-3 个）
- 目标平台
- 预估内容风格（informative / casual / professional）
- 参考链接

## 注意事项

- 热搜 API 为公开接口，可能受限流影响
- 微博/知乎要求 User-Agent 头
- 热度数值跨平台不可直接对比（量纲不同）
- 同一话题不同源热度差异大属正常现象
