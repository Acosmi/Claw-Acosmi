package log

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 对应原版 src/logging/logger.ts 中的文件日志功能：
// - 按日期滚动日志文件 (openacosmi-YYYY-MM-DD.log)
// - 24h 自动清理旧日志
// - JSON Lines 格式输出

const (
	logPrefix   = "openacosmi"
	logSuffix   = ".log"
	maxLogAge   = 24 * time.Hour
	rollingFmt  = "2006-01-02" // Go 日期格式
	rollingName = logPrefix + "-" + rollingFmt + logSuffix
)

var DefaultLogDir = filepath.Join(os.TempDir(), "openacosmi")

// FileWriter 文件日志写入器，支持按日期自动滚动。
type FileWriter struct {
	mu      sync.Mutex
	dir     string
	file    *os.File
	curDate string // 当前日志文件的日期 (YYYY-MM-DD)
}

// NewFileWriter 创建文件日志写入器。
// dir 为日志目录，为空则使用 DefaultLogDir。
func NewFileWriter(dir string) *FileWriter {
	if dir == "" {
		dir = DefaultLogDir
	}
	return &FileWriter{dir: dir}
}

// WriteEntry 写入一条 JSON Lines 格式的日志条目。
// 自动处理日期滚动和目录创建。
func (fw *FileWriter) WriteEntry(entry map[string]interface{}) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	today := time.Now().Format(rollingFmt)

	// 日期变化时滚动文件
	if fw.file == nil || fw.curDate != today {
		if err := fw.rotate(today); err != nil {
			return err
		}
	}

	entry["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal log entry: %w", err)
	}
	data = append(data, '\n')

	_, err = fw.file.Write(data)
	return err
}

// Close 关闭当前文件。
func (fw *FileWriter) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.file != nil {
		err := fw.file.Close()
		fw.file = nil
		return err
	}
	return nil
}

// CurrentPath 返回当前日志文件路径。
func (fw *FileWriter) CurrentPath() string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.file != nil {
		return fw.file.Name()
	}
	today := time.Now().Format(rollingFmt)
	return filepath.Join(fw.dir, fmt.Sprintf("%s-%s%s", logPrefix, today, logSuffix))
}

// rotate 切换到新日期的日志文件。调用者必须持有 mu。
func (fw *FileWriter) rotate(date string) error {
	// 关闭旧文件
	if fw.file != nil {
		_ = fw.file.Close()
		fw.file = nil
	}

	// 确保目录存在
	if err := os.MkdirAll(fw.dir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	// 打开新文件
	name := filepath.Join(fw.dir, fmt.Sprintf("%s-%s%s", logPrefix, date, logSuffix))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	fw.file = f
	fw.curDate = date

	// 后台清理旧日志（不阻塞写入）
	go pruneOldLogs(fw.dir)

	return nil
}

// pruneOldLogs 删除超过 maxLogAge 的滚动日志文件。
// 对应原版 pruneOldRollingLogs()。
func pruneOldLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-maxLogAge)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isRollingLogName(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// isRollingLogName 判断文件名是否为日期滚动日志格式。
// 格式: openacosmi-YYYY-MM-DD.log
func isRollingLogName(name string) bool {
	if !strings.HasPrefix(name, logPrefix+"-") {
		return false
	}
	if !strings.HasSuffix(name, logSuffix) {
		return false
	}
	// openacosmi-YYYY-MM-DD.log → 长度固定
	expected := len(logPrefix) + 1 + len("2006-01-02") + len(logSuffix)
	return len(name) == expected
}

// DefaultRollingPath 返回今天的默认日志文件路径。
func DefaultRollingPath() string {
	today := time.Now().Format(rollingFmt)
	return filepath.Join(DefaultLogDir, fmt.Sprintf("%s-%s%s", logPrefix, today, logSuffix))
}
