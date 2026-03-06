package main

// cmd_logs.go — 日志查看 CLI
// 对应 TS: src/commands/logs-cli.ts
//
// 子命令:
//   openacosmi logs               — 读取最近日志（默认 tail 100 行）
//   openacosmi logs --follow      — 流式追踪日志（tail -f 模式）
//   openacosmi logs --level=warn  — 按日志级别过滤

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Acosmi/ClawAcosmi/internal/cli"
	"github.com/Acosmi/ClawAcosmi/internal/config"
)

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View gateway logs",
		Long: `View and stream Claw Acosmi gateway logs.

Examples:
  openacosmi logs                    # Show last 100 lines
  openacosmi logs --tail 50          # Show last 50 lines
  openacosmi logs -f                 # Stream logs (follow mode)
  openacosmi logs --level warn       # Show only warn+ level logs
  openacosmi logs --level error -f   # Stream errors only`,
		RunE: func(cmd *cobra.Command, args []string) error {
			follow, _ := cmd.Flags().GetBool("follow")
			tail, _ := cmd.Flags().GetInt("tail")
			level, _ := cmd.Flags().GetString("level")
			jsonFlag, _ := cmd.Flags().GetBool("json")
			source, _ := cmd.Flags().GetString("source")

			logPath := resolveLogFilePath(source)

			if follow {
				return followLogs(cmd, logPath, level, jsonFlag)
			}
			return tailLogs(cmd, logPath, tail, level, jsonFlag)
		},
	}
	cmd.Flags().BoolP("follow", "f", false, "Follow log output (stream mode)")
	cmd.Flags().Int("tail", 100, "Number of lines to show from end")
	cmd.Flags().String("level", "", "Filter by log level (debug, info, warn, error)")
	cmd.Flags().String("source", "", "Log file path override (default: auto-detect)")
	return cmd
}

// resolveLogFilePath 解析日志文件路径。
// 对应 TS: logs-cli.ts 日志路径解析
func resolveLogFilePath(override string) string {
	if override != "" {
		return override
	}
	// 尝试从配置获取
	stateDir := config.ResolveStateDir()
	return filepath.Join(stateDir, "logs", "gateway.log")
}

// tailLogs 读取日志文件最后 N 行。
func tailLogs(cmd *cobra.Command, logPath string, numLines int, level string, jsonFlag bool) error {
	// 先尝试 gateway RPC（如果 gateway 在线可获取实时日志）
	if level == "" && !jsonFlag {
		result, err := cli.CallGatewayFromCLI("logs.tail", cli.GatewayRPCOpts{
			TimeoutMs: 5000,
		}, map[string]interface{}{
			"tail": numLines,
		})
		if err == nil {
			if m, ok := result.(map[string]interface{}); ok {
				if lines, ok := m["lines"].([]interface{}); ok {
					for _, line := range lines {
						if s, ok := line.(string); ok {
							cmd.Println(s)
						}
					}
					return nil
				}
			}
		}
	}

	// Fallback: 直接读取日志文件
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Printf("📋 日志文件不存在: %s\n", logPath)
			cmd.Println("   Gateway 可能尚未启动，或日志路径配置不同。")
			return nil
		}
		return fmt.Errorf("无法打开日志文件: %w", err)
	}
	defer f.Close()

	// 读取所有行
	lines, err := readAllLines(f)
	if err != nil {
		return fmt.Errorf("读取日志失败: %w", err)
	}

	// 取最后 N 行
	if len(lines) > numLines {
		lines = lines[len(lines)-numLines:]
	}

	// 按级别过滤
	minLevel := parseLogLevel(level)
	for _, line := range lines {
		if minLevel > 0 && !matchesMinLevel(line, minLevel) {
			continue
		}
		if jsonFlag {
			// 尝试格式化 JSON 日志行
			cmd.Println(line)
		} else {
			cmd.Println(formatLogLine(line))
		}
	}

	return nil
}

// followLogs 流式追踪日志。
// 对齐 TS logs-cli.ts: 优先 RPC 轮询 Gateway，不可达时 fallback 到本地文件。
func followLogs(cmd *cobra.Command, logPath string, level string, jsonFlag bool) error {
	minLevel := parseLogLevel(level)

	// 先尝试通过 Gateway RPC 轮询（远程模式）
	// 对齐 TS: logs-cli.ts 通过 RPC logs.tail 实现 --follow
	if err := followLogsViaRPC(cmd, minLevel, jsonFlag); err == nil {
		return nil // RPC 模式正常退出（不会到这里，因为 follow 是无限循环）
	}
	// RPC 不可用，fallback 到本地文件
	cmd.PrintErrln("ℹ️  Gateway RPC 不可达，回退到本地文件追踪模式")

	return followLogsLocal(cmd, logPath, minLevel, jsonFlag)
}

// followLogsViaRPC 通过 Gateway RPC 轮询日志（远程/本地 Gateway 均适用）。
// TS 对照: logs-cli.ts follow 模式通过 logs.tail RPC 轮询实现。
func followLogsViaRPC(cmd *cobra.Command, minLevel int, jsonFlag bool) error {
	// 首次探测 Gateway 是否可达
	_, err := cli.CallGatewayFromCLI("logs.tail", cli.GatewayRPCOpts{
		TimeoutMs: 3000,
	}, map[string]interface{}{
		"tail": 10,
	})
	if err != nil {
		return err // Gateway 不可达
	}

	cmd.Printf("📋 通过 Gateway RPC 追踪日志 (Ctrl+C 退出)\n\n")

	// 用行数偏移量做简单的游标追踪
	var lastLineCount int
	pollInterval := 1 * time.Second

	for {
		result, err := cli.CallGatewayFromCLI("logs.tail", cli.GatewayRPCOpts{
			TimeoutMs: 5000,
		}, map[string]interface{}{
			"tail": 200, // 获取最后 200 行用于增量比对
		})
		if err != nil {
			// RPC 中断，等待重试
			time.Sleep(pollInterval)
			continue
		}

		if m, ok := result.(map[string]interface{}); ok {
			if lines, ok := m["lines"].([]interface{}); ok {
				currentCount := len(lines)
				// 仅输出增量部分
				startIdx := 0
				if lastLineCount > 0 && lastLineCount <= currentCount {
					startIdx = lastLineCount
				} else if lastLineCount > currentCount {
					// 日志被截断或轮转，从头输出
					startIdx = 0
				}
				for i := startIdx; i < currentCount; i++ {
					if s, ok := lines[i].(string); ok {
						if minLevel > 0 && !matchesMinLevel(s, minLevel) {
							continue
						}
						if jsonFlag {
							cmd.Println(s)
						} else {
							cmd.Println(formatLogLine(s))
						}
					}
				}
				lastLineCount = currentCount
			}
		}

		time.Sleep(pollInterval)
	}
}

// followLogsLocal 本地文件追踪日志（原始 fallback 模式）。
func followLogsLocal(cmd *cobra.Command, logPath string, minLevel int, jsonFlag bool) error {
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Printf("📋 等待日志文件创建: %s\n", logPath)
			// 等待文件出现
			for {
				time.Sleep(500 * time.Millisecond)
				f, err = os.Open(logPath)
				if err == nil {
					break
				}
			}
		} else {
			return fmt.Errorf("无法打开日志文件: %w", err)
		}
	}
	defer f.Close()

	// 跳到文件末尾
	_, _ = f.Seek(0, io.SeekEnd)

	cmd.Printf("📋 跟踪日志: %s (Ctrl+C 退出)\n", logPath)
	cmd.Println()

	reader := bufio.NewReader(f)
	var partial string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line != "" {
					partial += line
				}
				time.Sleep(200 * time.Millisecond)
				continue
			}
			return err
		}

		fullLine := partial + strings.TrimRight(line, "\n\r")
		partial = ""

		if fullLine == "" {
			continue
		}

		if minLevel > 0 && !matchesMinLevel(fullLine, minLevel) {
			continue
		}

		if jsonFlag {
			cmd.Println(fullLine)
		} else {
			cmd.Println(formatLogLine(fullLine))
		}
	}
}

// ---------- 辅助函数 ----------

const (
	logLevelDebug = 1
	logLevelInfo  = 2
	logLevelWarn  = 3
	logLevelError = 4
)

func parseLogLevel(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return logLevelDebug
	case "info":
		return logLevelInfo
	case "warn", "warning":
		return logLevelWarn
	case "error", "err":
		return logLevelError
	default:
		return 0 // 不过滤
	}
}

func extractLogLevel(line string) int {
	upper := strings.ToUpper(line)
	// slog 格式: level=INFO 或 "level":"INFO"
	switch {
	case strings.Contains(upper, "LEVEL=ERROR") || strings.Contains(upper, `"LEVEL":"ERROR"`):
		return logLevelError
	case strings.Contains(upper, "LEVEL=WARN") || strings.Contains(upper, `"LEVEL":"WARN"`):
		return logLevelWarn
	case strings.Contains(upper, "LEVEL=INFO") || strings.Contains(upper, `"LEVEL":"INFO"`):
		return logLevelInfo
	case strings.Contains(upper, "LEVEL=DEBUG") || strings.Contains(upper, `"LEVEL":"DEBUG"`):
		return logLevelDebug
	// 常见日志格式
	case strings.Contains(upper, " ERROR ") || strings.Contains(upper, "[ERROR]"):
		return logLevelError
	case strings.Contains(upper, " WARN ") || strings.Contains(upper, "[WARN]"):
		return logLevelWarn
	case strings.Contains(upper, " INFO ") || strings.Contains(upper, "[INFO]"):
		return logLevelInfo
	case strings.Contains(upper, " DEBUG ") || strings.Contains(upper, "[DEBUG]"):
		return logLevelDebug
	default:
		return logLevelInfo // 默认 info
	}
}

func matchesMinLevel(line string, minLevel int) bool {
	return extractLogLevel(line) >= minLevel
}

func formatLogLine(line string) string {
	// 尝试解析 JSON 日志行并美化
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(line), &m); err == nil {
		ts, _ := m["time"].(string)
		level, _ := m["level"].(string)
		msg, _ := m["msg"].(string)
		if msg == "" {
			msg, _ = m["message"].(string)
		}

		levelIcon := "  "
		switch strings.ToUpper(level) {
		case "ERROR":
			levelIcon = "❌"
		case "WARN", "WARNING":
			levelIcon = "⚠️ "
		case "INFO":
			levelIcon = "ℹ️ "
		case "DEBUG":
			levelIcon = "🔍"
		}

		// 格式化时间
		if ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				ts = t.Format("15:04:05")
			} else if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				ts = t.Format("15:04:05.000")
			}
		}

		if ts != "" {
			return fmt.Sprintf("%s %s %s %s", ts, levelIcon, strings.ToUpper(level), msg)
		}
		return fmt.Sprintf("%s %s %s", levelIcon, strings.ToUpper(level), msg)
	}

	// 非 JSON，原样返回
	return line
}

func readAllLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	// 增大缓冲区以处理长日志行
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
