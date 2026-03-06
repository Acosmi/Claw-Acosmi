package media

// ============================================================================
// media/media_cron.go — 媒体巡检 Cron 定时任务注册
//
// 复用现有 cron 框架（backend/internal/cron/），为媒体子智能体注册
// 三类定时巡检任务：热点监控、发布状态跟踪、互动回复。
// ============================================================================

import (
	"fmt"

	"github.com/Acosmi/ClawAcosmi/internal/cron"
)

// ---------- 配置 ----------

// MediaCronConfig 媒体巡检 cron 配置。
type MediaCronConfig struct {
	// TrendingPatrolIntervalMs 热点巡检间隔（默认 4h = 14_400_000）。
	TrendingPatrolIntervalMs int64
	// PublishCheckIntervalMs 发布状态检查间隔（默认 2h = 7_200_000）。
	PublishCheckIntervalMs int64
	// InteractPatrolIntervalMs 互动巡检间隔（默认 1h = 3_600_000）。
	InteractPatrolIntervalMs int64
	// Enabled 全局开关。
	Enabled bool
	// AutoSpawnEnabled 自动 spawn 开关（默认 false）。
	AutoSpawnEnabled bool
	// MaxAutoSpawnsPerDay 每日最大自动 spawn 次数（默认 5）。
	MaxAutoSpawnsPerDay int
	// HeatThreshold 热度阈值（默认 10000）。
	HeatThreshold float64
}

// DefaultMediaCronConfig 返回默认媒体巡检配置。
func DefaultMediaCronConfig() MediaCronConfig {
	return MediaCronConfig{
		TrendingPatrolIntervalMs: 14_400_000, // 4h
		PublishCheckIntervalMs:   7_200_000,  // 2h
		InteractPatrolIntervalMs: 3_600_000,  // 1h
		Enabled:                  true,
		AutoSpawnEnabled:         false,
		MaxAutoSpawnsPerDay:      5,
		HeatThreshold:            10000,
	}
}

// ---------- 接口 ----------

// CronServiceAdder 抽象 CronService.Add 方法，便于测试 mock。
type CronServiceAdder interface {
	Add(input cron.CronJobCreate) (*cron.CronAddResult, error)
}

// ---------- Job 定义 ----------

// mediaPatrolJob 描述一个巡检任务的元数据。
type mediaPatrolJob struct {
	Name    string
	Desc    string
	Message string
	EveryMs int64
}

// buildMediaPatrolJobs 根据配置构造巡检任务列表。
func buildMediaPatrolJobs(cfg MediaCronConfig) []mediaPatrolJob {
	trendingMsg := "扫描当前热搜，检查是否有值得创作的新话题。使用 trending 工具获取热点列表，对比已处理话题，如有新热点则启动创作流程。"
	if cfg.AutoSpawnEnabled {
		trendingMsg += fmt.Sprintf(
			"\n\n【自动创作模式已启用】当发现高分机会话题（评分≥60）时，可直接调用 spawn_media_agent 工具自动创建内容草稿，无需等待人工指令。每日上限 %d 次。",
			autoSpawnLimit(cfg.MaxAutoSpawnsPerDay),
		)
	}
	return []mediaPatrolJob{
		{
			Name:    "media.patrol.trending",
			Desc:    "热点监控巡检 — 定时扫描热搜话题",
			Message: trendingMsg,
			EveryMs: cfg.TrendingPatrolIntervalMs,
		},
		{
			Name:    "media.patrol.publish",
			Desc:    "发布状态跟踪 — 确认已提交内容是否成功发布",
			Message: "检查最近的发布状态，确认所有已提交内容是否成功发布。",
			EveryMs: cfg.PublishCheckIntervalMs,
		},
		{
			Name:    "media.patrol.interact",
			Desc:    "互动回复巡检 — 检查新评论和私信",
			Message: "检查小红书/微信新评论和私信，按互动规则回复。",
			EveryMs: cfg.InteractPatrolIntervalMs,
		},
	}
}

// ---------- 注册 ----------

// MediaCronJobRef 已注册的 cron job 引用。
type MediaCronJobRef struct {
	JobID   string // cron 服务分配的 ID
	JobName string // job 名称（如 "media.patrol.trending"）
}

// RegisterMediaCronJobs 注册媒体巡检任务到 CronService。
// 返回已注册 job 引用列表。若 cfg.Enabled=false 则不注册，返回空列表。
func RegisterMediaCronJobs(cronSvc CronServiceAdder, cfg MediaCronConfig) ([]MediaCronJobRef, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	jobs := buildMediaPatrolJobs(cfg)
	refs := make([]MediaCronJobRef, 0, len(jobs))

	for _, j := range jobs {
		enabled := true
		result, err := cronSvc.Add(cron.CronJobCreate{
			Name:        j.Name,
			Description: j.Desc,
			Enabled:     &enabled,
			Schedule: cron.CronSchedule{
				Kind:    cron.ScheduleKindEvery,
				EveryMs: j.EveryMs,
			},
			SessionTarget: cron.SessionTargetIsolated,
			WakeMode:      cron.WakeModeNow,
			Payload: cron.CronPayload{
				Kind:    cron.PayloadKindAgentTurn,
				Message: j.Message,
			},
		})
		if err != nil {
			return refs, fmt.Errorf("register media cron job %q: %w", j.Name, err)
		}
		refs = append(refs, MediaCronJobRef{JobID: result.JobID, JobName: j.Name})
	}
	return refs, nil
}

// autoSpawnLimit 返回每日自动 spawn 上限，确保最低为 1。
func autoSpawnLimit(maxPerDay int) int {
	if maxPerDay <= 0 {
		return 5 // 默认值
	}
	return maxPerDay
}
