package infra

// state_migrations_statedir.go — 状态目录自动迁移
// 对应 TS: state-migrations.ts autoMigrateLegacyStateDir (L425-570)

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

var (
	autoMigrateStateDirOnce sync.Once
	autoMigrateStateOnce    sync.Once
)

// ResetAutoMigrateForTest 重置迁移状态（测试用）。
func ResetAutoMigrateForTest() {
	autoMigrateStateDirOnce = sync.Once{}
	autoMigrateStateOnce = sync.Once{}
}

// AutoMigrateLegacyStateDir 自动迁移旧版状态目录。
func AutoMigrateLegacyStateDir(envStateDirOverride string, homeDir string) StateDirMigrationResult {
	var result StateDirMigrationResult
	autoMigrateStateDirOnce.Do(func() {
		result = doAutoMigrateStateDir(envStateDirOverride, homeDir)
	})
	return result
}

func doAutoMigrateStateDir(envOverride, homeDir string) StateDirMigrationResult {
	if envOverride != "" {
		return StateDirMigrationResult{Skipped: true}
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return StateDirMigrationResult{}
		}
	}

	targetDir := filepath.Join(homeDir, config.NewStateDirname)
	legacyDirs := []string{
		filepath.Join(homeDir, ".clawdbot"),
		filepath.Join(homeDir, ".pi-coding"),
	}

	// 找到存在的旧版目录
	var legacyDir string
	for _, d := range legacyDirs {
		if info, err := os.Lstat(d); err == nil && (info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			legacyDir = d
			break
		}
	}
	if legacyDir == "" {
		return StateDirMigrationResult{}
	}

	// 检查符号链接
	info, err := os.Lstat(legacyDir)
	if err != nil {
		return StateDirMigrationResult{}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target := resolveSymlinkTarget(legacyDir)
		if target == "" || filepath.Clean(target) == filepath.Clean(targetDir) {
			return StateDirMigrationResult{}
		}
		return StateDirMigrationResult{
			Warnings: []string{fmt.Sprintf("Legacy state dir is a symlink (%s → %s); skipping", legacyDir, target)},
		}
	}

	// 目标已存在
	if existsDir(targetDir) {
		return StateDirMigrationResult{
			Warnings: []string{fmt.Sprintf("State dir migration skipped: target already exists (%s)", targetDir)},
		}
	}

	// 执行迁移：rename + symlink
	if err := os.Rename(legacyDir, targetDir); err != nil {
		return StateDirMigrationResult{
			Warnings: []string{fmt.Sprintf("Failed to move %s → %s: %v", legacyDir, targetDir, err)},
		}
	}

	change := fmt.Sprintf("State dir: %s → %s (legacy path now symlinked)", legacyDir, targetDir)
	if err := os.Symlink(targetDir, legacyDir); err != nil {
		// Windows: try junction
		if runtime.GOOS == "windows" {
			// Go os.Symlink on Windows creates junctions for dirs
			_ = os.Symlink(targetDir, legacyDir)
		}
		if _, statErr := os.Lstat(legacyDir); statErr != nil {
			// 回滚
			if rbErr := os.Rename(targetDir, legacyDir); rbErr != nil {
				return StateDirMigrationResult{
					Changes:  []string{fmt.Sprintf("State dir: %s → %s", legacyDir, targetDir)},
					Warnings: []string{fmt.Sprintf("Symlink failed and rollback failed: %v", rbErr)},
					Migrated: true,
				}
			}
			return StateDirMigrationResult{
				Warnings: []string{"State dir migration rolled back (failed to link legacy path)"},
			}
		}
	}

	return StateDirMigrationResult{
		Migrated: true,
		Changes:  []string{change},
	}
}
