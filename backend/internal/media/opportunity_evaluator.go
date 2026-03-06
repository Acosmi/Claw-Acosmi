package media

// ============================================================================
// media/opportunity_evaluator.go — 热点机会评估引擎
//
// 纯函数评估引擎，供 cron 巡检 agent 调用来判断哪些热点话题值得自动创作。
// 评估维度: 热度分(40) + 新鲜度分(30) + 多源确认分(20) + 时效分(10) = 满分100。
// ============================================================================

import (
	"sort"
	"strings"
	"time"
)

// ---------- 类型 ----------

// OpportunityAction 建议动作。
type OpportunityAction string

const (
	ActionSkip   OpportunityAction = "skip"   // 跳过
	ActionWatch  OpportunityAction = "watch"  // 观望
	ActionCreate OpportunityAction = "create" // 建议创作
)

// OpportunityScore 机会评分结果。
type OpportunityScore struct {
	Topic   TrendingTopic     `json:"topic"`
	Score   float64           `json:"score"`   // 0-100 综合评分
	Reasons []string          `json:"reasons"` // 评分原因列表
	Action  OpportunityAction `json:"action"`  // 建议动作
}

// OpportunityConfig 评估配置。
type OpportunityConfig struct {
	HeatThreshold   float64 // 热度阈值（低于此值跳过，默认 10000）
	MaxTopicsPerRun int     // 单次最多建议创作数（默认 3）
	CooldownHours   int     // 同源话题冷却时间（默认 24h, 保留扩展）
}

// DefaultOpportunityConfig 返回默认评估配置。
func DefaultOpportunityConfig() OpportunityConfig {
	return OpportunityConfig{
		HeatThreshold:   10000,
		MaxTopicsPerRun: 3,
		CooldownHours:   24,
	}
}

// ---------- 评估引擎 ----------

// EvaluateOpportunities 评估热点话题列表，返回按分数降序排列的评估结果。
// 当 MaxTopicsPerRun > 0 时，仅返回 ActionCreate 的前 N 个结果加上所有非 Create 结果。
func EvaluateOpportunities(
	topics []TrendingTopic,
	stateStore MediaStateStore,
	cfg OpportunityConfig,
) []OpportunityScore {
	if len(topics) == 0 {
		return nil
	}

	now := time.Now()

	// 构建多源确认索引：标题 → 出现次数
	sourceCountMap := buildSourceCountMap(topics)

	results := make([]OpportunityScore, 0, len(topics))
	for _, topic := range topics {
		score, reasons := scoreTopic(topic, stateStore, sourceCountMap, cfg, now)
		action := mapAction(score)
		results = append(results, OpportunityScore{
			Topic:   topic,
			Score:   score,
			Reasons: reasons,
			Action:  action,
		})
	}

	// 按分数降序排列
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制 ActionCreate 数量
	if cfg.MaxTopicsPerRun > 0 {
		results = limitCreateActions(results, cfg.MaxTopicsPerRun)
	}

	return results
}

// ---------- 内部函数 ----------

// scoreTopic 计算单个话题的综合评分。
func scoreTopic(
	topic TrendingTopic,
	stateStore MediaStateStore,
	sourceCountMap map[string]int,
	cfg OpportunityConfig,
	now time.Time,
) (float64, []string) {
	var total float64
	var reasons []string

	// 1. 热度分 (0-40): HeatScore 归一化，>50000 满分
	heatScore := normalizeHeat(topic.HeatScore, cfg.HeatThreshold)
	total += heatScore
	if heatScore > 0 {
		reasons = append(reasons, "heat:"+formatFloat(heatScore))
	}

	// 2. 新鲜度分: 未处理 +30，已处理 0
	if stateStore != nil && !stateStore.IsTopicProcessed(topic.Title) {
		total += 30
		reasons = append(reasons, "fresh:+30")
	} else if stateStore != nil {
		reasons = append(reasons, "already_processed:+0")
	} else {
		// stateStore 为 nil 时默认给新鲜度分
		total += 30
		reasons = append(reasons, "fresh:+30(no_store)")
	}

	// 3. 多源确认分: 同一话题在多个数据源出现 +20
	normalizedTitle := normalizeTitle(topic.Title)
	if count, ok := sourceCountMap[normalizedTitle]; ok && count > 1 {
		total += 20
		reasons = append(reasons, "multi_source:+20")
	}

	// 4. 时效分: FetchedAt 在 2h 内 +10，2-6h 线性衰减，>6h 为 0
	freshnessScore := calcFreshness(topic.FetchedAt, now)
	total += freshnessScore
	if freshnessScore > 0 {
		reasons = append(reasons, "timeliness:+"+formatFloat(freshnessScore))
	}

	return total, reasons
}

// normalizeHeat 将热度分归一化到 0-40。
// 低于 threshold 为 0，线性增长到 50000 为满分 40。
func normalizeHeat(heat, threshold float64) float64 {
	if heat < threshold {
		return 0
	}
	const maxHeat = 50000
	const maxScore = 40.0
	if heat >= maxHeat || threshold >= maxHeat {
		return maxScore
	}
	return (heat - threshold) / (maxHeat - threshold) * maxScore
}

// calcFreshness 计算时效分 (0-10)。2h 内满分，2-6h 线性衰减，>6h 为 0。
func calcFreshness(fetchedAt time.Time, now time.Time) float64 {
	if fetchedAt.IsZero() {
		return 0
	}
	hours := now.Sub(fetchedAt).Hours()
	if hours <= 2 {
		return 10
	}
	if hours >= 6 {
		return 0
	}
	// 2-6h 线性衰减
	return 10 * (6 - hours) / 4
}

// mapAction 根据分数映射建议动作。
func mapAction(score float64) OpportunityAction {
	if score >= 60 {
		return ActionCreate
	}
	if score >= 30 {
		return ActionWatch
	}
	return ActionSkip
}

// limitCreateActions 限制 ActionCreate 的数量，多余的降级为 ActionWatch。
func limitCreateActions(results []OpportunityScore, max int) []OpportunityScore {
	createCount := 0
	for i := range results {
		if results[i].Action == ActionCreate {
			createCount++
			if createCount > max {
				results[i].Action = ActionWatch
				results[i].Reasons = append(results[i].Reasons, "capped:max_per_run")
			}
		}
	}
	return results
}

// buildSourceCountMap 构建标题模糊匹配的多源计数表。
func buildSourceCountMap(topics []TrendingTopic) map[string]int {
	counts := make(map[string]int)
	for _, t := range topics {
		key := normalizeTitle(t.Title)
		counts[key]++
	}
	return counts
}

// normalizeTitle 归一化标题用于模糊匹配。
func normalizeTitle(title string) string {
	s := strings.TrimSpace(title)
	s = strings.ToLower(s)
	// 去除常见标点符号
	for _, r := range []string{"#", "【", "】", "「", "」", "《", "》", " ", "\t"} {
		s = strings.ReplaceAll(s, r, "")
	}
	return s
}

// formatFloat 格式化浮点数为字符串（保留 1 位小数）。
func formatFloat(f float64) string {
	// 简单实现，避免引入 fmt.Sprintf 的开销
	intPart := int(f)
	decPart := int((f - float64(intPart)) * 10)
	if decPart < 0 {
		decPart = -decPart
	}
	return strings.Join([]string{itoa(intPart), ".", itoa(decPart)}, "")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
