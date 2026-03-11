package media

// ============================================================================
// media/trending.go — 热点源聚合模块
// 定义 TrendingSource 接口 + TrendingAggregator 组合器。
//
// Design doc: docs/xinshenji/impl-tracking-media-subagent.md §P0-6
// ============================================================================

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

// ---------- 接口 ----------

// TrendingSource 热点数据源接口。
// 每个具体实现（微博、百度、知乎等）在 Phase 1 中实现。
type TrendingSource interface {
	// Fetch 拉取指定分类下的热点话题，limit ≤ 0 时使用源默认值。
	Fetch(ctx context.Context, category string, limit int) ([]TrendingTopic, error)
	// Name 返回数据源标识（如 "weibo", "baidu"）。
	Name() string
}

// ---------- 聚合器 ----------

// TrendingAggregator 组合多个 TrendingSource，并发拉取后聚合。
type TrendingAggregator struct {
	mu      sync.RWMutex
	sources []TrendingSource
}

// NewTrendingAggregator 创建热点聚合器。
func NewTrendingAggregator(sources ...TrendingSource) *TrendingAggregator {
	return &TrendingAggregator{sources: sources}
}

// AddSource 动态添加数据源。
func (a *TrendingAggregator) AddSource(src TrendingSource) {
	if src == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sources = append(a.sources, src)
}

// SetSources 原子替换全部热点源。
func (a *TrendingAggregator) SetSources(sources []TrendingSource) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sources = make([]TrendingSource, 0, len(sources))
	for _, src := range sources {
		if src != nil {
			a.sources = append(a.sources, src)
		}
	}
}

// SourceNames 返回已注册数据源名称列表。
func (a *TrendingAggregator) SourceNames() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	names := make([]string, len(a.sources))
	for i, src := range a.sources {
		names[i] = src.Name()
	}
	return names
}

// FetchResult 单个数据源的拉取结果。
type FetchResult struct {
	Source string
	Topics []TrendingTopic
	Err    error
}

// FetchAll 并发拉取所有源，聚合结果按 HeatScore 降序。
// 单个源失败不影响其他源，错误通过 FetchResult.Err 返回。
func (a *TrendingAggregator) FetchAll(
	ctx context.Context,
	category string,
	limit int,
) ([]TrendingTopic, []FetchResult) {
	a.mu.RLock()
	srcs := make([]TrendingSource, len(a.sources))
	copy(srcs, a.sources)
	a.mu.RUnlock()

	if len(srcs) == 0 {
		return nil, nil
	}

	results := make([]FetchResult, len(srcs))
	var wg sync.WaitGroup
	wg.Add(len(srcs))

	for i, src := range srcs {
		go func(idx int, s TrendingSource) {
			defer wg.Done()
			topics, err := s.Fetch(ctx, category, limit)
			results[idx] = FetchResult{
				Source: s.Name(),
				Topics: topics,
				Err:    err,
			}
			if err != nil {
				slog.Warn("trending source fetch failed",
					"source", s.Name(),
					"error", err,
				)
			}
		}(i, src)
	}
	wg.Wait()

	// Aggregate all successful topics.
	var all []TrendingTopic
	for _, r := range results {
		if r.Err == nil {
			all = append(all, r.Topics...)
		}
	}

	// Sort by HeatScore descending.
	sort.Slice(all, func(i, j int) bool {
		return all[i].HeatScore > all[j].HeatScore
	})

	// Apply global limit if requested.
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	return all, results
}

// FetchBySource 拉取指定数据源的热点。
// Lock is released before calling Fetch to avoid blocking AddSource.
func (a *TrendingAggregator) FetchBySource(
	ctx context.Context,
	sourceName, category string,
	limit int,
) ([]TrendingTopic, error) {
	a.mu.RLock()
	var found TrendingSource
	for _, src := range a.sources {
		if src.Name() == sourceName {
			found = src
			break
		}
	}
	a.mu.RUnlock()

	if found == nil {
		return nil, fmt.Errorf("trending source %q not found", sourceName)
	}
	return found.Fetch(ctx, category, limit)
}
