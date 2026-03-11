package i18n

// i18n_onboarding_en.go — Onboarding English language pack
// Mirrors: i18n_onboarding_zh.go

func init() {
	RegisterBundle(LangEnUS, map[string]string{
		// ── gateway/wizard_finalize.go ──
		"onboard.title":                    "Setup Wizard",
		"onboard.daemon.title":             "Gateway Service",
		"onboard.daemon.confirm":           "Install Gateway service (recommended)",
		"onboard.daemon.quickstart":        "Gateway service: QuickStart skips daemon install.\nStart manually: crabclaw gateway serve",
		"onboard.daemon.systemd_unavail":   "Systemd user services are unavailable; skipping service install.\nUse your container supervisor or `docker compose up -d`.",
		"onboard.daemon.already_installed": "Gateway service already installed",
		"onboard.daemon.opt_restart":       "Restart",
		"onboard.daemon.opt_reinstall":     "Reinstall",
		"onboard.daemon.opt_skip":          "Skip",
		"onboard.daemon.restarted":         "Gateway service restarted.",
		"onboard.daemon.installed":         "Gateway service installed.",
		"onboard.daemon.install_failed":    "Gateway service install failed: %s",
		"onboard.daemon.hint_unix":         "Tip: rerun `crabclaw gateway install` after fixing the error.",
		"onboard.daemon.hint_windows":      "Tip: rerun from an elevated PowerShell or skip service install.",

		"onboard.completion.title":  "Shell completion",
		"onboard.completion.prompt": "Enable %s shell completion for crabclaw? (`openacosmi` remains available as a compatibility alias)",
		"onboard.completion.hint":   "Shell completion for crabclaw: run `crabclaw completion %s >> %s`\nThen restart your shell or run: source %s\nCompatibility alias `openacosmi` also works.",

		"onboard.hatch.title":         "Launch",
		"onboard.hatch.prompt":        "How do you want to hatch your bot?",
		"onboard.hatch.opt_tui":       "Hatch in current terminal (TUI)",
		"onboard.hatch.opt_web":       "Open the Web UI",
		"onboard.hatch.opt_later":     "Later (manual start)",
		"onboard.hatch.web_opened":    "Web UI opened in browser.",
		"onboard.hatch.web_failed":    "Failed to open browser: %s\nOpen manually: %s",
		"onboard.hatch.later_hint":    "Start later: crabclaw",
		"onboard.controlui.title":     "Control UI",
		"onboard.finalize.outro":      "Onboarding complete. Use the dashboard link above to control Crab Claw（蟹爪）.",
		"onboard.finalize.probe_ok":   "Gateway reachable: %s",
		"onboard.finalize.probe_fail": "Gateway not responding: %s\nStart manually: crabclaw gateway serve",
		"onboard.finalize.web_search": "Tip: enable web search to enhance AI answer quality.\nConfigure: config.json → agents.defaults.webSearch",
		"onboard.finalize.run_manual": "Run: crabclaw",

		// ── gateway/wizard_gateway_config.go ──
		"onboard.gw.bind_title":          "Gateway bind",
		"onboard.gw.auth_title":          "Gateway auth",
		"onboard.gw.ts_title":            "Tailscale exposure",
		"onboard.gw.ts_funnel_hint":      "Tailscale Funnel will expose your Gateway to the internet.\nMake sure it's enabled: tailscale up --operator=$USER",
		"onboard.gw.ts_serve_hint":       "Tailscale Serve exposes within your tailnet only.\nMake sure it's enabled: tailscale up --operator=$USER",
		"onboard.gw.ts_reset_confirm":    "Reset Tailscale serve/funnel on exit?",
		"onboard.gw.ts_bind_note":        "Tailscale requires bind=loopback. Adjusting bind to loopback.",
		"onboard.gw.ts_funnel_auth_note": "Tailscale funnel requires password auth.",

		// ── gateway/wizard_onboarding.go ──
		"onboard.welcome":               "Welcome to the Crab Claw（蟹爪） Setup Wizard 🚀",
		"onboard.provider.title":        "Model Provider",
		"onboard.provider.select":       "Choose your AI model provider",
		"onboard.provider.env_detected": "Environment variable %s detected. Use existing credentials?",
		"onboard.model.title":           "Model Selection",
		"onboard.model.select":          "Select default model",
		"onboard.model.confirm":         "Confirm this model?",
		"onboard.apikey.prompt":         "Enter API key",

		// ── cmd/setup_channels.go ──
		"onboard.ch.title":           "Channels",
		"onboard.ch.all_set":         "All channels are already configured.",
		"onboard.ch.none":            "No channels selected.",
		"onboard.ch.status_title":    "Channel Status",
		"onboard.ch.action":          "Select action",
		"onboard.ch.disable_confirm": "Confirm disable %s?",
		"onboard.ch.howto_title":     "How channels work",
		"onboard.ch.dm_policy":       "Default DM policy for channels",
		"onboard.ch.dm_input":        "Enter allowlist (comma separated)",

		// ── cmd/setup_skills.go ──
		"onboard.skill.title":           "Skills",
		"onboard.skill.intro":           "Skills add extended capabilities to your bot.\nCrab Claw（蟹爪） ships with several built-in skills, and you can install community skills.",
		"onboard.skill.configure":       "Configure skills now? (recommended)",
		"onboard.skill.node_missing":    "Node.js not detected. Some skills require Node.js.\nRecommended: install Node.js via Homebrew",
		"onboard.skill.brew_confirm":    "Show Homebrew install command?",
		"onboard.skill.brew_hint":       "Install Node.js:\nbrew install node\n\nRe-run setup after installing.",
		"onboard.skill.win_pkg_missing": "No package manager detected (winget/Scoop).\nSome skills require additional tools that are easier to install with a package manager.",
		"onboard.skill.win_pkg_confirm": "Show package manager install commands?",
		"onboard.skill.win_pkg_hint":    "Option 1 — winget (built-in on Windows 10/11):\nwinget install GoLang.Go\nwinget install OpenJS.NodeJS.LTS\n\nOption 2 — Scoop (no admin required):\npowershell -c \"irm get.scoop.sh | iex\"\nscoop install nodejs\n\nRe-run setup after installing.",
		"onboard.skill.node_manager":    "Preferred node manager for skill installs",
		"onboard.skill.api_key_q":       "This skill requires an API key. Enter now?",
		"onboard.skill.api_key_input":   "Enter %s API key",
		"onboard.skill.summary":         "Skills configuration complete.",

		// ── cmd/setup_hooks.go ──
		"onboard.hook.title":   "Hooks",
		"onboard.hook.intro":   "Hooks let you run custom scripts on specific events.\nCrab Claw（蟹爪） supports pre-reply, post-reply and other lifecycle hooks.",
		"onboard.hook.summary": "Hooks configuration complete.",
		"onboard.hook.none":    "No hooks detected in current directory.\nCreate a hooks/ directory and add scripts to enable.",

		// ── cmd/setup_remote.go ──
		"onboard.remote.title":          "Remote Connection",
		"onboard.remote.discover":       "Discover gateway on LAN (Bonjour)?",
		"onboard.remote.discover_hint":  "Scanning for Crab Claw Gateways on the local network...\nThis requires a Gateway to be running on another machine.",
		"onboard.remote.discover_error": "Discovery error: %v",
		"onboard.remote.discover_none":  "No gateways found",
		"onboard.remote.discover_found": "Found %d gateway(s)",
		"onboard.remote.select_gw":      "Select gateway",
		"onboard.remote.conn_method":    "Connection method",
		"onboard.remote.ssh_hint":       "SSH tunnel mode:\nssh -L 8080:localhost:{port} user@{host}\nThen connect to http://localhost:8080",
		"onboard.remote.url_input":      "Gateway URL",
		"onboard.remote.auth_method":    "Gateway auth",
		"onboard.remote.token_input":    "Gateway Token",

		// ── cmd/setup_auth_options.go ──
		"onboard.auth.provider_select": "Model/auth provider",
		"onboard.auth.no_methods":      "No auth methods available for that provider.",
		"onboard.auth.method_select":   "%s auth method",

		// ── cmd/setup_auth_credentials.go ──
		"onboard.auth.cred_select": "Select credential storage",

		// ── channels/onboarding_discord.go ──
		"onboard.ch.discord.title":     "Discord",
		"onboard.ch.discord.intro":     "Discord bot setup steps:\n1. Go to https://discord.com/developers/applications\n2. Create New App → Bot → Copy Token\n3. Enable Message Content Intent\n4. Use OAuth2 URL Generator to invite to server",
		"onboard.ch.discord.env_found": "DISCORD_BOT_TOKEN detected. Use env var?",
		"onboard.ch.discord.token":     "Enter Discord bot token",
		"onboard.ch.discord.keep":      "Discord token already configured. Keep it?",

		// ── channels/onboarding_slack.go ──
		"onboard.ch.slack.title":     "Slack",
		"onboard.ch.slack.intro":     "Slack app setup steps:\n1. Go to https://api.slack.com/apps\n2. Create app using the Manifest below\n3. Install to workspace → Copy Bot Token and App Token",
		"onboard.ch.slack.manifest":  "Slack App Manifest (for reference)",
		"onboard.ch.slack.bot_env":   "SLACK_BOT_TOKEN detected. Use env var?",
		"onboard.ch.slack.bot_token": "Slack Bot Token (xoxb-...)",
		"onboard.ch.slack.bot_keep":  "Slack bot token already configured. Keep it?",
		"onboard.ch.slack.app_env":   "SLACK_APP_TOKEN detected. Use env var?",
		"onboard.ch.slack.app_token": "Slack App Token (xapp-...)",
		"onboard.ch.slack.app_keep":  "Slack app token already configured. Keep it?",

		// ── channels/onboarding_telegram.go ──
		"onboard.ch.telegram.title":     "Telegram",
		"onboard.ch.telegram.intro":     "Telegram bot setup steps:\n1. Search for @BotFather in Telegram\n2. Send /newbot and follow the prompts\n3. Copy the Bot Token",
		"onboard.ch.telegram.guide":     "BotFather Guide",
		"onboard.ch.telegram.env_found": "TELEGRAM_BOT_TOKEN detected. Use env var?",
		"onboard.ch.telegram.token":     "Telegram Bot Token",
		"onboard.ch.telegram.keep":      "Telegram token already configured. Keep it?",
		"onboard.ch.telegram.handle":    "Enter Telegram Handle",

		// ── channels/onboarding_whatsapp.go ──
		"onboard.ch.whatsapp.title":      "WhatsApp",
		"onboard.ch.whatsapp.link_hint":  "WhatsApp requires linking via QR code.\nAfter completing setup, run: crabclaw gateway start\nThen scan the QR code with your phone.\nDocs: https://docs.openacosmi.dev/whatsapp",
		"onboard.ch.whatsapp.link_fail":  "WhatsApp linking failed: %s\nYou can link later: crabclaw channels login --channel whatsapp",
		"onboard.ch.whatsapp.linked":     "WhatsApp linked",
		"onboard.ch.whatsapp.link_later": "%s\nLink later: crabclaw channels login --channel whatsapp --verbose",
		"onboard.ch.whatsapp.selfchat":   "Enable self-chat mode? (use your own number)",
		"onboard.ch.whatsapp.phone":      "Enter WhatsApp phone number",
		"onboard.ch.whatsapp.phone_hint": "Phone number format guide",

		// ── channels/onboarding_signal.go ──
		"onboard.ch.signal.title":  "Signal",
		"onboard.ch.signal.setup":  "Signal requires a dedicated phone number.",
		"onboard.ch.signal.keep":   "Signal account set (%s). Keep it?",
		"onboard.ch.signal.number": "Signal bot number (E.164)",
		"onboard.ch.signal.handle": "Configure Signal Handle",
		"onboard.ch.signal.hint":   "Signal Handle setup guide",

		// ── channels/onboarding_imessage.go ──
		"onboard.ch.imessage.title":    "iMessage",
		"onboard.ch.imessage.cli_path": "imsg CLI path",
		"onboard.ch.imessage.cli_req":  "imsg CLI path required to enable iMessage.",
		"onboard.ch.imessage.setup":    "iMessage setup guide",
		"onboard.ch.imessage.handle":   "Configure iMessage Handle",
		"onboard.ch.imessage.hint":     "iMessage Handle setup guide",

		// ── channels/onboarding_channel_access.go ──
		"onboard.ch.access.title":   "%s access",
		"onboard.ch.access.input":   "Enter allowed identifiers (comma separated)",
		"onboard.ch.access.confirm": "Configure custom access rules for %s?",
	})
}
