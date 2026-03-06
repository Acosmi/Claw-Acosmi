package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

const remoteProgressMinInterval = 8 * time.Second

type progressDeliveryTarget struct {
	Channel   string
	To        string
	AccountID string
	ThreadID  string
}

func progressDeliveryTargetFromMsgContext(msgCtx *autoreply.MsgContext) progressDeliveryTarget {
	if msgCtx == nil {
		return progressDeliveryTarget{}
	}
	return progressDeliveryTarget{
		Channel:   msgCtx.OriginatingChannel,
		To:        msgCtx.OriginatingTo,
		AccountID: msgCtx.AccountID,
		ThreadID:  msgCtx.MessageThreadID,
	}
}

func buildMsgContextProgressCallback(state *GatewayState, msgCtx *autoreply.MsgContext) func(context.Context, runner.ProgressUpdate) runner.ProgressReportStatus {
	if state == nil {
		return nil
	}
	return buildRemoteProgressCallback(state.ChannelMgr(), progressDeliveryTargetFromMsgContext(msgCtx))
}

// buildRemoteProgressCallback 创建单次 run 级别的远程进度投递回调。
// 仅在调用方显式接线时启用；闭包内部做粗粒度节流和去重，避免渠道刷屏。
func buildRemoteProgressCallback(channelMgr *channels.Manager, target progressDeliveryTarget) func(context.Context, runner.ProgressUpdate) runner.ProgressReportStatus {
	if channelMgr == nil {
		return nil
	}
	target.Channel = strings.ToLower(strings.TrimSpace(target.Channel))
	target.To = strings.TrimSpace(target.To)
	target.AccountID = strings.TrimSpace(target.AccountID)
	target.ThreadID = strings.TrimSpace(target.ThreadID)
	if target.Channel == "" || target.To == "" {
		return nil
	}

	var mu sync.Mutex
	var lastSentAt time.Time
	var lastFingerprint string

	return func(ctx context.Context, update runner.ProgressUpdate) runner.ProgressReportStatus {
		text := formatRemoteProgressText(update)
		if text == "" {
			return runner.ProgressReportStatus{}
		}
		now := time.Now()
		fingerprint := text

		mu.Lock()
		defer mu.Unlock()

		if fingerprint == lastFingerprint {
			return runner.ProgressReportStatus{Throttled: true}
		}
		if !lastSentAt.IsZero() && now.Sub(lastSentAt) < remoteProgressMinInterval {
			return runner.ProgressReportStatus{Throttled: true}
		}

		sendCtx := ctx
		if sendCtx == nil {
			sendCtx = context.Background()
		}
		_, err := channelMgr.SendMessage(channels.ChannelID(target.Channel), channels.OutboundSendParams{
			Ctx:       sendCtx,
			To:        target.To,
			Text:      text,
			AccountID: target.AccountID,
			ThreadID:  target.ThreadID,
		})
		if err != nil {
			return runner.ProgressReportStatus{Error: err.Error()}
		}

		lastFingerprint = fingerprint
		lastSentAt = now
		return runner.ProgressReportStatus{RemoteDelivered: true}
	}
}

func formatRemoteProgressText(update runner.ProgressUpdate) string {
	summary := strings.TrimSpace(update.Summary)
	if summary == "" {
		return ""
	}

	suffix := make([]string, 0, 2)
	if phase := strings.TrimSpace(update.Phase); phase != "" {
		suffix = append(suffix, phase)
	}
	if update.Percent > 0 && update.Percent <= 100 {
		suffix = append(suffix, fmt.Sprintf("%d%%", update.Percent))
	}
	if len(suffix) == 0 {
		return "[Progress] " + summary
	}
	return fmt.Sprintf("[Progress] %s (%s)", summary, strings.Join(suffix, " | "))
}
