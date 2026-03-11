package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/google/uuid"
)

// Telegram Update Offset 存储 — 继承自 src/telegram/update-offset-store.ts (83L)

const storeVersion = 1

type updateOffsetState struct {
	Version      int  `json:"version"`
	LastUpdateID *int `json:"lastUpdateId"`
}

var sanitizeAccountRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeAccountForFile(accountID string) string {
	trimmed := strings.TrimSpace(accountID)
	if trimmed == "" {
		return "default"
	}
	// 对齐 TS: /[^a-z0-9._-]+/gi 保留原始大小写
	return sanitizeAccountRe.ReplaceAllString(trimmed, "_")
}

func resolveUpdateOffsetPath(accountID string) string {
	stateDir := resolveStateDir()
	normalized := sanitizeAccountForFile(accountID)
	return filepath.Join(stateDir, "telegram", fmt.Sprintf("update-offset-%s.json", normalized))
}

// resolveStateDir 解析状态目录（继承自 config/paths.ts）
func resolveStateDir() string {
	return config.ResolveStateDir()
}

// ReadTelegramUpdateOffset 读取已保存的 update offset。
func ReadTelegramUpdateOffset(accountID string) (*int, error) {
	filePath := resolveUpdateOffsetPath(accountID)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, nil // 静默忽略读取错误
	}
	var state updateOffsetState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, nil
	}
	if state.Version != storeVersion {
		return nil, nil
	}
	return state.LastUpdateID, nil
}

// WriteTelegramUpdateOffset 原子写入 update offset。
func WriteTelegramUpdateOffset(accountID string, updateID int) error {
	filePath := resolveUpdateOffsetPath(accountID)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	state := updateOffsetState{Version: storeVersion, LastUpdateID: &updateID}
	data, _ := json.MarshalIndent(state, "", "  ")
	data = append(data, '\n')

	// 原子写入：先写临时文件再 rename
	tmp := filepath.Join(dir, fmt.Sprintf("%s.%s.tmp", filepath.Base(filePath), uuid.New().String()))
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	return os.Rename(tmp, filePath)
}
