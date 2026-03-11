package gateway

// escalation_audit.go — P2 提权审计日志
// 审计日志记录每次提权/降权事件，供安全审查和合规使用。
//
// 格式: JSON Lines（每行一个 JSON 对象）
// 路径: ~/.openacosmi/escalation-audit.log

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- 审计事件类型 ----------

// AuditEventType 审计事件类型枚举。
type AuditEventType string

const (
	AuditEventRequest      AuditEventType = "request"
	AuditEventApprove      AuditEventType = "approve"
	AuditEventDeny         AuditEventType = "deny"
	AuditEventExpire       AuditEventType = "expire"
	AuditEventTaskComplete AuditEventType = "task_complete"
	AuditEventManualRevoke AuditEventType = "manual_revoke"
)

// EscalationAuditEntry 审计日志条目。
type EscalationAuditEntry struct {
	Timestamp      time.Time      `json:"timestamp"`
	Event          AuditEventType `json:"event"`
	RequestID      string         `json:"requestId"`
	RequestedLevel string         `json:"requestedLevel,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	RunID          string         `json:"runId,omitempty"`
	SessionID      string         `json:"sessionId,omitempty"`
	TTLMinutes     int            `json:"ttlMinutes,omitempty"`
}

// ---------- 审计日志写入器 ----------

const defaultAuditLogFile = "escalation-audit.log"

// EscalationAuditLogger 审计日志写入器。
type EscalationAuditLogger struct {
	mu       sync.Mutex
	filePath string
}

// NewEscalationAuditLogger 创建审计日志写入器。
func NewEscalationAuditLogger() *EscalationAuditLogger {
	return &EscalationAuditLogger{
		filePath: filepath.Join(config.ResolveStateDir(), defaultAuditLogFile),
	}
}

// FilePath 返回审计日志文件路径。
func (l *EscalationAuditLogger) FilePath() string {
	return l.filePath
}

// Log 追加一条审计日志。
func (l *EscalationAuditLogger) Log(entry EscalationAuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 确保目录存在
	dir := filepath.Dir(l.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	f, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = f.Write(data)
}

// ReadRecent 读取最近 N 条审计日志（最新在前）。
func (l *EscalationAuditLogger) ReadRecent(limit int) ([]EscalationAuditEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}

	f, err := os.Open(l.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	// 读取所有行
	var all []EscalationAuditEntry
	scanner := bufio.NewScanner(f)
	// 设置更大的 buffer 以支持长行
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry EscalationAuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // 跳过损坏行
		}
		all = append(all, entry)
	}

	if len(all) == 0 {
		return nil, nil
	}

	// 返回最近 N 条（最新在前）
	start := 0
	if len(all) > limit {
		start = len(all) - limit
	}
	recent := all[start:]

	// 反转顺序（最新在前）
	result := make([]EscalationAuditEntry, len(recent))
	for i, entry := range recent {
		result[len(recent)-1-i] = entry
	}
	return result, nil
}
