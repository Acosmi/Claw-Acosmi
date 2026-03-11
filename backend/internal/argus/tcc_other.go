//go:build !darwin

package argus

// tcc_other.go — 非 macOS 平台的 TCC 存根
//
// 仅 macOS 有 TCC 权限系统，其他平台无需检测。
// Recovery() 逻辑与 darwin 版本一致（仅使用 struct 字段，无平台特化 API）。

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PermissionState 权限状态。
type PermissionState string

const (
	PermGranted PermissionState = "granted"
	PermDenied  PermissionState = "denied"
	PermUnknown PermissionState = "unknown"
)

// TCCStatus macOS TCC 权限状态。
type TCCStatus struct {
	ScreenRecording         PermissionState `json:"screen_recording"`
	Accessibility           PermissionState `json:"accessibility"`
	ScreenRecordingDaysLeft int             `json:"screen_recording_days_left,omitempty"`
	ScreenRecordingExpiring bool            `json:"screen_recording_expiring,omitempty"`
	PermissionSubject       string          `json:"permission_subject,omitempty"`
	PermissionPath          string          `json:"permission_path,omitempty"`
	DetectionMode           string          `json:"detection_mode,omitempty"`
}

// CheckTCCPermissions 非 macOS 平台总是返回 granted。
func CheckTCCPermissions() TCCStatus {
	return CheckTCCPermissionsForBinary("")
}

// CheckTCCPermissionsForBinary 非 macOS 平台总是返回 granted。
func CheckTCCPermissionsForBinary(binaryPath string) TCCStatus {
	subject, path := detectPermissionSubject(binaryPath)
	return TCCStatus{
		ScreenRecording:         PermGranted,
		Accessibility:           PermGranted,
		ScreenRecordingDaysLeft: -1,
		PermissionSubject:       subject,
		PermissionPath:          path,
		DetectionMode:           "not_applicable",
	}
}

func detectPermissionSubject(binaryPath string) (string, string) {
	execPath, err := os.Executable()
	if err != nil {
		if strings.TrimSpace(binaryPath) != "" {
			return "helper_process", binaryPath
		}
		return "main_app", ""
	}
	if strings.TrimSpace(binaryPath) == "" || binaryPath == execPath {
		return "main_app", execPath
	}
	return "helper_process", binaryPath
}

// HasRequiredPermissions 检查是否具备所有必需权限。
func (s TCCStatus) HasRequiredPermissions() bool {
	return s.ScreenRecording == PermGranted && s.Accessibility == PermGranted
}

// Recovery 返回面向用户的恢复指引（与 darwin 版本逻辑一致）。
func (s TCCStatus) Recovery() string {
	var missing []string
	if s.ScreenRecording == PermDenied {
		missing = append(missing, "Screen Recording")
	}
	if s.Accessibility == PermDenied {
		missing = append(missing, "Accessibility")
	}
	if len(missing) == 0 {
		if s.ScreenRecordingExpiring {
			return fmt.Sprintf("Screen Recording permission expires in %d days. Re-authorize in System Settings > Privacy & Security > Screen Recording.", s.ScreenRecordingDaysLeft)
		}
		return ""
	}
	return fmt.Sprintf("Grant %s permission in System Settings > Privacy & Security > %s.",
		strings.Join(missing, " and "),
		strings.Join(missing, " / "))
}

// SequoiaExpiryDaysFromModTime 从文件修改时间计算到期天数（用于测试）。
func SequoiaExpiryDaysFromModTime(modTime time.Time) int {
	expiryDate := modTime.Add(30 * 24 * time.Hour)
	remaining := int(time.Until(expiryDate).Hours() / 24)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ParseSequoiaExpiryFromStat 从 stat -f 输出解析修改时间戳（非 macOS 存根）。
func ParseSequoiaExpiryFromStat(statOutput string) (time.Time, bool) {
	statOutput = strings.TrimSpace(statOutput)
	ts, err := strconv.ParseInt(statOutput, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(ts, 0), true
}
