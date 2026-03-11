package i18n

// i18n_onboarding_zh.go — Onboarding 中文语言包
// 对应模块：gateway/wizard + cmd/setup + channels/onboarding
//
// Key 命名规范: onboard.{module}.{action}

func init() {
	RegisterBundle(LangZhCN, map[string]string{
		// ── gateway/wizard_finalize.go ──
		"onboard.title":                    "设置向导",
		"onboard.daemon.title":             "Gateway 服务",
		"onboard.daemon.confirm":           "安装 Gateway 服务（推荐）",
		"onboard.daemon.quickstart":        "Gateway 服务: QuickStart 跳过 daemon 安装。\n手动启动: crabclaw gateway serve",
		"onboard.daemon.systemd_unavail":   "Systemd 用户服务不可用；跳过服务安装。\n使用容器管理器或 `docker compose up -d`。",
		"onboard.daemon.already_installed": "Gateway 服务已安装",
		"onboard.daemon.opt_restart":       "重启",
		"onboard.daemon.opt_reinstall":     "重新安装",
		"onboard.daemon.opt_skip":          "跳过",
		"onboard.daemon.restarted":         "Gateway 服务已重启。",
		"onboard.daemon.installed":         "Gateway 服务已安装。",
		"onboard.daemon.install_failed":    "Gateway 服务安装失败: %s",
		"onboard.daemon.hint_unix":         "提示: 修复错误后重新运行 `crabclaw gateway install`。",
		"onboard.daemon.hint_windows":      "提示: 使用管理员 PowerShell 重新运行或跳过服务安装。",

		"onboard.completion.title":  "Shell 补全",
		"onboard.completion.prompt": "为 crabclaw 启用 %s shell 补全？（兼容别名 `openacosmi` 仍可用）",
		"onboard.completion.hint":   "为 crabclaw 启用 shell 补全：运行 `crabclaw completion %s >> %s`\n然后重启 shell 或运行: source %s\n兼容别名 `openacosmi` 也可使用。",

		"onboard.hatch.title":         "启动方式",
		"onboard.hatch.prompt":        "您想如何启动机器人？",
		"onboard.hatch.opt_tui":       "在当前终端启动（TUI 模式）",
		"onboard.hatch.opt_web":       "打开 Web 控制台",
		"onboard.hatch.opt_later":     "稍后手动启动",
		"onboard.hatch.web_opened":    "已在浏览器中打开 Web 控制台。",
		"onboard.hatch.web_failed":    "浏览器打开失败: %s\n手动打开: %s",
		"onboard.hatch.later_hint":    "稍后启动: crabclaw",
		"onboard.controlui.title":     "控制台",
		"onboard.finalize.outro":      "设置完成。使用上方的控制台链接管理 Crab Claw（蟹爪）。",
		"onboard.finalize.probe_ok":   "Gateway 可达: %s",
		"onboard.finalize.probe_fail": "Gateway 未响应: %s\n手动启动: crabclaw gateway serve",
		"onboard.finalize.web_search": "提示: 启用 Web 搜索可增强 AI 回答质量。\n配置: config.json → agents.defaults.webSearch",
		"onboard.finalize.run_manual": "运行: crabclaw",

		// ── gateway/wizard_gateway_config.go ──
		"onboard.gw.bind_title":          "Gateway 绑定",
		"onboard.gw.auth_title":          "Gateway 认证",
		"onboard.gw.ts_title":            "Tailscale 暴露",
		"onboard.gw.ts_funnel_hint":      "Tailscale Funnel 将通过外网暴露您的 Gateway。\n确保已启用: tailscale up --operator=$USER",
		"onboard.gw.ts_serve_hint":       "Tailscale Serve 仅在 tailnet 内部暴露。\n确保已启用: tailscale up --operator=$USER",
		"onboard.gw.ts_reset_confirm":    "退出时重置 Tailscale serve/funnel？",
		"onboard.gw.ts_bind_note":        "Tailscale 需要 bind=loopback。已自动调整绑定为 loopback。",
		"onboard.gw.ts_funnel_auth_note": "Tailscale funnel 需要密码认证。",

		// ── gateway/wizard_onboarding.go ──
		"onboard.welcome":               "欢迎使用 Crab Claw（蟹爪）设置向导 🚀",
		"onboard.provider.title":        "模型提供商",
		"onboard.provider.select":       "选择 AI 模型提供商",
		"onboard.provider.env_detected": "检测到环境变量 %s。使用已有凭证？",
		"onboard.model.title":           "模型选择",
		"onboard.model.select":          "选择默认模型",
		"onboard.model.confirm":         "确认使用此模型？",
		"onboard.apikey.prompt":         "输入 API 密钥",

		// ── cmd/setup_channels.go ──
		"onboard.ch.title":           "频道管理",
		"onboard.ch.all_set":         "所有频道已配置完成。",
		"onboard.ch.none":            "未选择任何频道。",
		"onboard.ch.status_title":    "频道状态",
		"onboard.ch.action":          "选择操作",
		"onboard.ch.disable_confirm": "确认禁用 %s？",
		"onboard.ch.howto_title":     "频道工作原理",
		"onboard.ch.dm_policy":       "频道默认 DM 策略",
		"onboard.ch.dm_input":        "输入允许列表（逗号分隔）",

		// ── cmd/setup_skills.go ──
		"onboard.skill.title":           "技能管理",
		"onboard.skill.intro":           "技能（Skills）为机器人添加扩展能力。\nCrab Claw（蟹爪）自带多个内置技能，您也可以安装社区技能。",
		"onboard.skill.configure":       "现在配置技能？（推荐）",
		"onboard.skill.node_missing":    "Node.js 未检测到。安装一些技能需要 Node.js。\n推荐: 通过 Homebrew 安装 Node.js",
		"onboard.skill.brew_confirm":    "显示 Homebrew 安装命令？",
		"onboard.skill.brew_hint":       "安装 Node.js:\nbrew install node\n\n安装后重新运行设置向导。",
		"onboard.skill.win_pkg_missing": "未检测到包管理器（winget/Scoop）。\n安装部分技能依赖的工具时，使用包管理器会更加方便。",
		"onboard.skill.win_pkg_confirm": "显示包管理器安装命令？",
		"onboard.skill.win_pkg_hint":    "方式一 — winget（Windows 10/11 自带）：\nwinget install GoLang.Go\nwinget install OpenJS.NodeJS.LTS\n\n方式二 — Scoop（无需管理员权限）：\npowershell -c \"irm get.scoop.sh | iex\"\nscoop install nodejs\n\n安装后重新运行设置向导。",
		"onboard.skill.node_manager":    "技能安装使用的包管理器",
		"onboard.skill.api_key_q":       "此技能需要 API 密钥。现在输入？",
		"onboard.skill.api_key_input":   "输入 %s API 密钥",
		"onboard.skill.summary":         "技能配置完成。",

		// ── cmd/setup_hooks.go ──
		"onboard.hook.title":   "Hooks 管理",
		"onboard.hook.intro":   "Hooks 可在特定事件触发时执行自定义脚本。\nCrab Claw（蟹爪）支持 pre-reply、post-reply 等生命周期钩子。",
		"onboard.hook.summary": "Hooks 配置完成。",
		"onboard.hook.none":    "当前目录下未检测到 hooks。\n创建 hooks/ 目录并添加脚本以启用。",

		// ── cmd/setup_remote.go ──
		"onboard.remote.title":          "远程连接",
		"onboard.remote.discover":       "在局域网发现 Gateway（Bonjour）？",
		"onboard.remote.discover_hint":  "正在搜索局域网上的 Crab Claw Gateway...\n这需要 Gateway 已在另一台机器上运行。",
		"onboard.remote.discover_error": "发现错误: %v",
		"onboard.remote.discover_none":  "未找到 Gateway",
		"onboard.remote.discover_found": "找到 %d 个 Gateway",
		"onboard.remote.select_gw":      "选择 Gateway",
		"onboard.remote.conn_method":    "连接方式",
		"onboard.remote.ssh_hint":       "SSH 隧道模式:\nssh -L 8080:localhost:{port} user@{host}\n然后连接 http://localhost:8080",
		"onboard.remote.url_input":      "Gateway URL",
		"onboard.remote.auth_method":    "Gateway 认证",
		"onboard.remote.token_input":    "Gateway Token",

		// ── cmd/setup_auth_options.go ──
		"onboard.auth.provider_select": "模型/认证提供商",
		"onboard.auth.no_methods":      "该提供商没有可用的认证方式。",
		"onboard.auth.method_select":   "%s 认证方式",

		// ── cmd/setup_auth_credentials.go ──
		"onboard.auth.cred_select": "选择凭证存储方式",

		// ── channels/onboarding_discord.go ──
		"onboard.ch.discord.title":     "Discord",
		"onboard.ch.discord.intro":     "Discord 机器人设置步骤：\n1. 访问 https://discord.com/developers/applications\n2. 创建新应用 → Bot → 复制 Token\n3. 启用 Message Content Intent\n4. 使用 OAuth2 URL Generator 邀请到服务器",
		"onboard.ch.discord.env_found": "DISCORD_BOT_TOKEN 已检测到。使用环境变量？",
		"onboard.ch.discord.token":     "输入 Discord 机器人 Token",
		"onboard.ch.discord.keep":      "Discord Token 已配置。保留？",

		// ── channels/onboarding_slack.go ──
		"onboard.ch.slack.title":     "Slack",
		"onboard.ch.slack.intro":     "Slack 应用设置步骤：\n1. 访问 https://api.slack.com/apps\n2. 使用下方 Manifest 创建应用\n3. 安装到工作区 → 复制 Bot Token 和 App Token",
		"onboard.ch.slack.manifest":  "Slack App Manifest（参考）",
		"onboard.ch.slack.bot_env":   "SLACK_BOT_TOKEN 已检测到。使用环境变量？",
		"onboard.ch.slack.bot_token": "Slack Bot Token (xoxb-...)",
		"onboard.ch.slack.bot_keep":  "Slack Bot Token 已配置。保留？",
		"onboard.ch.slack.app_env":   "SLACK_APP_TOKEN 已检测到。使用环境变量？",
		"onboard.ch.slack.app_token": "Slack App Token (xapp-...)",
		"onboard.ch.slack.app_keep":  "Slack App Token 已配置。保留？",

		// ── channels/onboarding_telegram.go ──
		"onboard.ch.telegram.title":     "Telegram",
		"onboard.ch.telegram.intro":     "Telegram 机器人设置步骤：\n1. 在 Telegram 中搜索 @BotFather\n2. 发送 /newbot 并按提示操作\n3. 复制 Bot Token",
		"onboard.ch.telegram.guide":     "BotFather 操作指南",
		"onboard.ch.telegram.env_found": "TELEGRAM_BOT_TOKEN 已检测到。使用环境变量？",
		"onboard.ch.telegram.token":     "Telegram Bot Token",
		"onboard.ch.telegram.keep":      "Telegram Token 已配置。保留？",
		"onboard.ch.telegram.handle":    "输入 Telegram Handle",

		// ── channels/onboarding_whatsapp.go ──
		"onboard.ch.whatsapp.title":      "WhatsApp",
		"onboard.ch.whatsapp.link_hint":  "WhatsApp 需要通过 QR 码链接。\n完成设置后运行: crabclaw gateway start\n然后用手机扫描 QR 码。\n文档: https://docs.openacosmi.dev/whatsapp",
		"onboard.ch.whatsapp.link_fail":  "WhatsApp 链接失败: %s\n稍后链接: crabclaw channels login --channel whatsapp",
		"onboard.ch.whatsapp.linked":     "WhatsApp 已链接",
		"onboard.ch.whatsapp.link_later": "%s\n稍后链接: crabclaw channels login --channel whatsapp --verbose",
		"onboard.ch.whatsapp.selfchat":   "启用自聊模式？（使用您自己的号码）",
		"onboard.ch.whatsapp.phone":      "输入 WhatsApp 手机号",
		"onboard.ch.whatsapp.phone_hint": "手机号格式说明",

		// ── channels/onboarding_signal.go ──
		"onboard.ch.signal.title":  "Signal",
		"onboard.ch.signal.setup":  "Signal 需要注册一个专用号码。",
		"onboard.ch.signal.keep":   "Signal 账号已设置 (%s)。保留？",
		"onboard.ch.signal.number": "Signal 机器人号码 (E.164 格式)",
		"onboard.ch.signal.handle": "配置 Signal Handle",
		"onboard.ch.signal.hint":   "Signal Handle 设置说明",

		// ── channels/onboarding_imessage.go ──
		"onboard.ch.imessage.title":    "iMessage",
		"onboard.ch.imessage.cli_path": "imsg CLI 路径",
		"onboard.ch.imessage.cli_req":  "启用 iMessage 需要 imsg CLI 路径。",
		"onboard.ch.imessage.setup":    "iMessage 设置说明",
		"onboard.ch.imessage.handle":   "配置 iMessage Handle",
		"onboard.ch.imessage.hint":     "iMessage Handle 设置说明",

		// ── channels/onboarding_channel_access.go ──
		"onboard.ch.access.title":   "%s 访问控制",
		"onboard.ch.access.input":   "输入允许的标识符（逗号分隔）",
		"onboard.ch.access.confirm": "为 %s 配置自定义访问规则？",
	})
}
