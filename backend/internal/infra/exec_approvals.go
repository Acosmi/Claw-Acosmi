package infra

// exec_approvals.go — Exec Approvals 配置文件管理
// 对应 TS: src/infra/exec-approvals.ts
//
// 管理状态目录中 exec-approvals.json 文件的读写。
// 提供 snapshot（含 SHA256 hash）用于乐观并发控制。

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- 类型定义 ----------

// ExecSecurity 执行安全级别。
type ExecSecurity string

const (
	ExecSecurityDeny      ExecSecurity = "deny"
	ExecSecurityAllowlist ExecSecurity = "allowlist"
	ExecSecuritySandboxed ExecSecurity = "sandboxed" // L2: 沙箱全权限（无网络）
	ExecSecurityFull      ExecSecurity = "full"
)

// ExecEscalationFallback 提权到期后的回落策略。
// - base: 回落到 defaults.security（现有语义）
// - sandboxed: 至少回落到 L2(sandboxed)
type ExecEscalationFallback string

const (
	ExecEscalationFallbackBase      ExecEscalationFallback = "base"
	ExecEscalationFallbackSandboxed ExecEscalationFallback = "sandboxed"
)

// ExecAsk 询问策略。
type ExecAsk string

const (
	ExecAskOff    ExecAsk = "off"
	ExecAskOnMiss ExecAsk = "on-miss"
	ExecAskAlways ExecAsk = "always"
)

// ExecHost 执行主机类型。
type ExecHost string

const (
	ExecHostSandbox ExecHost = "sandbox"
	ExecHostGateway ExecHost = "gateway"
	ExecHostNode    ExecHost = "node"
)

// CommandRuleAction 命令规则动作。
type CommandRuleAction string

const (
	RuleActionAllow CommandRuleAction = "allow"
	RuleActionAsk   CommandRuleAction = "ask"
	RuleActionDeny  CommandRuleAction = "deny"
)

// CommandRule 命令级规则（Allow/Ask/Deny）。
// 行业对照: ABAC/PBAC 策略引擎 (Cerbos, OPA)
//
// 模式匹配支持: glob 模式（如 "rm -rf *"）、前缀匹配（如 "npm "）、子串匹配（如 "*sudo*"）。
type CommandRule struct {
	ID          string            `json:"id"`
	Pattern     string            `json:"pattern"`               // glob/前缀/子串模式
	Action      CommandRuleAction `json:"action"`                // allow/ask/deny
	Description string            `json:"description,omitempty"` // 规则描述
	IsPreset    bool              `json:"isPreset,omitempty"`    // 是否为内置预设
	Priority    int               `json:"priority,omitempty"`    // 优先级（0=最高）
	CreatedAt   *int64            `json:"createdAt,omitempty"`
}

// LevelOrder 返回安全级别的排序值（L0=0, L1=1, L2=2, L3=3）。
func LevelOrder(s ExecSecurity) int {
	return levelOrderMap[s]
}

var levelOrderMap = map[ExecSecurity]int{
	ExecSecurityDeny:      0,
	ExecSecurityAllowlist: 1,
	ExecSecuritySandboxed: 2,
	ExecSecurityFull:      3,
}

// MinSecurity 取安全级别的最低值。
func MinSecurity(a, b ExecSecurity) ExecSecurity {
	order := levelOrderMap
	if order[a] <= order[b] {
		return a
	}
	return b
}

// MaxAsk 取 Ask 级别的最高值。
func MaxAsk(a, b ExecAsk) ExecAsk {
	order := map[ExecAsk]int{
		ExecAskOff:    0,
		ExecAskOnMiss: 1,
		ExecAskAlways: 2,
	}
	if order[a] >= order[b] {
		return a
	}
	return b
}

// ExecApprovalsDefaults 默认审批配置。
type ExecApprovalsDefaults struct {
	Security           ExecSecurity           `json:"security,omitempty"`
	EscalationFallback ExecEscalationFallback `json:"escalationFallback,omitempty"`
	Ask                ExecAsk                `json:"ask,omitempty"`
	AskFallback        ExecSecurity           `json:"askFallback,omitempty"`
	AutoAllowSkills    *bool                  `json:"autoAllowSkills,omitempty"`
	Rules              []CommandRule          `json:"rules,omitempty"` // P3: Allow/Ask/Deny 命令规则
}

// ExecAllowlistEntry 白名单条目。
type ExecAllowlistEntry struct {
	ID               string `json:"id,omitempty"`
	Pattern          string `json:"pattern"`
	LastUsedAt       *int64 `json:"lastUsedAt,omitempty"`
	LastUsedCommand  string `json:"lastUsedCommand,omitempty"`
	LastResolvedPath string `json:"lastResolvedPath,omitempty"`
}

// ExecApprovalsAgent Agent 级审批配置。
type ExecApprovalsAgent struct {
	ExecApprovalsDefaults
	Allowlist []ExecAllowlistEntry `json:"allowlist,omitempty"`
}

// ExecApprovalsSocket Socket 配置。
type ExecApprovalsSocket struct {
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

// PersistedEscalationRequest 持久化的待审批提权请求（磁盘表示）。
// 用于 gateway 重启后恢复未过期的审批请求。
// 定义在 infra 包避免与 gateway 包循环依赖。
type PersistedMountRequest struct {
	HostPath  string `json:"hostPath"`
	MountMode string `json:"mountMode"` // "ro" | "rw"
}

type PersistedEscalationRequest struct {
	ID             string                  `json:"id"`
	RequestedLevel string                  `json:"requestedLevel"` // "allowlist" | "sandboxed" | "full"
	Reason         string                  `json:"reason"`
	RunID          string                  `json:"runId,omitempty"`
	SessionID      string                  `json:"sessionId,omitempty"`
	RequestedAtMs  int64                   `json:"requestedAtMs"` // Unix 毫秒
	TTLMinutes     int                     `json:"ttlMinutes"`
	MountRequests  []PersistedMountRequest `json:"mountRequests,omitempty"`
}

// ExecApprovalsFile 审批配置文件结构。
type ExecApprovalsFile struct {
	Version           int                            `json:"version"`
	Socket            *ExecApprovalsSocket           `json:"socket,omitempty"`
	Defaults          *ExecApprovalsDefaults         `json:"defaults,omitempty"`
	Agents            map[string]*ExecApprovalsAgent `json:"agents,omitempty"`
	PendingEscalation *PersistedEscalationRequest    `json:"pendingEscalation,omitempty"`
}

// ExecApprovalsSnapshot 配置快照（含 hash 用于 OCC）。
type ExecApprovalsSnapshot struct {
	Path   string             `json:"path"`
	Exists bool               `json:"exists"`
	Hash   string             `json:"hash"`
	File   *ExecApprovalsFile `json:"file"`
}

// ---------- 常量 ----------

const (
	defaultExecApprovalsFile = "exec-approvals.json"
	defaultExecApprovalsSock = "exec-approvals.sock"
)

// execApprovalsMu 保护 exec-approvals.json 的所有 read-modify-write 操作。
// 防止多个 goroutine 并发写入导致数据丢失。
var execApprovalsMu sync.Mutex

// AcquireExecApprovalsLock 获取 exec-approvals.json 的写保护锁。
// 外部包在执行 read-modify-write 操作时必须先获取此锁，
// 完成操作后调用返回的 unlock 函数释放锁。
func AcquireExecApprovalsLock() (unlock func()) {
	execApprovalsMu.Lock()
	return execApprovalsMu.Unlock
}

// ---------- 公开 API ----------

// ResolveExecApprovalsPath 解析审批配置文件路径。
func ResolveExecApprovalsPath() string {
	return filepath.Join(resolveOpenAcosmiDir(), defaultExecApprovalsFile)
}

// ResolveExecApprovalsSocketPath 解析审批 socket 路径。
func ResolveExecApprovalsSocketPath() string {
	return filepath.Join(resolveOpenAcosmiDir(), defaultExecApprovalsSock)
}

// ReadExecApprovalsSnapshot 读取审批配置快照。
func ReadExecApprovalsSnapshot() *ExecApprovalsSnapshot {
	filePath := ResolveExecApprovalsPath()

	data, err := os.ReadFile(filePath)
	if err != nil {
		// 文件不存在：返回默认空 snapshot
		file := newDefaultExecApprovalsFile()
		return &ExecApprovalsSnapshot{
			Path:   filePath,
			Exists: false,
			Hash:   hashRaw(nil),
			File:   file,
		}
	}

	var parsed ExecApprovalsFile
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Version != 1 {
		// 解析失败：使用默认值
		file := newDefaultExecApprovalsFile()
		return &ExecApprovalsSnapshot{
			Path:   filePath,
			Exists: true,
			Hash:   hashRaw(data),
			File:   file,
		}
	}

	return &ExecApprovalsSnapshot{
		Path:   filePath,
		Exists: true,
		Hash:   hashRaw(data),
		File:   &parsed,
	}
}

// SaveExecApprovals 持久化审批配置。
func SaveExecApprovals(file *ExecApprovalsFile) error {
	filePath := ResolveExecApprovalsPath()

	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir exec-approvals dir: %w", err)
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal exec-approvals: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("write exec-approvals: %w", err)
	}

	// best-effort chmod
	_ = os.Chmod(filePath, 0o600)

	return nil
}

// EnsureExecApprovals 确保审批配置文件存在（含 socket + token）。
// 使用 execApprovalsMu 保护 read-modify-write。
func EnsureExecApprovals() (*ExecApprovalsFile, error) {
	execApprovalsMu.Lock()
	defer execApprovalsMu.Unlock()
	return ensureExecApprovalsLocked()
}

// EnsureExecApprovalsLocked 是 EnsureExecApprovals 的无锁版本。
// 调用方必须已持有 execApprovalsMu（通过 AcquireExecApprovalsLock）。
func EnsureExecApprovalsLocked() (*ExecApprovalsFile, error) {
	return ensureExecApprovalsLocked()
}

func ensureExecApprovalsLocked() (*ExecApprovalsFile, error) {
	snapshot := ReadExecApprovalsSnapshot()
	file := snapshot.File

	if file.Socket == nil {
		file.Socket = &ExecApprovalsSocket{}
	}
	if file.Socket.Path == "" {
		file.Socket.Path = ResolveExecApprovalsSocketPath()
	}
	if file.Socket.Token == "" {
		file.Socket.Token = generateToken()
	}

	if err := SaveExecApprovals(file); err != nil {
		return file, err
	}
	return file, nil
}

// RedactExecApprovals 移除敏感字段（token）用于返回给前端。
func RedactExecApprovals(file *ExecApprovalsFile) *ExecApprovalsFile {
	if file == nil {
		return nil
	}
	redacted := *file
	if file.Socket != nil && file.Socket.Path != "" {
		redacted.Socket = &ExecApprovalsSocket{Path: file.Socket.Path}
	} else {
		redacted.Socket = nil
	}
	return &redacted
}

// ---------- 内部辅助 ----------

func resolveOpenAcosmiDir() string {
	return config.ResolveStateDir()
}

func hashRaw(data []byte) string {
	h := sha256.New()
	if data != nil {
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func generateToken() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func newDefaultExecApprovalsFile() *ExecApprovalsFile {
	return &ExecApprovalsFile{
		Version: 1,
		Agents:  make(map[string]*ExecApprovalsAgent),
	}
}

// ---------- 提权请求持久化辅助 ----------

// SaveEscalationPending 将待审批提权请求持久化到 exec-approvals.json。
// 采用 read-modify-write 模式，保留文件中已有的规则和配置。
// 使用 execApprovalsMu 保护，防止并发写入丢失数据。
func SaveEscalationPending(req *PersistedEscalationRequest) error {
	execApprovalsMu.Lock()
	defer execApprovalsMu.Unlock()
	snapshot := ReadExecApprovalsSnapshot()
	file := snapshot.File
	if file == nil {
		file = newDefaultExecApprovalsFile()
	}
	file.PendingEscalation = req
	return SaveExecApprovals(file)
}

// ClearEscalationPending 从 exec-approvals.json 中移除持久化的提权请求。
// 使用 execApprovalsMu 保护，防止并发写入丢失数据。
func ClearEscalationPending() error {
	execApprovalsMu.Lock()
	defer execApprovalsMu.Unlock()
	snapshot := ReadExecApprovalsSnapshot()
	file := snapshot.File
	if file == nil || file.PendingEscalation == nil {
		return nil // 无需清理
	}
	file.PendingEscalation = nil
	return SaveExecApprovals(file)
}

// PersistBaseSecurityLevel 持久化基础安全级别到 exec-approvals.json。
// 使用 execApprovalsMu 保护 read-modify-write。
func PersistBaseSecurityLevel(level ExecSecurity) error {
	execApprovalsMu.Lock()
	defer execApprovalsMu.Unlock()
	snapshot := ReadExecApprovalsSnapshot()
	file := snapshot.File
	if file == nil {
		file = &ExecApprovalsFile{Version: 1}
	}
	if file.Defaults == nil {
		file.Defaults = &ExecApprovalsDefaults{}
	}
	file.Defaults.Security = level
	return SaveExecApprovals(file)
}

// ReadEscalationPending 从 exec-approvals.json 读取持久化的提权请求。
// 文件不存在或无持久化请求时返回 nil。
func ReadEscalationPending() *PersistedEscalationRequest {
	snapshot := ReadExecApprovalsSnapshot()
	if snapshot.File == nil {
		return nil
	}
	return snapshot.File.PendingEscalation
}

// ParseExecApprovalsFileFromMap 将 map[string]any 反序列化为 *ExecApprovalsFile。
// 用于解析 gateway API 返回的 JSON 对象。
func ParseExecApprovalsFileFromMap(m map[string]any) *ExecApprovalsFile {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var f ExecApprovalsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil
	}
	if f.Agents == nil {
		f.Agents = make(map[string]*ExecApprovalsAgent)
	}
	return &f
}
