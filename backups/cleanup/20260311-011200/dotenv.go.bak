package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 对应 TS src/infra/dotenv.ts — .env 文件加载
// 审计修复项: CLI-P2-2 (dotenv)

// LoadDotEnv 加载 .env 文件中的环境变量。
// 先加载 CWD/.env，再加载全局 fallback (~/.openacosmi/.env)。
// 不覆盖已存在的环境变量。
// 对应 TS loadDotEnv({ quiet })。
func LoadDotEnv(quiet bool) {
	// 1. 加载当前工作目录的 .env
	loadDotEnvFile(".env", quiet)

	// 2. 加载全局 fallback: ~/.openacosmi/.env（或 CRABCLAW_STATE_DIR / OPENACOSMI_STATE_DIR 下的 .env）
	globalDir := envValueCompat("CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR")
	if globalDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		globalDir = filepath.Join(home, ".openacosmi")
	}
	globalEnvPath := filepath.Join(globalDir, ".env")
	loadDotEnvFile(globalEnvPath, quiet)
}

// loadDotEnvFile 解析单个 .env 文件并设置环境变量（不覆盖已有值）。
// 简化实现，等价于 godotenv 核心逻辑，避免引入额外依赖。
func loadDotEnvFile(path string, quiet bool) {
	f, err := os.Open(path)
	if err != nil {
		return // 文件不存在或无权限，静默跳过
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 移除可选的 export 前缀
		line = strings.TrimPrefix(line, "export ")

		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])

		// 移除引号包裹
		val = unquoteDotEnvValue(val)

		// 不覆盖已有环境变量
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, val); err != nil && !quiet {
			fmt.Fprintf(os.Stderr, "[dotenv] failed to set %s: %v\n", key, err)
		}
	}
}

// unquoteDotEnvValue 移除 .env 值两端的引号（单引号或双引号）。
func unquoteDotEnvValue(val string) string {
	if len(val) < 2 {
		return val
	}
	first, last := val[0], val[len(val)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return val[1 : len(val)-1]
	}
	return val
}
