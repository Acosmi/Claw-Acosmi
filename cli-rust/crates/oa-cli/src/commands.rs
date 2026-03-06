/// Subcommand routing for the Claw Acosmi CLI.
///
/// Defines the `Commands` enum connecting all command crates, defines
/// Clap `Args` wrappers for each command group, and provides the `dispatch`
/// function to route to the correct handler.
///
/// Source: `backend/cmd/openacosmi/main.go` - `registerAllCommands()`
use std::collections::HashMap;

use anyhow::Result;
use clap::{Args, Subcommand};

// ---------------------------------------------------------------------------
// Top-level subcommands
// ---------------------------------------------------------------------------

/// All top-level subcommands.
#[derive(Debug, Subcommand)]
pub enum Commands {
    /// System health check — probe gateway, channels, agents, sessions.
    Health(HealthArgs),

    /// Show system status dashboard.
    Status(StatusArgs),

    /// List session entries from the session store.
    Sessions(SessionsArgs),

    /// Channel management (list, add, remove, resolve, capabilities, logs, status).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Channels(ChannelsCommand),

    /// Model configuration (list, set, aliases, fallbacks).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Models(ModelsCommand),

    /// Agent management (list, add, delete, identity).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Agents(AgentsCommand),

    /// Sandbox container management (list, recreate, explain).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Sandbox(SandboxCommand),

    /// Coding sub-agent — MCP server with edit, read, grep, glob, bash tools.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Coder(CoderCommand),

    /// Authentication wizard — configure auth providers and API keys.
    Auth(AuthArgs),

    /// Configuration wizard — gateway, channels, daemon, workspace.
    Configure(ConfigureArgs),

    /// Initial onboarding setup wizard.
    Onboard(OnboardArgs),

    /// System diagnostics and repair.
    Doctor(DoctorArgs),

    /// Run or send a message to an AI agent.
    Agent(AgentArgs),

    /// Open the Claw Acosmi dashboard in a browser.
    Dashboard(DashboardArgs),

    /// Search Claw Acosmi documentation.
    Docs(DocsArgs),

    /// Reset Claw Acosmi state (config, sessions, workspace).
    Reset(ResetArgs),

    /// Initial workspace setup.
    Setup(SetupArgs),

    /// Uninstall Claw Acosmi components.
    Uninstall(UninstallArgs),

    /// Send a message through a channel.
    Message(MessageArgs),

    /// Comprehensive status report (debug output).
    StatusAll(StatusAllArgs),

    /// Probe gateway endpoints.
    GatewayStatus(GatewayStatusArgs),

    /// Gateway service management (run, start, stop, status, install, uninstall, call, health, probe, discover).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Gateway(GatewayCommand),

    /// Daemon service management (legacy alias for gateway).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Daemon(DaemonCommand),

    /// View and manage gateway logs.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Logs(LogsCommand),

    /// Agent memory and vector storage management.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Memory(MemoryCommand),

    /// Scheduled job management (cron).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Cron(CronCommand),

    /// Direct config file manipulation (get, set, unset).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Config(ConfigCommand),

    /// Generate shell completion scripts.
    Completion(CompletionArgs),

    /// ACP (Agent Client Protocol) bridge commands.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Acp(AcpCommand),

    /// Manage exec approvals (allowlists).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Approvals(ApprovalsCommand),

    /// Device pairing and token management.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Devices(DevicesCommand),

    /// Directory lookups (contacts, groups, self).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Directory(DirectoryCommand),

    /// DNS helpers for wide-area discovery.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Dns(DnsCommand),

    /// Headless node host management.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Node(NodeCommand),

    /// DM pairing request management.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Pairing(PairingCommand),

    /// System events, heartbeat, and presence.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    System(SystemCommand),

    /// Terminal UI connected to the Gateway.
    Tui(TuiArgs),

    /// Self-update and channel management.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Update(UpdateCommand),

    /// Voice call plugin commands.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Voicecall(VoicecallCommand),

    /// Webhook and Gmail Pub/Sub integration.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Webhooks(WebhooksCommand),

    /// Skill management (list, info, check).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Skills(SkillsCommand),

    /// Plugin management (list, info, install, enable, disable, doctor).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Plugins(PluginsCommand),

    /// Security audit and management.
    Security(SecurityArgs),

    /// Hook management (list, info, check, enable, disable, install, update).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Hooks(HooksCommand),

    /// Browser automation (status, start, stop, tabs, open, screenshot, profiles).
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Browser(BrowserCommand),
}

// ---------------------------------------------------------------------------
// Per-command Args structs
// ---------------------------------------------------------------------------

/// Arguments for the `health` command.
#[derive(Debug, Args)]
pub struct HealthArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,

    /// Enable verbose output.
    #[arg(long, short)]
    pub verbose: bool,

    /// Timeout in milliseconds for the gateway health call.
    #[arg(long)]
    pub timeout_ms: Option<u64>,
}

/// Arguments for the `status` command.
#[derive(Debug, Args)]
pub struct StatusArgs {
    /// Enable deep scanning.
    #[arg(long)]
    pub deep: bool,

    /// Show usage statistics.
    #[arg(long)]
    pub usage: bool,

    /// Timeout in milliseconds.
    #[arg(long)]
    pub timeout_ms: Option<u64>,

    /// Show all status details.
    #[arg(long)]
    pub all: bool,
}

/// Arguments for the `sessions` command.
#[derive(Debug, Args)]
pub struct SessionsArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,

    /// Override the session store path.
    #[arg(long)]
    pub store: Option<String>,

    /// Filter to sessions active within the last N minutes.
    #[arg(long)]
    pub active: Option<String>,
}

/// Arguments for the `status-all` command.
#[derive(Debug, Args)]
pub struct StatusAllArgs {
    /// Timeout in milliseconds.
    #[arg(long)]
    pub timeout_ms: Option<u64>,
}

/// Arguments for the `gateway-status` command.
#[derive(Debug, Args)]
pub struct GatewayStatusArgs {
    /// Gateway URL to probe.
    #[arg(long)]
    pub url: Option<String>,

    /// Auth token for the gateway.
    #[arg(long)]
    pub token: Option<String>,

    /// Auth password for the gateway.
    #[arg(long)]
    pub password: Option<String>,

    /// Timeout for the probe.
    #[arg(long)]
    pub timeout: Option<String>,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

// -- Channels subcommands ---------------------------------------------------

/// Channels subcommand group.
#[derive(Debug, Args)]
pub struct ChannelsCommand {
    #[command(subcommand)]
    pub action: ChannelsAction,
}

/// Individual channel actions.
#[derive(Debug, Subcommand)]
pub enum ChannelsAction {
    /// List configured channels.
    List(ChannelsListArgs),
    /// Add a new channel account.
    Add(ChannelsAddArgs),
    /// Remove or disable a channel account.
    Remove(ChannelsRemoveArgs),
    /// Resolve a channel contact.
    Resolve(ChannelsResolveArgs),
    /// Show channel capabilities.
    Capabilities(ChannelsCapabilitiesArgs),
    /// View channel logs.
    Logs(ChannelsLogsArgs),
    /// Check channel status.
    Status(ChannelsStatusArgs),
    /// Login to a channel account.
    Login(ChannelsLoginArgs),
    /// Logout from a channel account.
    Logout(ChannelsLogoutArgs),
}

#[derive(Debug, Args)]
pub struct ChannelsLoginArgs {
    /// Channel kind (whatsapp, telegram, discord, slack, signal, imessage).
    pub channel: String,
    /// Account identifier.
    pub account: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct ChannelsLogoutArgs {
    /// Channel kind.
    pub channel: String,
    /// Account identifier.
    pub account: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct ChannelsListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Include usage information.
    #[arg(long)]
    pub usage: bool,
}

#[derive(Debug, Args)]
pub struct ChannelsAddArgs {
    /// Channel kind (whatsapp, telegram, discord, slack, signal, imessage).
    pub channel: String,
    /// Account identifier.
    pub account: Option<String>,
}

#[derive(Debug, Args)]
pub struct ChannelsRemoveArgs {
    /// Channel identifier.
    pub channel: String,
    /// Account identifier.
    pub account: Option<String>,
    /// Delete entirely (vs just disable).
    #[arg(long)]
    pub delete: bool,
}

#[derive(Debug, Args)]
pub struct ChannelsResolveArgs {
    /// Entries to resolve.
    pub entries: Vec<String>,
    /// Resolution kind.
    #[arg(long)]
    pub kind: Option<String>,
    /// Channel filter.
    #[arg(long)]
    pub channel: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct ChannelsCapabilitiesArgs {
    /// Channel filter.
    #[arg(long)]
    pub channel: Option<String>,
    /// Account filter.
    #[arg(long)]
    pub account: Option<String>,
    /// Timeout for probes.
    #[arg(long)]
    pub timeout: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct ChannelsLogsArgs {
    /// Channel filter.
    #[arg(long)]
    pub channel: Option<String>,
    /// Number of log lines to display.
    #[arg(long, short)]
    pub lines: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct ChannelsStatusArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Include probe results.
    #[arg(long)]
    pub probe: bool,
    /// Timeout for probes.
    #[arg(long)]
    pub timeout: Option<String>,
}

// -- Models subcommands -----------------------------------------------------

/// Models subcommand group.
#[derive(Debug, Args)]
pub struct ModelsCommand {
    #[command(subcommand)]
    pub action: ModelsAction,
}

/// Individual model actions.
#[derive(Debug, Subcommand)]
pub enum ModelsAction {
    /// List configured models.
    List(ModelsListArgs),
    /// Set the primary model.
    Set(ModelsSetArgs),
    /// Set the primary image model.
    SetImage(ModelsSetImageArgs),
    /// Manage model aliases.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Aliases(ModelsAliasesCommand),
    /// Manage model fallbacks.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    Fallbacks(ModelsFallbacksCommand),
    /// Manage image model fallbacks.
    #[command(subcommand_required = true, arg_required_else_help = true)]
    ImageFallbacks(ModelsImageFallbacksCommand),
}

#[derive(Debug, Args)]
pub struct ModelsListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Plain output (no color/formatting).
    #[arg(long)]
    pub plain: bool,
}

#[derive(Debug, Args)]
pub struct ModelsSetArgs {
    /// Model identifier to set as primary.
    pub model: String,
}

#[derive(Debug, Args)]
pub struct ModelsSetImageArgs {
    /// Model identifier to set as primary image model.
    pub model: String,
}

/// Aliases subcommand group.
#[derive(Debug, Args)]
pub struct ModelsAliasesCommand {
    #[command(subcommand)]
    pub action: ModelsAliasesAction,
}

#[derive(Debug, Subcommand)]
pub enum ModelsAliasesAction {
    /// List all aliases.
    List(ModelsAliasesListArgs),
    /// Add an alias.
    Add(ModelsAliasesAddArgs),
    /// Remove an alias.
    Remove(ModelsAliasesRemoveArgs),
}

#[derive(Debug, Args)]
pub struct ModelsAliasesListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Plain output.
    #[arg(long)]
    pub plain: bool,
}

#[derive(Debug, Args)]
pub struct ModelsAliasesAddArgs {
    /// Alias name.
    pub alias: String,
    /// Model identifier.
    pub model: String,
}

#[derive(Debug, Args)]
pub struct ModelsAliasesRemoveArgs {
    /// Alias name to remove.
    pub alias: String,
}

/// Fallbacks subcommand group.
#[derive(Debug, Args)]
pub struct ModelsFallbacksCommand {
    #[command(subcommand)]
    pub action: ModelsFallbacksAction,
}

#[derive(Debug, Subcommand)]
pub enum ModelsFallbacksAction {
    /// List model fallbacks.
    List(ModelsFallbacksListArgs),
    /// Add a model fallback.
    Add(ModelsFallbacksAddArgs),
    /// Remove a model fallback.
    Remove(ModelsFallbacksRemoveArgs),
    /// Clear all model fallbacks.
    Clear,
}

#[derive(Debug, Args)]
pub struct ModelsFallbacksListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Plain output.
    #[arg(long)]
    pub plain: bool,
}

#[derive(Debug, Args)]
pub struct ModelsFallbacksAddArgs {
    /// Model identifier.
    pub model: String,
}

#[derive(Debug, Args)]
pub struct ModelsFallbacksRemoveArgs {
    /// Model identifier.
    pub model: String,
}

/// Image fallbacks subcommand group.
#[derive(Debug, Args)]
pub struct ModelsImageFallbacksCommand {
    #[command(subcommand)]
    pub action: ModelsImageFallbacksAction,
}

#[derive(Debug, Subcommand)]
pub enum ModelsImageFallbacksAction {
    /// List image model fallbacks.
    List(ModelsImageFallbacksListArgs),
    /// Add an image model fallback.
    Add(ModelsImageFallbacksAddArgs),
    /// Remove an image model fallback.
    Remove(ModelsImageFallbacksRemoveArgs),
    /// Clear all image model fallbacks.
    Clear,
}

#[derive(Debug, Args)]
pub struct ModelsImageFallbacksListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Plain output.
    #[arg(long)]
    pub plain: bool,
}

#[derive(Debug, Args)]
pub struct ModelsImageFallbacksAddArgs {
    /// Model identifier.
    pub model: String,
}

#[derive(Debug, Args)]
pub struct ModelsImageFallbacksRemoveArgs {
    /// Model identifier.
    pub model: String,
}

// -- Agents subcommands -----------------------------------------------------

/// Agents subcommand group (plural — agent lifecycle management).
#[derive(Debug, Args)]
pub struct AgentsCommand {
    #[command(subcommand)]
    pub action: AgentsAction,
}

/// Individual agents actions.
#[derive(Debug, Subcommand)]
pub enum AgentsAction {
    /// List configured agents.
    List(AgentsListArgs),
    /// Add a new agent.
    Add(AgentsAddArgs),
    /// Delete an agent.
    Delete(AgentsDeleteArgs),
    /// Set an agent's identity (name, theme, emoji, avatar).
    SetIdentity(AgentsSetIdentityArgs),
}

#[derive(Debug, Args)]
pub struct AgentsListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Show binding details.
    #[arg(long)]
    pub bindings: bool,
}

#[derive(Debug, Args)]
pub struct AgentsAddArgs {
    /// Agent identifier.
    pub id: String,
    /// Display name.
    #[arg(long)]
    pub name: Option<String>,
    /// Workspace path.
    #[arg(long)]
    pub workspace: Option<String>,
    /// Model override.
    #[arg(long)]
    pub model: Option<String>,
}

#[derive(Debug, Args)]
pub struct AgentsDeleteArgs {
    /// Agent identifier.
    pub id: String,
    /// Skip confirmation prompt.
    #[arg(long, short)]
    pub yes: bool,
}

#[derive(Debug, Args)]
pub struct AgentsSetIdentityArgs {
    /// Agent identifier.
    pub id: String,
    /// Display name.
    #[arg(long)]
    pub name: Option<String>,
    /// Theme.
    #[arg(long)]
    pub theme: Option<String>,
    /// Emoji.
    #[arg(long)]
    pub emoji: Option<String>,
    /// Avatar URL/path.
    #[arg(long)]
    pub avatar: Option<String>,
}

// -- Sandbox subcommands ----------------------------------------------------

/// Sandbox subcommand group.
#[derive(Debug, Args)]
pub struct SandboxCommand {
    #[command(subcommand)]
    pub action: SandboxAction,
}

/// Individual sandbox actions.
#[derive(Debug, Subcommand)]
pub enum SandboxAction {
    /// List sandbox containers.
    List(SandboxListArgs),
    /// Recreate a sandbox container.
    Recreate(SandboxRecreateArgs),
    /// Explain sandbox configuration.
    Explain(SandboxExplainArgs),
    /// Execute a command inside a sandboxed process.
    Run(SandboxRunArgs),
    /// Start a persistent sandbox Worker process (internal).
    WorkerStart(SandboxWorkerStartArgs),
}

#[derive(Debug, Args)]
pub struct SandboxListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Show browser containers only.
    #[arg(long)]
    pub browser: bool,
}

#[derive(Debug, Args)]
pub struct SandboxRecreateArgs {
    /// Filter by agent ID.
    #[arg(long)]
    pub agent: Option<String>,
    /// Filter by session key.
    #[arg(long)]
    pub session: Option<String>,
    /// Recreate all containers.
    #[arg(long)]
    pub all: bool,
    /// Recreate browser containers.
    #[arg(long)]
    pub browser: bool,
    /// Skip confirmation prompt.
    #[arg(long)]
    pub force: bool,
}

#[derive(Debug, Args)]
pub struct SandboxExplainArgs {
    /// Agent ID to explain.
    #[arg(long)]
    pub agent: Option<String>,
    /// Session key override.
    #[arg(long)]
    pub session: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct SandboxRunArgs {
    /// Security level: deny (L0), allowlist (L1), or sandboxed (L2). Legacy aliases: sandbox, full.
    #[arg(long, default_value = "allowlist")]
    pub security: String,
    /// Workspace directory (mounted into the sandbox).
    #[arg(long, default_value = ".")]
    pub workspace: String,
    /// Network policy: none, restricted, or host.
    #[arg(long)]
    pub net: Option<String>,
    /// Execution timeout in seconds.
    #[arg(long)]
    pub timeout: Option<u64>,
    /// Output format: json or text.
    #[arg(long, default_value = "json")]
    pub format: String,
    /// Backend preference: auto, native, or docker.
    #[arg(long, default_value = "auto")]
    pub backend: String,
    /// Additional bind mount (host:sandbox[:ro|rw]). Can be repeated.
    #[arg(long = "mount")]
    pub mounts: Vec<String>,
    /// Environment variable (KEY=VALUE). Can be repeated.
    #[arg(long = "env")]
    pub envs: Vec<String>,
    /// Memory limit in bytes (0 = no limit).
    #[arg(long, default_value = "0")]
    pub memory: u64,
    /// CPU limit in millicores (1000 = 1 core, 0 = no limit).
    #[arg(long, default_value = "0")]
    pub cpu: u32,
    /// Maximum number of processes (0 = no limit).
    #[arg(long, default_value = "0")]
    pub pids: u32,
    /// Show execution plan without running (also triggered automatically for L2/sandboxed).
    #[arg(long)]
    pub dry_run: bool,
    /// Command and arguments to execute.
    #[arg(trailing_var_arg = true, required = true)]
    pub command: Vec<String>,
}

/// Arguments for the `sandbox worker-start` subcommand (internal).
#[derive(Debug, Args)]
pub struct SandboxWorkerStartArgs {
    /// Workspace directory for sandboxed commands.
    #[arg(long, default_value = ".")]
    pub workspace: String,
    /// Default timeout in seconds for commands.
    #[arg(long, default_value = "120")]
    pub timeout: u64,
    /// Security level: deny (L0), allowlist (L1), sandboxed (L2). Legacy aliases: sandbox, full.
    #[arg(long, default_value = "allowlist")]
    pub security_level: String,
    /// Idle timeout in seconds. Worker exits if no request arrives within this duration.
    /// 0 = no idle timeout (wait forever).
    #[arg(long, default_value = "0")]
    pub idle_timeout: u64,
}

// -- Coder subcommands ------------------------------------------------------

/// Coder sub-agent command group.
#[derive(Debug, Args)]
pub struct CoderCommand {
    #[command(subcommand)]
    pub action: CoderAction,
}

/// Individual coder actions.
#[derive(Debug, Subcommand)]
pub enum CoderAction {
    /// Start the MCP coding agent server (stdin/stdout JSON-RPC 2.0).
    Start(CoderStartArgs),
}

#[derive(Debug, Args)]
pub struct CoderStartArgs {
    /// Workspace directory for file operations.
    #[arg(long, default_value = ".")]
    pub workspace: String,
    /// Enable sandboxed execution for bash tool.
    #[arg(long)]
    pub sandboxed: bool,
}

// -- Auth -------------------------------------------------------------------

/// Arguments for the `auth` command.
#[derive(Debug, Args)]
pub struct AuthArgs;

// -- Configure --------------------------------------------------------------

/// Arguments for the `configure` command.
#[derive(Debug, Args)]
pub struct ConfigureArgs {
    /// Run specific sections only (comma-separated).
    #[arg(long, value_delimiter = ',')]
    pub sections: Option<Vec<String>>,
}

// -- Onboard ----------------------------------------------------------------

/// Arguments for the `onboard` command.
#[derive(Debug, Args)]
pub struct OnboardArgs {
    /// Gateway mode (local or remote).
    #[arg(long)]
    pub mode: Option<String>,

    /// Auth choice preset.
    #[arg(long)]
    pub auth_choice: Option<String>,

    /// Workspace directory.
    #[arg(long)]
    pub workspace: Option<String>,

    /// Run non-interactively.
    #[arg(long)]
    pub non_interactive: bool,

    /// Accept risk for non-interactive mode.
    #[arg(long)]
    pub accept_risk: bool,

    /// Reset existing config before onboarding.
    #[arg(long)]
    pub reset: bool,

    /// Anthropic API key.
    #[arg(long)]
    pub anthropic_api_key: Option<String>,

    /// OpenAI API key.
    #[arg(long)]
    pub openai_api_key: Option<String>,

    /// OpenRouter API key.
    #[arg(long)]
    pub openrouter_api_key: Option<String>,

    /// Gateway port.
    #[arg(long)]
    pub gateway_port: Option<u16>,

    /// Gateway bind address.
    #[arg(long)]
    pub gateway_bind: Option<String>,

    /// Gateway auth mode.
    #[arg(long)]
    pub gateway_auth: Option<String>,

    /// Gateway token.
    #[arg(long)]
    pub gateway_token: Option<String>,

    /// Gateway password.
    #[arg(long)]
    pub gateway_password: Option<String>,

    /// Install daemon service.
    #[arg(long)]
    pub install_daemon: bool,

    /// Daemon runtime (node or bun).
    #[arg(long)]
    pub daemon_runtime: Option<String>,

    /// Skip channel setup.
    #[arg(long)]
    pub skip_channels: bool,

    /// Skip skills setup.
    #[arg(long)]
    pub skip_skills: bool,

    /// Skip health check.
    #[arg(long)]
    pub skip_health: bool,

    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

// -- Doctor -----------------------------------------------------------------

/// Arguments for the `doctor` command.
#[derive(Debug, Args)]
pub struct DoctorArgs {
    /// Accept all prompts automatically.
    #[arg(long, short)]
    pub yes: bool,

    /// Run without interactive prompts.
    #[arg(long)]
    pub non_interactive: bool,

    /// Enable deep scanning.
    #[arg(long)]
    pub deep: bool,

    /// Apply recommended repairs automatically.
    #[arg(long)]
    pub repair: bool,

    /// Allow aggressive / destructive repairs.
    #[arg(long)]
    pub force: bool,
}

// -- Agent (singular) -------------------------------------------------------

/// Arguments for the `agent` command (run a single agent).
#[derive(Debug, Args)]
pub struct AgentArgs {
    /// Message to send to the agent.
    pub message: String,

    /// Agent ID override.
    #[arg(long)]
    pub agent_id: Option<String>,

    /// Delivery target (phone number, email).
    #[arg(long)]
    pub to: Option<String>,

    /// Session identifier.
    #[arg(long)]
    pub session_id: Option<String>,

    /// Session key.
    #[arg(long)]
    pub session_key: Option<String>,

    /// Thinking level (off, minimal, low, medium, high, xhigh).
    #[arg(long)]
    pub thinking: Option<String>,

    /// Output as JSON.
    #[arg(long)]
    pub json: bool,

    /// Timeout in seconds.
    #[arg(long)]
    pub timeout: Option<String>,

    /// Verbose output level (on, full, off).
    #[arg(long)]
    pub verbose: Option<String>,
}

// -- Supporting commands ----------------------------------------------------

/// Arguments for the `dashboard` command.
#[derive(Debug, Args)]
pub struct DashboardArgs {
    /// Do not open the browser.
    #[arg(long)]
    pub no_open: bool,
}

/// Arguments for the `docs` command.
#[derive(Debug, Args)]
pub struct DocsArgs {
    /// Search query.
    pub query: Vec<String>,
}

/// Arguments for the `reset` command.
#[derive(Debug, Args)]
pub struct ResetArgs {
    /// Reset scope (config, config-creds-sessions, full).
    pub scope: Option<String>,
    /// Dry run — show what would be removed.
    #[arg(long)]
    pub dry_run: bool,
    /// Accept all prompts.
    #[arg(long, short)]
    pub yes: bool,
    /// Disable interactive prompts.
    #[arg(long)]
    pub non_interactive: bool,
}

/// Arguments for the `setup` command.
#[derive(Debug, Args)]
pub struct SetupArgs {
    /// Workspace directory.
    pub workspace: Option<String>,
}

/// Arguments for the `uninstall` command.
#[derive(Debug, Args)]
pub struct UninstallArgs {
    /// Include the gateway service scope.
    #[arg(long)]
    pub service: bool,
    /// Include the state+config scope.
    #[arg(long)]
    pub state: bool,
    /// Include workspace directories scope.
    #[arg(long)]
    pub workspace: bool,
    /// Include the macOS app scope.
    #[arg(long)]
    pub app: bool,
    /// Include all scopes.
    #[arg(long)]
    pub all: bool,
    /// Accept all prompts.
    #[arg(long, short)]
    pub yes: bool,
    /// Disable interactive prompts.
    #[arg(long)]
    pub non_interactive: bool,
    /// Dry run.
    #[arg(long)]
    pub dry_run: bool,
}

/// Arguments for the `message` command.
#[derive(Debug, Args)]
pub struct MessageArgs {
    /// Action to perform (send, deliver, forward, reply, react).
    pub action: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
    /// Dry run.
    #[arg(long)]
    pub dry_run: bool,
}

/// Arguments for the `completion` command.
#[derive(Debug, Args)]
pub struct CompletionArgs {
    /// Shell to generate completions for (bash, zsh, fish, powershell).
    pub shell: clap_complete::Shell,
}

// -- Gateway subcommands ----------------------------------------------------

/// Gateway subcommand group.
#[derive(Debug, Args)]
pub struct GatewayCommand {
    #[command(subcommand)]
    pub action: GatewayAction,
}

/// Individual gateway actions.
#[derive(Debug, Subcommand)]
pub enum GatewayAction {
    /// Run the gateway process in the foreground.
    Run(GatewayRunArgs),
    /// Start the gateway as a background service.
    Start(GatewayStartArgs),
    /// Stop the gateway service.
    Stop,
    /// Show gateway service status.
    Status(GatewayStatusCmdArgs),
    /// Install the gateway as a system service.
    Install(GatewayInstallArgs),
    /// Uninstall the gateway system service.
    Uninstall,
    /// Call a gateway RPC method directly.
    Call(GatewayCallArgs),
    /// Show gateway usage cost.
    UsageCost(GatewayUsageCostArgs),
    /// Show gateway health.
    Health(GatewayHealthArgs),
    /// Probe gateway endpoints (HTTP, RPC, discovery).
    Probe(GatewayProbeArgs),
    /// Discover gateway instances on the network.
    Discover(GatewayDiscoverArgs),
}

#[derive(Debug, Args)]
pub struct GatewayRunArgs {
    /// Port to listen on.
    #[arg(long)]
    pub port: Option<u16>,
    /// Path to control UI static files.
    #[arg(long)]
    pub control_ui_dir: Option<String>,
}

#[derive(Debug, Args)]
pub struct GatewayStartArgs {
    /// Port to listen on.
    #[arg(long)]
    pub port: Option<u16>,
    /// Force restart if already running.
    #[arg(long)]
    pub force: bool,
}

#[derive(Debug, Args)]
pub struct GatewayStatusCmdArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct GatewayInstallArgs {
    /// Port to listen on.
    #[arg(long)]
    pub port: Option<u16>,
}

#[derive(Debug, Args)]
pub struct GatewayCallArgs {
    /// RPC method name.
    pub method: String,
    /// JSON-encoded parameters.
    #[arg(long)]
    pub params: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct GatewayUsageCostArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct GatewayHealthArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct GatewayProbeArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct GatewayDiscoverArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

// -- Daemon subcommands (legacy alias) --------------------------------------

/// Daemon subcommand group.
#[derive(Debug, Args)]
pub struct DaemonCommand {
    #[command(subcommand)]
    pub action: DaemonAction,
}

/// Individual daemon actions (legacy aliases for gateway).
#[derive(Debug, Subcommand)]
pub enum DaemonAction {
    /// Show daemon status.
    Status(DaemonStatusArgs),
    /// Start the daemon.
    Start,
    /// Stop the daemon.
    Stop,
    /// Restart the daemon.
    Restart,
    /// Install the daemon service.
    Install,
    /// Uninstall the daemon service.
    Uninstall,
}

#[derive(Debug, Args)]
pub struct DaemonStatusArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

// -- Logs subcommands -------------------------------------------------------

/// Logs subcommand group.
#[derive(Debug, Args)]
pub struct LogsCommand {
    #[command(subcommand)]
    pub action: LogsAction,
}

/// Individual log actions.
#[derive(Debug, Subcommand)]
pub enum LogsAction {
    /// Follow (tail) the most recent log file.
    Follow(LogsFollowArgs),
    /// List available log files.
    List(LogsListArgs),
    /// Show contents of a log file.
    Show(LogsShowArgs),
    /// Clear all log files.
    Clear(LogsClearArgs),
    /// Export all logs to a single file.
    Export(LogsExportArgs),
}

#[derive(Debug, Args)]
pub struct LogsFollowArgs {
    /// Number of lines to display.
    #[arg(long, short)]
    pub lines: Option<usize>,
    /// Filter by channel.
    #[arg(long)]
    pub channel: Option<String>,
}

#[derive(Debug, Args)]
pub struct LogsListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct LogsShowArgs {
    /// Specific log file to show.
    pub file: Option<String>,
    /// Number of lines to display.
    #[arg(long, short)]
    pub lines: Option<usize>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct LogsClearArgs {
    /// Skip confirmation prompt.
    #[arg(long, short)]
    pub yes: bool,
}

#[derive(Debug, Args)]
pub struct LogsExportArgs {
    /// Output file path.
    pub output: String,
}

// -- Memory subcommands -----------------------------------------------------

/// Memory subcommand group.
#[derive(Debug, Args)]
pub struct MemoryCommand {
    #[command(subcommand)]
    pub action: MemoryAction,
}

/// Individual memory actions.
#[derive(Debug, Subcommand)]
pub enum MemoryAction {
    /// Show memory system status.
    Status(MemoryStatusArgs),
    /// Trigger memory re-indexing.
    Index,
    /// Check memory system health.
    Check(MemoryCheckArgs),
    /// Search agent memory.
    Search(MemorySearchArgs),
}

#[derive(Debug, Args)]
pub struct MemoryStatusArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct MemoryCheckArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct MemorySearchArgs {
    /// Search query.
    pub query: String,
    /// Maximum results to return.
    #[arg(long)]
    pub limit: Option<usize>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

// -- Cron subcommands -------------------------------------------------------

/// Cron subcommand group.
#[derive(Debug, Args)]
pub struct CronCommand {
    #[command(subcommand)]
    pub action: CronAction,
}

/// Individual cron actions.
#[derive(Debug, Subcommand)]
pub enum CronAction {
    /// Show cron scheduler status.
    Status(CronStatusArgs),
    /// List scheduled jobs.
    List(CronListArgs),
    /// Add a scheduled job.
    Add(CronAddArgs),
    /// Edit a scheduled job.
    Edit(CronEditArgs),
    /// Remove a scheduled job.
    #[command(alias = "rm")]
    Remove(CronRemoveArgs),
    /// Enable a scheduled job.
    Enable(CronEnableArgs),
    /// Disable a scheduled job.
    Disable(CronDisableArgs),
    /// View run history for a job.
    Runs(CronRunsArgs),
    /// Trigger immediate execution of a job.
    Run(CronRunArgs),
}

#[derive(Debug, Args)]
pub struct CronStatusArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct CronListArgs {
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct CronAddArgs {
    /// Job name.
    #[arg(long)]
    pub name: String,
    /// Cron schedule expression.
    #[arg(long)]
    pub schedule: String,
    /// Agent ID to execute.
    #[arg(long)]
    pub agent_id: String,
    /// Message to send to the agent.
    #[arg(long)]
    pub message: String,
}

#[derive(Debug, Args)]
pub struct CronEditArgs {
    /// Job ID.
    pub id: String,
    /// New name.
    #[arg(long)]
    pub name: Option<String>,
    /// New schedule.
    #[arg(long)]
    pub schedule: Option<String>,
    /// New agent ID.
    #[arg(long)]
    pub agent_id: Option<String>,
    /// New message.
    #[arg(long)]
    pub message: Option<String>,
}

#[derive(Debug, Args)]
pub struct CronRemoveArgs {
    /// Job ID.
    pub id: String,
}

#[derive(Debug, Args)]
pub struct CronEnableArgs {
    /// Job ID.
    pub id: String,
}

#[derive(Debug, Args)]
pub struct CronDisableArgs {
    /// Job ID.
    pub id: String,
}

#[derive(Debug, Args)]
pub struct CronRunsArgs {
    /// Job ID.
    pub id: String,
    /// Maximum runs to display.
    #[arg(long)]
    pub limit: Option<usize>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct CronRunArgs {
    /// Job ID.
    pub id: String,
}

// -- Config subcommands -----------------------------------------------------

/// Config subcommand group.
#[derive(Debug, Args)]
pub struct ConfigCommand {
    #[command(subcommand)]
    pub action: ConfigAction,
}

/// Individual config actions.
#[derive(Debug, Subcommand)]
pub enum ConfigAction {
    /// Get a config value by dot-separated path.
    Get(ConfigGetArgs),
    /// Set a config value by dot-separated path.
    Set(ConfigSetArgs),
    /// Remove a config value by dot-separated path.
    Unset(ConfigUnsetArgs),
}

#[derive(Debug, Args)]
pub struct ConfigGetArgs {
    /// Dot-separated config path (e.g. "gateway.port").
    pub path: String,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct ConfigSetArgs {
    /// Dot-separated config path.
    pub path: String,
    /// Value to set.
    pub value: String,
}

#[derive(Debug, Args)]
pub struct ConfigUnsetArgs {
    /// Dot-separated config path to remove.
    pub path: String,
}

// -- ACP subcommands --------------------------------------------------------

/// ACP subcommand group.
#[derive(Debug, Args)]
pub struct AcpCommand {
    #[command(subcommand)]
    pub action: AcpAction,
}

/// Individual ACP actions.
#[derive(Debug, Subcommand)]
pub enum AcpAction {
    /// Show ACP bridge status.
    Status,
    /// Invoke an ACP method.
    Invoke(AcpInvokeArgs),
}

#[derive(Debug, Args)]
pub struct AcpInvokeArgs {
    /// ACP method name.
    pub method: String,
}

// -- Approvals subcommands --------------------------------------------------

/// Approvals subcommand group.
#[derive(Debug, Args)]
pub struct ApprovalsCommand {
    #[command(subcommand)]
    pub action: ApprovalsAction,
}

/// Individual approvals actions.
#[derive(Debug, Subcommand)]
pub enum ApprovalsAction {
    /// Get current exec approvals.
    Get(ApprovalsGetArgs),
    /// Set approvals from a file.
    Set(ApprovalsSetArgs),
    /// Add a pattern to the allowlist.
    AllowlistAdd(AllowlistAddArgs),
    /// Remove a pattern from the allowlist.
    AllowlistRemove(AllowlistRemoveArgs),
}

#[derive(Debug, Args)]
pub struct ApprovalsGetArgs {
    /// Target the gateway.
    #[arg(long)]
    pub gateway: bool,
    /// Node approvals must be managed on the node host directly (currently unsupported here).
    #[arg(long)]
    pub node: Option<String>,
    /// Output as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct ApprovalsSetArgs {
    /// Path to the approvals file (JSON or YAML).
    pub file: String,
    /// Target the gateway.
    #[arg(long)]
    pub gateway: bool,
    /// Node approvals must be managed on the node host directly (currently unsupported here).
    #[arg(long)]
    pub node: Option<String>,
}

#[derive(Debug, Args)]
pub struct AllowlistAddArgs {
    /// Glob pattern to add.
    pub pattern: String,
    /// Restrict to a specific agent.
    #[arg(long)]
    pub agent: Option<String>,
    /// Target a specific node.
    #[arg(long)]
    pub node: Option<String>,
}

#[derive(Debug, Args)]
pub struct AllowlistRemoveArgs {
    /// Glob pattern to remove.
    pub pattern: String,
}

// -- Devices subcommands ----------------------------------------------------

/// Devices subcommand group.
#[derive(Debug, Args)]
pub struct DevicesCommand {
    #[command(subcommand)]
    pub action: DevicesAction,
}

/// Individual device actions.
#[derive(Debug, Subcommand)]
pub enum DevicesAction {
    /// List paired devices.
    List,
    /// Approve a pairing request.
    Approve(DevicesApproveArgs),
    /// Reject a pairing request.
    Reject(DevicesRejectArgs),
    /// Rotate a device token.
    Rotate(DevicesRotateArgs),
    /// Revoke a device token.
    Revoke(DevicesRevokeArgs),
}

#[derive(Debug, Args)]
pub struct DevicesApproveArgs {
    /// Pairing request ID.
    pub request_id: String,
}

#[derive(Debug, Args)]
pub struct DevicesRejectArgs {
    /// Pairing request ID.
    pub request_id: String,
}

#[derive(Debug, Args)]
pub struct DevicesRotateArgs {
    /// Device identifier.
    pub device: String,
    /// Token role.
    pub role: String,
}

#[derive(Debug, Args)]
pub struct DevicesRevokeArgs {
    /// Device identifier.
    pub device: String,
    /// Token role.
    pub role: String,
}

// -- Directory subcommands --------------------------------------------------

/// Directory subcommand group.
#[derive(Debug, Args)]
pub struct DirectoryCommand {
    #[command(subcommand)]
    pub action: DirectoryAction,
}

/// Individual directory actions.
#[derive(Debug, Subcommand)]
pub enum DirectoryAction {
    /// Look up self identity on a channel.
    #[command(name = "self")]
    Self_(DirectorySelfArgs),
    /// List peers on a channel.
    Peers(DirectoryPeersArgs),
    /// List groups on a channel.
    Groups(DirectoryGroupsArgs),
}

#[derive(Debug, Args)]
pub struct DirectorySelfArgs {
    /// Channel kind (whatsapp, telegram, etc.).
    pub channel: String,
}

#[derive(Debug, Args)]
pub struct DirectoryPeersArgs {
    /// Channel kind.
    pub channel: String,
    /// Search query.
    #[arg(long)]
    pub query: Option<String>,
}

#[derive(Debug, Args)]
pub struct DirectoryGroupsArgs {
    /// Channel kind.
    pub channel: String,
    /// Search query.
    #[arg(long)]
    pub query: Option<String>,
}

// -- DNS subcommands --------------------------------------------------------

/// DNS subcommand group.
#[derive(Debug, Args)]
pub struct DnsCommand {
    #[command(subcommand)]
    pub action: DnsAction,
}

/// Individual DNS actions.
#[derive(Debug, Subcommand)]
pub enum DnsAction {
    /// Set up DNS for wide-area discovery.
    Setup(DnsSetupArgs),
}

#[derive(Debug, Args)]
pub struct DnsSetupArgs {
    /// Apply the configuration (otherwise dry-run).
    #[arg(long)]
    pub apply: bool,
}

// -- Node subcommands -------------------------------------------------------

/// Node subcommand group.
#[derive(Debug, Args)]
pub struct NodeCommand {
    #[command(subcommand)]
    pub action: NodeAction,
}

/// Individual node actions.
#[derive(Debug, Subcommand)]
pub enum NodeAction {
    /// Run a headless node host.
    Run(NodeRunArgs),
    /// Install node as a system service.
    Install(NodeInstallArgs),
    /// Show node service status.
    Status,
    /// Stop the node service.
    Stop,
    /// Restart the node service.
    Restart,
    /// Uninstall the node service.
    Uninstall,
}

#[derive(Debug, Args)]
pub struct NodeRunArgs {
    /// Host to bind to.
    #[arg(long)]
    pub host: Option<String>,
    /// Port to listen on.
    #[arg(long)]
    pub port: Option<u16>,
}

#[derive(Debug, Args)]
pub struct NodeInstallArgs {
    /// Host to bind to.
    #[arg(long)]
    pub host: Option<String>,
    /// Port to listen on.
    #[arg(long)]
    pub port: Option<u16>,
}

// -- Pairing subcommands ----------------------------------------------------

/// Pairing subcommand group.
#[derive(Debug, Args)]
pub struct PairingCommand {
    #[command(subcommand)]
    pub action: PairingAction,
}

/// Individual pairing actions.
#[derive(Debug, Subcommand)]
pub enum PairingAction {
    /// List pending pairing requests.
    List(PairingListArgs),
    /// Approve a pairing request.
    Approve(PairingApproveArgs),
}

#[derive(Debug, Args)]
pub struct PairingListArgs {
    /// Channel to list requests for.
    pub channel: String,
}

#[derive(Debug, Args)]
pub struct PairingApproveArgs {
    /// Channel kind.
    pub channel: String,
    /// Pairing code.
    pub code: String,
    /// Send notification after approval.
    #[arg(long)]
    pub notify: bool,
}

// -- System subcommands -----------------------------------------------------

/// System subcommand group.
#[derive(Debug, Args)]
pub struct SystemCommand {
    #[command(subcommand)]
    pub action: SystemAction,
}

/// Individual system actions.
#[derive(Debug, Subcommand)]
pub enum SystemAction {
    /// Emit a system event.
    Event(SystemEventArgs),
    /// Show last heartbeat.
    HeartbeatLast,
    /// Enable heartbeat.
    HeartbeatEnable,
    /// Disable heartbeat.
    HeartbeatDisable,
    /// Show presence information.
    Presence,
}

#[derive(Debug, Args)]
pub struct SystemEventArgs {
    /// Event text.
    pub text: String,
    /// Delivery mode (next-heartbeat, immediate).
    #[arg(long)]
    pub mode: Option<String>,
}

// -- TUI command ------------------------------------------------------------

/// Arguments for the `tui` command.
#[derive(Debug, Args)]
pub struct TuiArgs {
    /// Gateway URL.
    #[arg(long)]
    pub url: Option<String>,
    /// Auth token.
    #[arg(long)]
    pub token: Option<String>,
    /// Session ID.
    #[arg(long)]
    pub session: Option<String>,
}

// -- Update subcommands -----------------------------------------------------

/// Update subcommand group.
#[derive(Debug, Args)]
pub struct UpdateCommand {
    #[command(subcommand)]
    pub action: UpdateAction,
}

/// Individual update actions.
#[derive(Debug, Subcommand)]
pub enum UpdateAction {
    /// Run the updater.
    Run(UpdateRunArgs),
    /// Show update status.
    Status,
    /// Interactive update wizard.
    Wizard,
}

#[derive(Debug, Args)]
pub struct UpdateRunArgs {
    /// Update channel (stable, beta, nightly).
    #[arg(long)]
    pub channel: Option<String>,
    /// Do not restart services after update.
    #[arg(long)]
    pub no_restart: bool,
}

// -- Voicecall subcommands --------------------------------------------------

/// Voicecall subcommand group.
#[derive(Debug, Args)]
pub struct VoicecallCommand {
    #[command(subcommand)]
    pub action: VoicecallAction,
}

/// Individual voicecall actions.
#[derive(Debug, Subcommand)]
pub enum VoicecallAction {
    /// Show call status.
    Status(VoicecallStatusArgs),
    /// Initiate a voice call.
    Call(VoicecallCallArgs),
    /// Continue an active call with a message.
    Continue(VoicecallContinueArgs),
    /// End an active call.
    End(VoicecallEndArgs),
    /// Expose the voice server.
    Expose(VoicecallExposeArgs),
    /// Unexpose the voice server.
    Unexpose,
}

#[derive(Debug, Args)]
pub struct VoicecallStatusArgs {
    /// Call ID.
    pub call_id: String,
}

#[derive(Debug, Args)]
pub struct VoicecallCallArgs {
    /// Recipient identifier.
    pub to: String,
    /// Initial message.
    pub message: String,
}

#[derive(Debug, Args)]
pub struct VoicecallContinueArgs {
    /// Call ID.
    pub call_id: String,
    /// Follow-up message.
    pub message: String,
}

#[derive(Debug, Args)]
pub struct VoicecallEndArgs {
    /// Call ID.
    pub call_id: String,
}

#[derive(Debug, Args)]
pub struct VoicecallExposeArgs {
    /// Expose mode (tailscale, cloudflare, ngrok).
    #[arg(long, default_value = "tailscale")]
    pub mode: String,
}

// -- Webhooks subcommands ---------------------------------------------------

/// Webhooks subcommand group.
#[derive(Debug, Args)]
pub struct WebhooksCommand {
    #[command(subcommand)]
    pub action: WebhooksAction,
}

/// Individual webhooks actions.
#[derive(Debug, Subcommand)]
pub enum WebhooksAction {
    /// List configured webhooks.
    List,
    /// Test a webhook endpoint.
    Test(WebhooksTestArgs),
    /// Set up Gmail Pub/Sub integration.
    GmailSetup(GmailSetupArgs),
    /// Run the Gmail Pub/Sub listener.
    GmailRun,
}

#[derive(Debug, Args)]
pub struct WebhooksTestArgs {
    /// Webhook URL to test.
    #[arg(long)]
    pub url: Option<String>,
}

#[derive(Debug, Args)]
pub struct GmailSetupArgs {
    /// Gmail account.
    pub account: String,
}

// -- Skills subcommands -----------------------------------------------------

/// Skills subcommand group.
#[derive(Debug, Args)]
pub struct SkillsCommand {
    #[command(subcommand)]
    pub action: SkillsAction,
}

/// Individual skills actions.
#[derive(Debug, Subcommand)]
pub enum SkillsAction {
    /// List available skills.
    List(SkillsListArgs),
    /// Show skill details.
    Info(SkillsInfoArgs),
    /// Verify skill system health.
    Check(SkillsCheckArgs),
}

#[derive(Debug, Args)]
pub struct SkillsListArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Show only eligible skills.
    #[arg(long)]
    pub eligible: bool,
    /// Show verbose details.
    #[arg(long, short)]
    pub verbose: bool,
}

#[derive(Debug, Args)]
pub struct SkillsInfoArgs {
    /// Skill name.
    pub name: String,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct SkillsCheckArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

// -- Plugins subcommands ----------------------------------------------------

/// Plugins subcommand group.
#[derive(Debug, Args)]
pub struct PluginsCommand {
    #[command(subcommand)]
    pub action: PluginsAction,
}

/// Individual plugins actions.
#[derive(Debug, Subcommand)]
pub enum PluginsAction {
    /// List loaded plugins.
    List(PluginsListArgs),
    /// Show plugin details.
    Info(PluginsInfoArgs),
    /// Install a plugin.
    Install(PluginsInstallArgs),
    /// Enable a plugin.
    Enable(PluginsEnableArgs),
    /// Disable a plugin.
    Disable(PluginsDisableArgs),
    /// Report plugin load errors.
    Doctor(PluginsDoctorArgs),
}

#[derive(Debug, Args)]
pub struct PluginsListArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct PluginsInfoArgs {
    /// Plugin ID.
    pub id: String,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct PluginsInstallArgs {
    /// Plugin path, tarball, or npm spec.
    pub spec: String,
    /// Link local path instead of copying (dev mode).
    #[arg(long, short)]
    pub link: bool,
}

#[derive(Debug, Args)]
pub struct PluginsEnableArgs {
    /// Plugin ID.
    pub id: String,
}

#[derive(Debug, Args)]
pub struct PluginsDisableArgs {
    /// Plugin ID.
    pub id: String,
}

#[derive(Debug, Args)]
pub struct PluginsDoctorArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

// -- Security subcommands ---------------------------------------------------

/// Arguments for the `security` command (currently: audit).
#[derive(Debug, Args)]
pub struct SecurityArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Include deep filesystem permission checks.
    #[arg(long)]
    pub deep: bool,
    /// Auto-fix safe defaults (chmod state/config).
    #[arg(long)]
    pub fix: bool,
}

// -- Hooks subcommands ------------------------------------------------------

/// Hooks subcommand group.
#[derive(Debug, Args)]
pub struct HooksCommand {
    #[command(subcommand)]
    pub action: HooksAction,
}

/// Individual hooks actions.
#[derive(Debug, Subcommand)]
pub enum HooksAction {
    /// List configured hooks.
    List(HooksListArgs),
    /// Show hook details.
    Info(HooksInfoArgs),
    /// Verify hook system health.
    Check(HooksCheckArgs),
    /// Enable a hook.
    Enable(HooksEnableArgs),
    /// Disable a hook.
    Disable(HooksDisableArgs),
    /// Install a hook pack.
    Install(HooksInstallArgs),
    /// Update hook packs.
    Update(HooksUpdateArgs),
}

#[derive(Debug, Args)]
pub struct HooksListArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
    /// Show only eligible hooks.
    #[arg(long)]
    pub eligible: bool,
    /// Show verbose details.
    #[arg(long, short)]
    pub verbose: bool,
}

#[derive(Debug, Args)]
pub struct HooksInfoArgs {
    /// Hook name.
    pub name: String,
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct HooksCheckArgs {
    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct HooksEnableArgs {
    /// Hook name.
    pub name: String,
}

#[derive(Debug, Args)]
pub struct HooksDisableArgs {
    /// Hook name.
    pub name: String,
}

#[derive(Debug, Args)]
pub struct HooksInstallArgs {
    /// Path or npm spec.
    pub spec: String,
    /// Link local path instead of copying (dev mode).
    #[arg(long, short)]
    pub link: bool,
}

#[derive(Debug, Args)]
pub struct HooksUpdateArgs {
    /// Specific hook-pack ID (optional if --all).
    pub id: Option<String>,
    /// Update all hook packs.
    #[arg(long)]
    pub all: bool,
    /// Dry run (preview without executing).
    #[arg(long)]
    pub dry_run: bool,
}

// -- Browser subcommands ----------------------------------------------------

/// Browser subcommand group.
#[derive(Debug, Args)]
pub struct BrowserCommand {
    #[command(subcommand)]
    pub action: BrowserAction,
}

/// Individual browser actions.
#[derive(Debug, Subcommand)]
pub enum BrowserAction {
    // -- Manage --
    /// Query browser control status.
    Status(BrowserStatusArgs),
    /// Start a managed browser instance.
    Start(BrowserStartArgs),
    /// Stop a managed browser instance.
    Stop(BrowserStopArgs),
    /// List open tabs.
    Tabs(BrowserTabsArgs),
    /// Open a URL in the browser.
    Open(BrowserOpenArgs),
    /// Focus a tab by target ID.
    Focus(BrowserFocusArgs),
    /// Close a tab (or active tab).
    Close(BrowserCloseArgs),
    /// Take a screenshot.
    Screenshot(BrowserScreenshotArgs),
    /// List browser profiles.
    Profiles(BrowserProfilesArgs),
    /// Create a new browser profile.
    CreateProfile(BrowserCreateProfileArgs),
    /// Delete a browser profile.
    DeleteProfile(BrowserDeleteProfileArgs),
    /// Reset browser profile data.
    ResetProfile(BrowserResetProfileArgs),
    // -- Inspect --
    /// Take an accessibility snapshot.
    Snapshot(BrowserSnapshotArgs),
    /// Show browser console output.
    Console(BrowserConsoleArgs),
    /// Show page errors.
    Errors(BrowserErrorsArgs),
    /// Show network requests.
    Requests(BrowserRequestsArgs),
    /// Read a network response body.
    #[command(name = "responsebody")]
    ResponseBody(BrowserResponseBodyArgs),
    /// Generate a PDF of the page.
    Pdf(BrowserPdfArgs),
    // -- Actions --
    /// Navigate to a URL.
    Navigate(BrowserNavigateArgs),
    /// Resize the viewport.
    Resize(BrowserResizeArgs),
    /// Click an element by ref.
    Click(BrowserClickArgs),
    /// Type text into an element by ref.
    #[command(name = "type")]
    TypeText(BrowserTypeArgs),
    /// Press a keyboard key.
    Press(BrowserPressArgs),
    /// Hover over an element by ref.
    Hover(BrowserHoverArgs),
    /// Scroll an element into view.
    #[command(name = "scrollintoview")]
    ScrollIntoView(BrowserScrollIntoViewArgs),
    /// Drag from one element to another.
    Drag(BrowserDragArgs),
    /// Select option(s) in a select element.
    Select(BrowserSelectArgs),
    /// Download a file by clicking a ref.
    Download(BrowserDownloadArgs),
    /// Wait for a download to complete.
    #[command(name = "waitfordownload")]
    WaitForDownload(BrowserWaitForDownloadArgs),
    /// Upload file(s) via file chooser.
    Upload(BrowserUploadArgs),
    /// Fill form fields (batch).
    Fill(BrowserFillArgs),
    /// Handle a browser dialog.
    Dialog(BrowserDialogArgs),
    /// Wait for a condition.
    Wait(BrowserWaitArgs),
    /// Evaluate JavaScript in page context.
    Evaluate(BrowserEvaluateArgs),
    /// Highlight an element by ref.
    Highlight(BrowserHighlightArgs),
    /// Trace recording.
    Trace(BrowserTraceCommand),
    // -- State --
    /// Manage cookies.
    Cookies(BrowserCookiesCommand),
    /// Manage localStorage / sessionStorage.
    Storage(BrowserStorageCommand),
    /// Set browser environment overrides.
    Set(BrowserSetCommand),
}

// -- Manage args --

#[derive(Debug, Args)]
pub struct BrowserStatusArgs {
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserStartArgs {
    #[arg(long)]
    pub browser_profile: Option<String>,
    #[arg(long)]
    pub headless: bool,
}

#[derive(Debug, Args)]
pub struct BrowserStopArgs {
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserTabsArgs {
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserOpenArgs {
    pub url: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserFocusArgs {
    pub target_id: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserCloseArgs {
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserScreenshotArgs {
    pub file: Option<String>,
    #[arg(long)]
    pub full_page: bool,
    #[arg(long, name = "ref")]
    pub ref_id: Option<String>,
    #[arg(long)]
    pub element: Option<String>,
    #[arg(long, name = "type")]
    pub format: Option<String>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserProfilesArgs {
    #[arg(long)]
    pub json: bool,
}

#[derive(Debug, Args)]
pub struct BrowserCreateProfileArgs {
    pub name: String,
    #[arg(long)]
    pub color: Option<String>,
    #[arg(long)]
    pub cdp_url: Option<String>,
    #[arg(long)]
    pub driver: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserDeleteProfileArgs {
    #[arg(long)]
    pub name: String,
}

// -- Inspect args --

#[derive(Debug, Args)]
pub struct BrowserSnapshotArgs {
    #[arg(long, default_value = "ai")]
    pub format: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub limit: Option<u32>,
    #[arg(long)]
    pub interactive: bool,
    #[arg(long)]
    pub compact: bool,
    #[arg(long)]
    pub depth: Option<u32>,
    #[arg(long)]
    pub selector: Option<String>,
    #[arg(long)]
    pub frame: Option<String>,
    #[arg(long)]
    pub efficient: bool,
    #[arg(long)]
    pub labels: bool,
    #[arg(long)]
    pub out: Option<String>,
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserConsoleArgs {
    #[arg(long)]
    pub level: Option<String>,
    #[arg(long)]
    pub clear: bool,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserErrorsArgs {
    #[arg(long)]
    pub clear: bool,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserRequestsArgs {
    #[arg(long)]
    pub filter: Option<String>,
    #[arg(long)]
    pub clear: bool,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserResponseBodyArgs {
    pub pattern: String,
    #[arg(long, default_value = "10000")]
    pub max_chars: u32,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserPdfArgs {
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

// -- Action args --

#[derive(Debug, Args)]
pub struct BrowserNavigateArgs {
    pub url: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserResizeArgs {
    pub width: u32,
    pub height: u32,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserClickArgs {
    pub ref_id: String,
    #[arg(long)]
    pub double: bool,
    #[arg(long, default_value = "left")]
    pub button: String,
    #[arg(long)]
    pub modifiers: Option<String>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserTypeArgs {
    pub ref_id: String,
    pub text: String,
    #[arg(long)]
    pub submit: bool,
    #[arg(long)]
    pub slowly: bool,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserPressArgs {
    pub key: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserHoverArgs {
    pub ref_id: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserScrollIntoViewArgs {
    pub ref_id: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserDragArgs {
    pub start_ref: String,
    pub end_ref: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSelectArgs {
    pub ref_id: String,
    pub values: Vec<String>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserDownloadArgs {
    pub ref_id: String,
    pub save_path: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserWaitForDownloadArgs {
    pub save_path: String,
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserUploadArgs {
    pub paths: Vec<String>,
    #[arg(long, name = "ref")]
    pub ref_id: Option<String>,
    #[arg(long)]
    pub input_ref: Option<String>,
    #[arg(long)]
    pub element: Option<String>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserFillArgs {
    #[arg(long)]
    pub fields: Option<String>,
    #[arg(long)]
    pub fields_file: Option<String>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserDialogArgs {
    #[arg(long)]
    pub accept: bool,
    #[arg(long)]
    pub dismiss: bool,
    #[arg(long)]
    pub prompt: Option<String>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserWaitArgs {
    pub selector: Option<String>,
    #[arg(long)]
    pub url: Option<String>,
    #[arg(long)]
    pub load: Option<String>,
    #[arg(long, name = "fn")]
    pub js_fn: Option<String>,
    #[arg(long)]
    pub text: Option<String>,
    #[arg(long)]
    pub text_gone: Option<String>,
    #[arg(long)]
    pub time: Option<u32>,
    #[arg(long)]
    pub timeout_ms: Option<u32>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserEvaluateArgs {
    #[arg(long, name = "fn")]
    pub js_fn: String,
    #[arg(long, name = "ref")]
    pub ref_id: Option<String>,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserHighlightArgs {
    pub ref_id: String,
    #[arg(long)]
    pub target_id: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

// -- Trace subcommand --

#[derive(Debug, Args)]
pub struct BrowserTraceCommand {
    #[command(subcommand)]
    pub action: BrowserTraceAction,
}

#[derive(Debug, Subcommand)]
pub enum BrowserTraceAction {
    /// Start recording a trace.
    Start(BrowserTraceStartArgs),
    /// Stop recording and save the trace.
    Stop(BrowserTraceStopArgs),
}

#[derive(Debug, Args)]
pub struct BrowserTraceStartArgs {
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserTraceStopArgs {
    #[arg(long)]
    pub browser_profile: Option<String>,
}

// -- Cookies subcommand --

#[derive(Debug, Args)]
pub struct BrowserCookiesCommand {
    #[command(subcommand)]
    pub action: Option<BrowserCookiesAction>,
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Subcommand)]
pub enum BrowserCookiesAction {
    /// Set a cookie.
    Set(BrowserCookiesSetArgs),
    /// Clear all cookies.
    Clear(BrowserCookiesClearArgs),
}

#[derive(Debug, Args)]
pub struct BrowserCookiesSetArgs {
    pub name: String,
    pub value: String,
    #[arg(long)]
    pub url: Option<String>,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserCookiesClearArgs {
    #[arg(long)]
    pub browser_profile: Option<String>,
}

// -- Storage subcommand --

#[derive(Debug, Args)]
pub struct BrowserStorageCommand {
    #[command(subcommand)]
    pub action: BrowserStorageAction,
}

#[derive(Debug, Subcommand)]
pub enum BrowserStorageAction {
    /// Get storage contents.
    Get(BrowserStorageGetArgs),
    /// Set a storage key.
    Set(BrowserStorageSetArgs),
    /// Clear storage.
    Clear(BrowserStorageClearArgs),
}

#[derive(Debug, Args)]
pub struct BrowserStorageGetArgs {
    /// local or session.
    pub kind: String,
    #[arg(long)]
    pub json: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserStorageSetArgs {
    /// local or session.
    pub kind: String,
    pub key: String,
    pub value: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserStorageClearArgs {
    /// local or session.
    pub kind: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

// -- Set subcommand --

#[derive(Debug, Args)]
pub struct BrowserSetCommand {
    #[command(subcommand)]
    pub action: BrowserSetAction,
}

#[derive(Debug, Subcommand)]
pub enum BrowserSetAction {
    /// Set offline mode.
    Offline(BrowserSetOfflineArgs),
    /// Set custom HTTP headers.
    Headers(BrowserSetHeadersArgs),
    /// Set HTTP basic auth credentials.
    Credentials(BrowserSetCredentialsArgs),
    /// Set geolocation override.
    Geo(BrowserSetGeoArgs),
    /// Set media color scheme preference.
    Media(BrowserSetMediaArgs),
    /// Set timezone override.
    Timezone(BrowserSetTimezoneArgs),
    /// Set locale override.
    Locale(BrowserSetLocaleArgs),
    /// Set device preset.
    Device(BrowserSetDeviceArgs),
}

#[derive(Debug, Args)]
pub struct BrowserSetOfflineArgs {
    /// on or off.
    pub state: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSetHeadersArgs {
    #[arg(long)]
    pub json: Option<String>,
    #[arg(long)]
    pub clear: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSetCredentialsArgs {
    pub username: Option<String>,
    pub password: Option<String>,
    #[arg(long)]
    pub clear: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSetGeoArgs {
    pub latitude: Option<f64>,
    pub longitude: Option<f64>,
    #[arg(long)]
    pub origin: Option<String>,
    #[arg(long)]
    pub clear: bool,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSetMediaArgs {
    /// dark, light, no-preference, none.
    pub scheme: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSetTimezoneArgs {
    pub timezone: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSetLocaleArgs {
    pub locale: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserSetDeviceArgs {
    pub device: String,
    #[arg(long)]
    pub browser_profile: Option<String>,
}

#[derive(Debug, Args)]
pub struct BrowserResetProfileArgs {
    /// Browser profile name.
    #[arg(long)]
    pub browser_profile: Option<String>,
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

/// Route the parsed subcommand to its handler.
#[allow(clippy::too_many_lines)]
pub async fn dispatch(cmd: Commands, json: bool, verbose: bool) -> Result<()> {
    match cmd {
        // -- Tier 1: Health, Status, Sessions --------------------------------
        Commands::Health(args) => {
            let ha = oa_cmd_health::HealthArgs {
                json: json || args.json,
                verbose: verbose || args.verbose,
                timeout_ms: args.timeout_ms,
            };
            oa_cmd_health::execute(&ha).await
        }
        Commands::Status(args) => {
            oa_cmd_status::status_command(
                json,
                args.deep,
                args.usage,
                args.timeout_ms,
                verbose,
                args.all,
            )
            .await
        }
        Commands::Sessions(args) => {
            let sa = oa_cmd_sessions::SessionsArgs {
                json: json || args.json,
                store: args.store,
                active: args.active,
            };
            oa_cmd_sessions::execute(&sa).await
        }
        Commands::StatusAll(args) => {
            oa_cmd_status::status_all::status_all_command(args.timeout_ms).await
        }
        Commands::GatewayStatus(args) => {
            oa_cmd_status::gateway_status::gateway_status_command(
                args.url.as_deref(),
                args.token.as_deref(),
                args.password.as_deref(),
                args.timeout.as_deref(),
                json || args.json,
            )
            .await
        }

        // -- Tier 2: Channels, Models, Agents, Sandbox ----------------------
        Commands::Channels(cmd) => dispatch_channels(cmd, json).await,
        Commands::Models(cmd) => dispatch_models(cmd, json).await,
        Commands::Agents(cmd) => dispatch_agents(cmd, json).await,
        Commands::Sandbox(cmd) => dispatch_sandbox(cmd, json).await,
        Commands::Coder(cmd) => dispatch_coder(cmd).await,

        // -- Tier 3: Auth, Configure, Onboard --------------------------------
        Commands::Auth(_) => oa_cmd_auth::execute().await,
        Commands::Configure(args) => {
            if let Some(ref sections) = args.sections {
                let parsed: Vec<oa_cmd_configure::shared::WizardSection> = sections
                    .iter()
                    .filter_map(|s| match s.as_str() {
                        "workspace" => Some(oa_cmd_configure::shared::WizardSection::Workspace),
                        "model" => Some(oa_cmd_configure::shared::WizardSection::Model),
                        "web" => Some(oa_cmd_configure::shared::WizardSection::Web),
                        "gateway" => Some(oa_cmd_configure::shared::WizardSection::Gateway),
                        "daemon" => Some(oa_cmd_configure::shared::WizardSection::Daemon),
                        "channels" => Some(oa_cmd_configure::shared::WizardSection::Channels),
                        "skills" => Some(oa_cmd_configure::shared::WizardSection::Skills),
                        "health" => Some(oa_cmd_configure::shared::WizardSection::Health),
                        _ => None,
                    })
                    .collect();
                if parsed.is_empty() {
                    anyhow::bail!(
                        "No valid sections provided. Valid: workspace, model, web, gateway, daemon, channels, skills, health"
                    );
                }
                oa_cmd_configure::execute_with_sections(parsed).await
            } else {
                oa_cmd_configure::execute().await
            }
        }
        Commands::Onboard(args) => {
            let opts = oa_cmd_onboard::types::OnboardOptions {
                mode: args.mode,
                auth_choice: args.auth_choice,
                workspace: args.workspace,
                non_interactive: Some(args.non_interactive),
                accept_risk: Some(args.accept_risk),
                reset: Some(args.reset),
                anthropic_api_key: args.anthropic_api_key,
                openai_api_key: args.openai_api_key,
                openrouter_api_key: args.openrouter_api_key,
                gateway_port: args.gateway_port,
                gateway_bind: args.gateway_bind,
                gateway_auth: args.gateway_auth,
                gateway_token: args.gateway_token,
                gateway_password: args.gateway_password,
                install_daemon: Some(args.install_daemon),
                daemon_runtime: args.daemon_runtime,
                skip_channels: Some(args.skip_channels),
                skip_skills: Some(args.skip_skills),
                skip_health: Some(args.skip_health),
                json: Some(json || args.json),
                ..Default::default()
            };
            oa_cmd_onboard::execute(opts).await
        }

        // -- Tier 4: Doctor, Agent, Supporting -------------------------------
        Commands::Doctor(args) => {
            let opts = oa_cmd_doctor::DoctorOptions {
                yes: Some(args.yes),
                non_interactive: Some(args.non_interactive),
                deep: Some(args.deep),
                repair: Some(args.repair),
                force: Some(args.force),
                ..Default::default()
            };
            oa_cmd_doctor::execute(opts).await
        }
        Commands::Agent(args) => {
            let opts = oa_cmd_agent::types::AgentCommandOpts {
                message: args.message,
                agent_id: args.agent_id,
                to: args.to,
                session_id: args.session_id,
                session_key: args.session_key,
                thinking: args.thinking,
                json: Some(json || args.json),
                timeout: args.timeout,
                verbose: args.verbose,
                ..Default::default()
            };
            let _result = oa_cmd_agent::agent_command::agent_command(&opts).await?;
            Ok(())
        }

        // -- Supporting commands ---------------------------------------------
        Commands::Dashboard(args) => {
            let opts = oa_cmd_supporting::dashboard::DashboardOptions {
                no_open: args.no_open,
            };
            oa_cmd_supporting::dashboard::dashboard_command(opts).await
        }
        Commands::Docs(args) => oa_cmd_supporting::docs::docs_search_command(&args.query).await,
        Commands::Reset(args) => {
            let scope = args
                .scope
                .as_deref()
                .and_then(oa_cmd_supporting::reset::ResetScope::from_str);
            let opts = oa_cmd_supporting::reset::ResetOptions {
                scope,
                dry_run: args.dry_run,
                yes: args.yes,
                non_interactive: args.non_interactive,
            };
            oa_cmd_supporting::reset::reset_command(&opts).await
        }
        Commands::Setup(args) => {
            let opts = oa_cmd_supporting::setup::SetupOptions {
                workspace: args.workspace,
            };
            oa_cmd_supporting::setup::setup_command(opts).await
        }
        Commands::Uninstall(args) => {
            let opts = oa_cmd_supporting::uninstall::UninstallOptions {
                service: args.service,
                state: args.state,
                workspace: args.workspace,
                app: args.app,
                all: args.all,
                yes: args.yes,
                non_interactive: args.non_interactive,
                dry_run: args.dry_run,
            };
            oa_cmd_supporting::uninstall::uninstall_command(&opts).await
        }
        Commands::Message(args) => {
            let opts = oa_cmd_supporting::message::MessageCommandOptions {
                action: args.action,
                json: json || args.json,
                dry_run: args.dry_run,
                params: HashMap::new(),
            };
            oa_cmd_supporting::message::message_command(&opts).await
        }

        // -- Tier 5: Gateway, Daemon, Logs, Memory, Cron, Config ---------------
        Commands::Gateway(cmd) => dispatch_gateway(cmd, json).await,
        Commands::Daemon(cmd) => dispatch_daemon(cmd, json).await,
        Commands::Logs(cmd) => dispatch_logs(cmd, json).await,
        Commands::Memory(cmd) => dispatch_memory(cmd, json).await,
        Commands::Cron(cmd) => dispatch_cron(cmd, json).await,
        Commands::Config(cmd) => dispatch_config(cmd, json).await,

        // -- Tier 6: Batch-3 legacy commands (newly adapted) ------------------
        Commands::Acp(cmd) => dispatch_acp(cmd, json).await,
        Commands::Approvals(cmd) => dispatch_approvals(cmd, json).await,
        Commands::Devices(cmd) => dispatch_devices(cmd, json).await,
        Commands::Directory(cmd) => dispatch_directory(cmd, json).await,
        Commands::Dns(cmd) => dispatch_dns(cmd).await,
        Commands::Node(cmd) => dispatch_node(cmd, json).await,
        Commands::Pairing(cmd) => dispatch_pairing(cmd, json).await,
        Commands::System(cmd) => dispatch_system(cmd, json).await,
        Commands::Tui(args) => oa_cmd_tui::launch::tui_launch_command(
            args.url.as_deref(),
            args.token.as_deref(),
            args.session.as_deref(),
        ),
        Commands::Update(cmd) => dispatch_update(cmd, json).await,
        Commands::Voicecall(cmd) => dispatch_voicecall(cmd, json).await,
        Commands::Webhooks(cmd) => dispatch_webhooks(cmd, json).await,

        // -- Tier 7: Skills, Plugins, Security, Hooks, Browser -----------------
        Commands::Skills(cmd) => dispatch_skills(cmd, json).await,
        Commands::Plugins(cmd) => dispatch_plugins(cmd, json).await,
        Commands::Security(args) => {
            let sa = oa_cmd_security::SecurityAuditArgs {
                json: json || args.json,
                deep: args.deep,
                fix: args.fix,
            };
            oa_cmd_security::security_audit_command(&sa).await
        }
        Commands::Hooks(cmd) => dispatch_hooks(cmd, json).await,
        Commands::Browser(cmd) => dispatch_browser(cmd, json).await,

        // -- Shell completions -----------------------------------------------
        Commands::Completion(args) => {
            let mut cmd = <crate::Cli as clap::CommandFactory>::command();
            clap_complete::generate(args.shell, &mut cmd, "openacosmi", &mut std::io::stdout());
            Ok(())
        }
    }
}

// ---------------------------------------------------------------------------
// Sub-dispatchers for nested subcommands
// ---------------------------------------------------------------------------

/// Dispatch channels subcommands.
async fn dispatch_channels(cmd: ChannelsCommand, json: bool) -> Result<()> {
    match cmd.action {
        ChannelsAction::List(args) => {
            let opts = oa_cmd_channels::list::ChannelsListOptions {
                json: json || args.json,
                usage: args.usage,
            };
            oa_cmd_channels::list::channels_list_command(&opts).await
        }
        ChannelsAction::Add(args) => {
            let opts = oa_cmd_channels::add::ChannelsAddOptions {
                channel: Some(args.channel),
                account: args.account,
                ..Default::default()
            };
            oa_cmd_channels::add::channels_add_command(&opts).await
        }
        ChannelsAction::Remove(args) => {
            let opts = oa_cmd_channels::remove::ChannelsRemoveOptions {
                channel: Some(args.channel),
                account: args.account,
                delete: args.delete,
            };
            oa_cmd_channels::remove::channels_remove_command(&opts).await
        }
        ChannelsAction::Resolve(args) => {
            let opts = oa_cmd_channels::resolve::ChannelsResolveOptions {
                channel: args.channel,
                kind: args.kind,
                json: json || args.json,
                entries: args.entries,
                ..Default::default()
            };
            oa_cmd_channels::resolve::channels_resolve_command(&opts).await
        }
        ChannelsAction::Capabilities(args) => {
            let opts = oa_cmd_channels::capabilities::ChannelsCapabilitiesOptions {
                channel: args.channel,
                account: args.account,
                timeout: args.timeout,
                json: json || args.json,
                ..Default::default()
            };
            oa_cmd_channels::capabilities::channels_capabilities_command(&opts).await
        }
        ChannelsAction::Logs(args) => {
            let opts = oa_cmd_channels::logs::ChannelsLogsOptions {
                channel: args.channel,
                lines: args.lines,
                json: json || args.json,
            };
            oa_cmd_channels::logs::channels_logs_command(&opts).await
        }
        ChannelsAction::Status(args) => {
            let opts = oa_cmd_channels::status::ChannelsStatusOptions {
                json: json || args.json,
                probe: args.probe,
                timeout: args.timeout,
            };
            oa_cmd_channels::status::channels_status_command(&opts).await
        }
        ChannelsAction::Login(args) => {
            let opts = oa_cmd_channels::login::ChannelsLoginOptions {
                channel: args.channel,
                account: args.account,
                json: json || args.json,
            };
            oa_cmd_channels::login::channels_login_command(&opts).await
        }
        ChannelsAction::Logout(args) => {
            let opts = oa_cmd_channels::logout::ChannelsLogoutOptions {
                channel: args.channel,
                account: args.account,
                json: json || args.json,
            };
            oa_cmd_channels::logout::channels_logout_command(&opts).await
        }
    }
}

/// Dispatch models subcommands.
async fn dispatch_models(cmd: ModelsCommand, json: bool) -> Result<()> {
    match cmd.action {
        ModelsAction::List(args) => {
            let cfg = oa_config::io::load_config().unwrap_or_default();
            let entries = oa_cmd_models::list_configured::resolve_configured_entries(&cfg);
            let output = if json || args.json {
                serde_json::to_string_pretty(&entries)?
            } else {
                entries
                    .iter()
                    .map(|e| {
                        let tags: Vec<&String> = e.tags.iter().collect();
                        format!(
                            "{} ({}/{}) [{}]",
                            e.key,
                            e.ref_provider,
                            e.ref_model,
                            tags.iter()
                                .map(|s| s.as_str())
                                .collect::<Vec<_>>()
                                .join(", ")
                        )
                    })
                    .collect::<Vec<_>>()
                    .join("\n")
            };
            println!("{output}");
            Ok(())
        }
        ModelsAction::Set(args) => {
            let msg = oa_cmd_models::set::models_set_command(&args.model).await?;
            println!("{msg}");
            Ok(())
        }
        ModelsAction::SetImage(args) => {
            let msg = oa_cmd_models::set_image::models_set_image_command(&args.model).await?;
            println!("{msg}");
            Ok(())
        }
        ModelsAction::Aliases(cmd) => dispatch_models_aliases(cmd, json).await,
        ModelsAction::Fallbacks(cmd) => dispatch_models_fallbacks(cmd, json).await,
        ModelsAction::ImageFallbacks(cmd) => dispatch_models_image_fallbacks(cmd, json).await,
    }
}

/// Dispatch model alias subcommands.
async fn dispatch_models_aliases(cmd: ModelsAliasesCommand, json: bool) -> Result<()> {
    match cmd.action {
        ModelsAliasesAction::List(args) => {
            let msg =
                oa_cmd_models::aliases::models_aliases_list_command(json || args.json, args.plain)?;
            println!("{msg}");
            Ok(())
        }
        ModelsAliasesAction::Add(args) => {
            let msg = oa_cmd_models::aliases::models_aliases_add_command(&args.alias, &args.model)
                .await?;
            println!("{msg}");
            Ok(())
        }
        ModelsAliasesAction::Remove(args) => {
            let msg = oa_cmd_models::aliases::models_aliases_remove_command(&args.alias).await?;
            println!("{msg}");
            Ok(())
        }
    }
}

/// Dispatch model fallback subcommands.
async fn dispatch_models_fallbacks(cmd: ModelsFallbacksCommand, json: bool) -> Result<()> {
    match cmd.action {
        ModelsFallbacksAction::List(args) => {
            let msg = oa_cmd_models::fallbacks::models_fallbacks_list_command(
                json || args.json,
                args.plain,
            )?;
            println!("{msg}");
            Ok(())
        }
        ModelsFallbacksAction::Add(args) => {
            let msg = oa_cmd_models::fallbacks::models_fallbacks_add_command(&args.model).await?;
            println!("{msg}");
            Ok(())
        }
        ModelsFallbacksAction::Remove(args) => {
            let msg =
                oa_cmd_models::fallbacks::models_fallbacks_remove_command(&args.model).await?;
            println!("{msg}");
            Ok(())
        }
        ModelsFallbacksAction::Clear => {
            let msg = oa_cmd_models::fallbacks::models_fallbacks_clear_command().await?;
            println!("{msg}");
            Ok(())
        }
    }
}

/// Dispatch image fallback subcommands.
async fn dispatch_models_image_fallbacks(
    cmd: ModelsImageFallbacksCommand,
    json: bool,
) -> Result<()> {
    match cmd.action {
        ModelsImageFallbacksAction::List(args) => {
            let msg = oa_cmd_models::image_fallbacks::models_image_fallbacks_list_command(
                json || args.json,
                args.plain,
            )?;
            println!("{msg}");
            Ok(())
        }
        ModelsImageFallbacksAction::Add(args) => {
            let msg =
                oa_cmd_models::image_fallbacks::models_image_fallbacks_add_command(&args.model)
                    .await?;
            println!("{msg}");
            Ok(())
        }
        ModelsImageFallbacksAction::Remove(args) => {
            let msg =
                oa_cmd_models::image_fallbacks::models_image_fallbacks_remove_command(&args.model)
                    .await?;
            println!("{msg}");
            Ok(())
        }
        ModelsImageFallbacksAction::Clear => {
            let msg =
                oa_cmd_models::image_fallbacks::models_image_fallbacks_clear_command().await?;
            println!("{msg}");
            Ok(())
        }
    }
}

/// Dispatch agents subcommands.
async fn dispatch_agents(cmd: AgentsCommand, json: bool) -> Result<()> {
    match cmd.action {
        AgentsAction::List(args) => {
            let cfg = oa_config::io::load_config().unwrap_or_default();
            let output =
                oa_cmd_agents::list::agents_list_command(&cfg, json || args.json, args.bindings)?;
            println!("{output}");
            Ok(())
        }
        AgentsAction::Add(args) => {
            let opts = oa_cmd_agents::add::AgentsAddOptions {
                id: &args.id,
                name: args.name.as_deref(),
                workspace: args.workspace.as_deref(),
                model: args.model.as_deref(),
            };
            let msg = oa_cmd_agents::add::agents_add_command(&opts).await?;
            println!("{msg}");
            Ok(())
        }
        AgentsAction::Delete(args) => {
            let opts = oa_cmd_agents::delete::AgentsDeleteOptions {
                id: &args.id,
                yes: args.yes,
            };
            let msg = oa_cmd_agents::delete::agents_delete_command(&opts).await?;
            println!("{msg}");
            Ok(())
        }
        AgentsAction::SetIdentity(args) => {
            let opts = oa_cmd_agents::set_identity::AgentsSetIdentityOptions {
                id: &args.id,
                name: args.name.as_deref(),
                theme: args.theme.as_deref(),
                emoji: args.emoji.as_deref(),
                avatar: args.avatar.as_deref(),
            };
            let msg = oa_cmd_agents::set_identity::agents_set_identity_command(&opts).await?;
            println!("{msg}");
            Ok(())
        }
    }
}

/// Dispatch sandbox subcommands.
async fn dispatch_sandbox(cmd: SandboxCommand, json: bool) -> Result<()> {
    match cmd.action {
        SandboxAction::List(args) => {
            let opts = oa_cmd_sandbox::list::SandboxListOptions {
                json: json || args.json,
                browser: args.browser,
            };
            oa_cmd_sandbox::list::sandbox_list_command(&opts).await
        }
        SandboxAction::Recreate(args) => {
            let opts = oa_cmd_sandbox::recreate::SandboxRecreateOptions {
                agent: args.agent,
                session: args.session,
                all: args.all,
                browser: args.browser,
                force: args.force,
            };
            oa_cmd_sandbox::recreate::sandbox_recreate_command(&opts).await
        }
        SandboxAction::Explain(args) => {
            let opts = oa_cmd_sandbox::explain::SandboxExplainOptions {
                agent: args.agent,
                session: args.session,
                json: json || args.json,
            };
            oa_cmd_sandbox::explain::sandbox_explain_command(&opts).await
        }
        SandboxAction::Run(args) => {
            let security = match args.security.as_str() {
                "deny" => oa_sandbox::config::SecurityLevel::L0Deny,
                "allowlist" | "sandbox" => oa_sandbox::config::SecurityLevel::L1Allowlist,
                "sandboxed" | "full" => oa_sandbox::config::SecurityLevel::L2Sandboxed,
                other => anyhow::bail!(
                    "invalid security level '{other}': expected deny, allowlist, or sandboxed (legacy: sandbox, full)"
                ),
            };
            let network = args
                .net
                .as_deref()
                .map(|n| match n {
                    "none" => Ok(oa_sandbox::config::NetworkPolicy::None),
                    "restricted" => Ok(oa_sandbox::config::NetworkPolicy::Restricted),
                    "host" => Ok(oa_sandbox::config::NetworkPolicy::Host),
                    other => Err(anyhow::anyhow!(
                        "invalid network policy '{other}': expected none, restricted, or host"
                    )),
                })
                .transpose()?;
            let format = match args.format.as_str() {
                "json" => oa_sandbox::config::OutputFormat::Json,
                "text" => oa_sandbox::config::OutputFormat::Text,
                other => anyhow::bail!("invalid format '{other}': expected json or text"),
            };
            let backend = match args.backend.as_str() {
                "auto" => oa_sandbox::config::BackendPreference::Auto,
                "native" => oa_sandbox::config::BackendPreference::Native,
                "docker" => oa_sandbox::config::BackendPreference::Docker,
                other => {
                    anyhow::bail!("invalid backend '{other}': expected auto, native, or docker")
                }
            };
            let workspace = std::path::Path::new(&args.workspace)
                .canonicalize()
                .unwrap_or_else(|_| std::path::PathBuf::from(&args.workspace));

            let (command, cmd_args) = if args.command.is_empty() {
                anyhow::bail!("no command specified");
            } else {
                (args.command[0].clone(), args.command[1..].to_vec())
            };

            let opts = oa_cmd_sandbox::run::SandboxRunOptions {
                security,
                workspace,
                network,
                timeout: args.timeout,
                format,
                backend,
                mounts: args.mounts,
                env: args.envs,
                memory: args.memory,
                cpu: args.cpu,
                pids: args.pids,
                dry_run: args.dry_run,
                command,
                args: cmd_args,
            };
            oa_cmd_sandbox::run::sandbox_run_command(&opts)
        }
        SandboxAction::WorkerStart(args) => {
            let workspace = std::path::Path::new(&args.workspace)
                .canonicalize()
                .unwrap_or_else(|_| std::path::PathBuf::from(&args.workspace));
            let opts = oa_cmd_sandbox::worker_cmd::WorkerStartOptions {
                workspace,
                timeout: args.timeout,
                security_level: args.security_level,
                idle_timeout: args.idle_timeout,
            };
            oa_cmd_sandbox::worker_cmd::sandbox_worker_start_command(&opts)
        }
    }
}

/// Dispatch coder subcommands.
async fn dispatch_coder(cmd: CoderCommand) -> Result<()> {
    match cmd.action {
        CoderAction::Start(args) => {
            let workspace = std::path::Path::new(&args.workspace)
                .canonicalize()
                .unwrap_or_else(|_| std::path::PathBuf::from(&args.workspace));
            let opts = oa_cmd_coder::CoderStartOptions {
                workspace,
                sandboxed: args.sandboxed,
            };
            oa_cmd_coder::coder_start_command(&opts)
        }
    }
}

/// Dispatch gateway subcommands.
async fn dispatch_gateway(cmd: GatewayCommand, json: bool) -> Result<()> {
    match cmd.action {
        GatewayAction::Run(args) => {
            oa_cmd_gateway::run::gateway_run_command(args.port, args.control_ui_dir.as_deref())
                .await
        }
        GatewayAction::Start(args) => {
            oa_cmd_gateway::start::gateway_start_command(args.port, args.force).await
        }
        GatewayAction::Stop => oa_cmd_gateway::stop::gateway_stop_command().await,
        GatewayAction::Status(args) => {
            oa_cmd_gateway::status::gateway_status_command(json || args.json).await
        }
        GatewayAction::Install(args) => {
            oa_cmd_gateway::install::gateway_install_command(args.port).await
        }
        GatewayAction::Uninstall => oa_cmd_gateway::uninstall::gateway_uninstall_command().await,
        GatewayAction::Call(args) => {
            oa_cmd_gateway::call::gateway_call_command(
                &args.method,
                args.params.as_deref(),
                json || args.json,
            )
            .await
        }
        GatewayAction::UsageCost(args) => {
            oa_cmd_gateway::usage_cost::gateway_usage_cost_command(json || args.json).await
        }
        GatewayAction::Health(args) => {
            oa_cmd_gateway::health::gateway_health_command(json || args.json).await
        }
        GatewayAction::Probe(args) => {
            oa_cmd_gateway::probe::gateway_probe_command(json || args.json).await
        }
        GatewayAction::Discover(args) => {
            oa_cmd_gateway::discover::gateway_discover_command(json || args.json).await
        }
    }
}

/// Dispatch daemon subcommands (legacy aliases for gateway).
async fn dispatch_daemon(cmd: DaemonCommand, json: bool) -> Result<()> {
    match cmd.action {
        DaemonAction::Status(args) => {
            oa_cmd_daemon::commands::daemon_status_command(json || args.json).await
        }
        DaemonAction::Start => oa_cmd_daemon::commands::daemon_start_command().await,
        DaemonAction::Stop => oa_cmd_daemon::commands::daemon_stop_command().await,
        DaemonAction::Restart => oa_cmd_daemon::commands::daemon_restart_command().await,
        DaemonAction::Install => oa_cmd_daemon::commands::daemon_install_command().await,
        DaemonAction::Uninstall => oa_cmd_daemon::commands::daemon_uninstall_command().await,
    }
}

/// Dispatch logs subcommands.
async fn dispatch_logs(cmd: LogsCommand, json: bool) -> Result<()> {
    match cmd.action {
        LogsAction::Follow(args) => {
            oa_cmd_logs::follow::logs_follow_command(args.lines, args.channel.as_deref()).await
        }
        LogsAction::List(args) => oa_cmd_logs::list::logs_list_command(json || args.json).await,
        LogsAction::Show(args) => {
            oa_cmd_logs::show::logs_show_command(
                args.file.as_deref(),
                args.lines,
                json || args.json,
            )
            .await
        }
        LogsAction::Clear(args) => oa_cmd_logs::clear::logs_clear_command(args.yes).await,
        LogsAction::Export(args) => oa_cmd_logs::export::logs_export_command(&args.output).await,
    }
}

/// Dispatch memory subcommands.
async fn dispatch_memory(cmd: MemoryCommand, json: bool) -> Result<()> {
    match cmd.action {
        MemoryAction::Status(args) => {
            oa_cmd_memory::status::memory_status_command(json || args.json).await
        }
        MemoryAction::Index => oa_cmd_memory::index::memory_index_command().await,
        MemoryAction::Check(args) => {
            oa_cmd_memory::check::memory_check_command(json || args.json).await
        }
        MemoryAction::Search(args) => {
            oa_cmd_memory::search::memory_search_command(&args.query, args.limit, json || args.json)
                .await
        }
    }
}

/// Dispatch cron subcommands.
async fn dispatch_cron(cmd: CronCommand, json: bool) -> Result<()> {
    match cmd.action {
        CronAction::Status(args) => {
            oa_cmd_cron::status::cron_status_command(json || args.json).await
        }
        CronAction::List(args) => oa_cmd_cron::list::cron_list_command(json || args.json).await,
        CronAction::Add(args) => {
            oa_cmd_cron::add::cron_add_command(
                &args.name,
                &args.schedule,
                &args.agent_id,
                &args.message,
            )
            .await
        }
        CronAction::Edit(args) => {
            oa_cmd_cron::edit::cron_edit_command(
                &args.id,
                args.name.as_deref(),
                args.schedule.as_deref(),
                args.agent_id.as_deref(),
                args.message.as_deref(),
            )
            .await
        }
        CronAction::Remove(args) => oa_cmd_cron::remove::cron_remove_command(&args.id).await,
        CronAction::Enable(args) => oa_cmd_cron::enable::cron_enable_command(&args.id).await,
        CronAction::Disable(args) => oa_cmd_cron::disable::cron_disable_command(&args.id).await,
        CronAction::Runs(args) => {
            oa_cmd_cron::runs::cron_runs_command(&args.id, args.limit, json || args.json).await
        }
        CronAction::Run(args) => oa_cmd_cron::run::cron_run_command(&args.id).await,
    }
}

/// Dispatch config subcommands.
async fn dispatch_config(cmd: ConfigCommand, json: bool) -> Result<()> {
    match cmd.action {
        ConfigAction::Get(args) => {
            oa_cmd_config::get::config_get_command(&args.path, json || args.json)
        }
        ConfigAction::Set(args) => {
            oa_cmd_config::set::config_set_command(&args.path, &args.value).await
        }
        ConfigAction::Unset(args) => oa_cmd_config::unset::config_unset_command(&args.path).await,
    }
}

// ---------------------------------------------------------------------------
// Batch-3 legacy command dispatchers
// ---------------------------------------------------------------------------

/// Dispatch ACP subcommands.
async fn dispatch_acp(cmd: AcpCommand, json: bool) -> Result<()> {
    match cmd.action {
        AcpAction::Status => oa_cmd_acp::status::acp_status_command(json),
        AcpAction::Invoke(args) => oa_cmd_acp::invoke::acp_invoke_command(&args.method, json),
    }
}

/// Dispatch approvals subcommands.
async fn dispatch_approvals(cmd: ApprovalsCommand, json: bool) -> Result<()> {
    match cmd.action {
        ApprovalsAction::Get(args) => {
            oa_cmd_approvals::get::approvals_get_command(
                args.gateway,
                args.node.as_deref(),
                json || args.json,
            )
            .await
        }
        ApprovalsAction::Set(args) => {
            oa_cmd_approvals::set::approvals_set_command(
                &args.file,
                args.gateway,
                args.node.as_deref(),
            )
            .await
        }
        ApprovalsAction::AllowlistAdd(args) => oa_cmd_approvals::allowlist::allowlist_add_command(
            &args.pattern,
            args.agent.as_deref(),
            args.node.as_deref(),
        ),
        ApprovalsAction::AllowlistRemove(args) => {
            oa_cmd_approvals::allowlist::allowlist_remove_command(&args.pattern)
        }
    }
}

/// Dispatch devices subcommands.
async fn dispatch_devices(cmd: DevicesCommand, json: bool) -> Result<()> {
    match cmd.action {
        DevicesAction::List => oa_cmd_devices::list::devices_list_command(json).await,
        DevicesAction::Approve(args) => {
            oa_cmd_devices::approve::devices_approve_command(&args.request_id).await
        }
        DevicesAction::Reject(args) => {
            oa_cmd_devices::reject::devices_reject_command(&args.request_id).await
        }
        DevicesAction::Rotate(args) => {
            oa_cmd_devices::rotate::devices_rotate_command(&args.device, &args.role).await
        }
        DevicesAction::Revoke(args) => {
            oa_cmd_devices::revoke::devices_revoke_command(&args.device, &args.role).await
        }
    }
}

/// Dispatch directory subcommands.
async fn dispatch_directory(cmd: DirectoryCommand, json: bool) -> Result<()> {
    match cmd.action {
        DirectoryAction::Self_(args) => {
            oa_cmd_directory::self_cmd::directory_self_command(&args.channel, json)
        }
        DirectoryAction::Peers(args) => {
            oa_cmd_directory::peers::peers_list_command(&args.channel, args.query.as_deref(), json)
        }
        DirectoryAction::Groups(args) => oa_cmd_directory::groups::groups_list_command(
            &args.channel,
            args.query.as_deref(),
            json,
        ),
    }
}

/// Dispatch DNS subcommands.
async fn dispatch_dns(cmd: DnsCommand) -> Result<()> {
    match cmd.action {
        DnsAction::Setup(args) => oa_cmd_dns::setup::dns_setup_command(args.apply).await,
    }
}

/// Dispatch node subcommands.
async fn dispatch_node(cmd: NodeCommand, json: bool) -> Result<()> {
    match cmd.action {
        NodeAction::Run(args) => {
            oa_cmd_node::run::node_run_command(args.host.as_deref(), args.port)
        }
        NodeAction::Install(args) => {
            oa_cmd_node::install::node_install_command(args.host.as_deref(), args.port)
        }
        NodeAction::Status => oa_cmd_node::status::node_status_command(json).await,
        NodeAction::Stop => oa_cmd_node::control::node_stop_command(),
        NodeAction::Restart => oa_cmd_node::control::node_restart_command(),
        NodeAction::Uninstall => oa_cmd_node::control::node_uninstall_command(),
    }
}

/// Dispatch pairing subcommands.
async fn dispatch_pairing(cmd: PairingCommand, json: bool) -> Result<()> {
    match cmd.action {
        PairingAction::List(args) => {
            oa_cmd_pairing::list::pairing_list_command(&args.channel, json).await
        }
        PairingAction::Approve(args) => {
            oa_cmd_pairing::approve::pairing_approve_command(&args.channel, &args.code, args.notify)
                .await
        }
    }
}

/// Dispatch system subcommands.
async fn dispatch_system(cmd: SystemCommand, json: bool) -> Result<()> {
    match cmd.action {
        SystemAction::Event(args) => {
            oa_cmd_system::event::system_event_command(&args.text, args.mode.as_deref(), json)
        }
        SystemAction::HeartbeatLast => oa_cmd_system::heartbeat::heartbeat_last_command(json),
        SystemAction::HeartbeatEnable => oa_cmd_system::heartbeat::heartbeat_enable_command(),
        SystemAction::HeartbeatDisable => oa_cmd_system::heartbeat::heartbeat_disable_command(),
        SystemAction::Presence => oa_cmd_system::presence::system_presence_command(json),
    }
}

/// Dispatch update subcommands.
async fn dispatch_update(cmd: UpdateCommand, json: bool) -> Result<()> {
    match cmd.action {
        UpdateAction::Run(args) => {
            oa_cmd_update::run::update_run_command(args.channel.as_deref(), args.no_restart, json)
                .await
        }
        UpdateAction::Status => oa_cmd_update::status_cmd::update_status_command(json).await,
        UpdateAction::Wizard => oa_cmd_update::wizard::update_wizard_command(),
    }
}

/// Dispatch voicecall subcommands.
async fn dispatch_voicecall(cmd: VoicecallCommand, json: bool) -> Result<()> {
    match cmd.action {
        VoicecallAction::Status(args) => {
            oa_cmd_voicecall::status_cmd::voicecall_status_command(&args.call_id, json)
        }
        VoicecallAction::Call(args) => {
            oa_cmd_voicecall::call::voicecall_call_command(&args.to, &args.message)
        }
        VoicecallAction::Continue(args) => {
            oa_cmd_voicecall::call::voicecall_continue_command(&args.call_id, &args.message)
        }
        VoicecallAction::End(args) => oa_cmd_voicecall::call::voicecall_end_command(&args.call_id),
        VoicecallAction::Expose(args) => {
            oa_cmd_voicecall::expose::voicecall_expose_command(&args.mode)
        }
        VoicecallAction::Unexpose => oa_cmd_voicecall::expose::voicecall_unexpose_command(),
    }
}

/// Dispatch webhooks subcommands.
async fn dispatch_webhooks(cmd: WebhooksCommand, json: bool) -> Result<()> {
    match cmd.action {
        WebhooksAction::List => oa_cmd_webhooks::list::webhooks_list_command(json),
        WebhooksAction::Test(args) => {
            oa_cmd_webhooks::test_cmd::webhooks_test_command(args.url.as_deref())
        }
        WebhooksAction::GmailSetup(args) => {
            oa_cmd_webhooks::gmail::gmail_setup_command(&args.account)
        }
        WebhooksAction::GmailRun => oa_cmd_webhooks::gmail::gmail_run_command(),
    }
}

/// Dispatch skills subcommands.
async fn dispatch_skills(cmd: SkillsCommand, json: bool) -> Result<()> {
    match cmd.action {
        SkillsAction::List(args) => {
            let sa = oa_cmd_skills::SkillsListArgs {
                json: json || args.json,
                eligible: args.eligible,
                verbose: args.verbose,
            };
            oa_cmd_skills::skills_list_command(&sa).await
        }
        SkillsAction::Info(args) => {
            let sa = oa_cmd_skills::SkillsInfoArgs {
                name: args.name,
                json: json || args.json,
            };
            oa_cmd_skills::skills_info_command(&sa).await
        }
        SkillsAction::Check(args) => {
            let sa = oa_cmd_skills::SkillsCheckArgs {
                json: json || args.json,
            };
            oa_cmd_skills::skills_check_command(&sa).await
        }
    }
}

/// Dispatch plugins subcommands.
async fn dispatch_plugins(cmd: PluginsCommand, json: bool) -> Result<()> {
    match cmd.action {
        PluginsAction::List(args) => {
            let pa = oa_cmd_plugins::PluginsListArgs {
                json: json || args.json,
            };
            oa_cmd_plugins::plugins_list_command(&pa).await
        }
        PluginsAction::Info(args) => {
            let pa = oa_cmd_plugins::PluginsInfoArgs {
                id: args.id,
                json: json || args.json,
            };
            oa_cmd_plugins::plugins_info_command(&pa).await
        }
        PluginsAction::Install(args) => {
            let pa = oa_cmd_plugins::PluginsInstallArgs {
                spec: args.spec,
                link: args.link,
            };
            oa_cmd_plugins::plugins_install_command(&pa).await
        }
        PluginsAction::Enable(args) => {
            let pa = oa_cmd_plugins::PluginsEnableArgs { id: args.id };
            oa_cmd_plugins::plugins_enable_command(&pa).await
        }
        PluginsAction::Disable(args) => {
            let pa = oa_cmd_plugins::PluginsDisableArgs { id: args.id };
            oa_cmd_plugins::plugins_disable_command(&pa).await
        }
        PluginsAction::Doctor(args) => {
            let pa = oa_cmd_plugins::PluginsDoctorArgs {
                json: json || args.json,
            };
            oa_cmd_plugins::plugins_doctor_command(&pa).await
        }
    }
}

/// Dispatch hooks subcommands.
async fn dispatch_hooks(cmd: HooksCommand, json: bool) -> Result<()> {
    match cmd.action {
        HooksAction::List(args) => {
            let ha = oa_cmd_hooks::HooksListArgs {
                json: json || args.json,
                eligible: args.eligible,
                verbose: args.verbose,
            };
            oa_cmd_hooks::hooks_list_command(&ha).await
        }
        HooksAction::Info(args) => {
            let ha = oa_cmd_hooks::HooksInfoArgs {
                name: args.name,
                json: json || args.json,
            };
            oa_cmd_hooks::hooks_info_command(&ha).await
        }
        HooksAction::Check(args) => {
            let ha = oa_cmd_hooks::HooksCheckArgs {
                json: json || args.json,
            };
            oa_cmd_hooks::hooks_check_command(&ha).await
        }
        HooksAction::Enable(args) => {
            let ha = oa_cmd_hooks::HooksEnableArgs { name: args.name };
            oa_cmd_hooks::hooks_enable_command(&ha).await
        }
        HooksAction::Disable(args) => {
            let ha = oa_cmd_hooks::HooksDisableArgs { name: args.name };
            oa_cmd_hooks::hooks_disable_command(&ha).await
        }
        HooksAction::Install(args) => {
            let ha = oa_cmd_hooks::HooksInstallArgs {
                spec: args.spec,
                link: args.link,
            };
            oa_cmd_hooks::hooks_install_command(&ha).await
        }
        HooksAction::Update(args) => {
            let ha = oa_cmd_hooks::HooksUpdateArgs {
                id: args.id,
                all: args.all,
                dry_run: args.dry_run,
            };
            oa_cmd_hooks::hooks_update_command(&ha).await
        }
    }
}

/// Dispatch browser subcommands.
async fn dispatch_browser(cmd: BrowserCommand, json: bool) -> Result<()> {
    match cmd.action {
        // -- Manage --
        BrowserAction::Status(args) => {
            oa_cmd_browser::browser_status_command(&oa_cmd_browser::BrowserStatusArgs {
                json: json || args.json,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Start(args) => {
            oa_cmd_browser::browser_start_command(&oa_cmd_browser::BrowserStartArgs {
                browser_profile: args.browser_profile,
                headless: args.headless,
            })
            .await
        }
        BrowserAction::Stop(args) => {
            oa_cmd_browser::browser_stop_command(&oa_cmd_browser::BrowserStopArgs {
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Tabs(args) => {
            oa_cmd_browser::browser_tabs_command(&oa_cmd_browser::BrowserTabsArgs {
                json: json || args.json,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Open(args) => {
            oa_cmd_browser::browser_open_command(&oa_cmd_browser::BrowserOpenArgs {
                url: args.url,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Focus(args) => {
            oa_cmd_browser::browser_focus_command(&oa_cmd_browser::BrowserFocusArgs {
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Close(args) => {
            oa_cmd_browser::browser_close_command(&oa_cmd_browser::BrowserCloseArgs {
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Screenshot(args) => {
            oa_cmd_browser::browser_screenshot_command(&oa_cmd_browser::BrowserScreenshotArgs {
                file: args.file,
                full_page: args.full_page,
                r#ref: args.ref_id,
                element: args.element,
                format: args.format,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Profiles(args) => {
            oa_cmd_browser::browser_profiles_command(&oa_cmd_browser::BrowserProfilesArgs {
                json: json || args.json,
            })
            .await
        }
        BrowserAction::CreateProfile(args) => {
            oa_cmd_browser::browser_create_profile_command(
                &oa_cmd_browser::BrowserCreateProfileArgs {
                    name: args.name,
                    color: args.color,
                    cdp_url: args.cdp_url,
                    driver: args.driver,
                },
            )
            .await
        }
        BrowserAction::DeleteProfile(args) => {
            oa_cmd_browser::browser_delete_profile_command(
                &oa_cmd_browser::BrowserDeleteProfileArgs { name: args.name },
            )
            .await
        }
        BrowserAction::ResetProfile(args) => {
            oa_cmd_browser::browser_reset_profile_command(
                &oa_cmd_browser::BrowserResetProfileArgs {
                    browser_profile: args.browser_profile,
                },
            )
            .await
        }
        // -- Inspect --
        BrowserAction::Snapshot(args) => {
            oa_cmd_browser::browser_snapshot_command(&oa_cmd_browser::BrowserSnapshotArgs {
                format: args.format,
                target_id: args.target_id,
                limit: args.limit,
                interactive: args.interactive,
                compact: args.compact,
                depth: args.depth,
                selector: args.selector,
                frame: args.frame,
                efficient: args.efficient,
                labels: args.labels,
                out: args.out,
                json: json || args.json,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Console(args) => {
            oa_cmd_browser::browser_console_command(&oa_cmd_browser::BrowserConsoleArgs {
                level: args.level,
                clear: args.clear,
                target_id: args.target_id,
                json: json || args.json,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Errors(args) => {
            oa_cmd_browser::browser_errors_command(&oa_cmd_browser::BrowserErrorsArgs {
                clear: args.clear,
                target_id: args.target_id,
                json: json || args.json,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Requests(args) => {
            oa_cmd_browser::browser_requests_command(&oa_cmd_browser::BrowserRequestsArgs {
                filter: args.filter,
                clear: args.clear,
                target_id: args.target_id,
                json: json || args.json,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::ResponseBody(args) => {
            oa_cmd_browser::browser_responsebody_command(&oa_cmd_browser::BrowserResponseBodyArgs {
                pattern: args.pattern,
                max_chars: args.max_chars,
                target_id: args.target_id,
                json: json || args.json,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Pdf(args) => {
            oa_cmd_browser::browser_pdf_command(&oa_cmd_browser::BrowserPdfArgs {
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        // -- Actions --
        BrowserAction::Navigate(args) => {
            oa_cmd_browser::browser_navigate_command(&oa_cmd_browser::BrowserNavigateArgs {
                url: args.url,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Resize(args) => {
            oa_cmd_browser::browser_resize_command(&oa_cmd_browser::BrowserResizeArgs {
                width: args.width,
                height: args.height,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Click(args) => {
            oa_cmd_browser::browser_click_command(&oa_cmd_browser::BrowserClickArgs {
                r#ref: args.ref_id,
                double: args.double,
                button: args.button,
                modifiers: args.modifiers,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::TypeText(args) => {
            oa_cmd_browser::browser_type_command(&oa_cmd_browser::BrowserTypeArgs {
                r#ref: args.ref_id,
                text: args.text,
                submit: args.submit,
                slowly: args.slowly,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Press(args) => {
            oa_cmd_browser::browser_press_command(&oa_cmd_browser::BrowserPressArgs {
                key: args.key,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Hover(args) => {
            oa_cmd_browser::browser_hover_command(&oa_cmd_browser::BrowserHoverArgs {
                r#ref: args.ref_id,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::ScrollIntoView(args) => {
            oa_cmd_browser::browser_scrollintoview_command(
                &oa_cmd_browser::BrowserScrollIntoViewArgs {
                    r#ref: args.ref_id,
                    target_id: args.target_id,
                    browser_profile: args.browser_profile,
                },
            )
            .await
        }
        BrowserAction::Drag(args) => {
            oa_cmd_browser::browser_drag_command(&oa_cmd_browser::BrowserDragArgs {
                start_ref: args.start_ref,
                end_ref: args.end_ref,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Select(args) => {
            oa_cmd_browser::browser_select_command(&oa_cmd_browser::BrowserSelectArgs {
                r#ref: args.ref_id,
                values: args.values,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Download(args) => {
            oa_cmd_browser::browser_download_command(&oa_cmd_browser::BrowserDownloadArgs {
                r#ref: args.ref_id,
                save_path: args.save_path,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::WaitForDownload(args) => {
            oa_cmd_browser::browser_waitfordownload_command(
                &oa_cmd_browser::BrowserWaitForDownloadArgs {
                    save_path: args.save_path,
                    timeout_ms: args.timeout_ms,
                    browser_profile: args.browser_profile,
                },
            )
            .await
        }
        BrowserAction::Upload(args) => {
            oa_cmd_browser::browser_upload_command(&oa_cmd_browser::BrowserUploadArgs {
                paths: args.paths,
                r#ref: args.ref_id,
                input_ref: args.input_ref,
                element: args.element,
                target_id: args.target_id,
                timeout_ms: args.timeout_ms,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Fill(args) => {
            oa_cmd_browser::browser_fill_command(&oa_cmd_browser::BrowserFillArgs {
                fields: args.fields,
                fields_file: args.fields_file,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Dialog(args) => {
            oa_cmd_browser::browser_dialog_command(&oa_cmd_browser::BrowserDialogArgs {
                accept: args.accept,
                dismiss: args.dismiss,
                prompt: args.prompt,
                target_id: args.target_id,
                timeout_ms: args.timeout_ms,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Wait(args) => {
            oa_cmd_browser::browser_wait_command(&oa_cmd_browser::BrowserWaitArgs {
                selector: args.selector,
                url: args.url,
                load: args.load,
                js_fn: args.js_fn,
                text: args.text,
                text_gone: args.text_gone,
                time: args.time,
                timeout_ms: args.timeout_ms,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Evaluate(args) => {
            oa_cmd_browser::browser_evaluate_command(&oa_cmd_browser::BrowserEvaluateArgs {
                js_fn: args.js_fn,
                r#ref: args.ref_id,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        BrowserAction::Highlight(args) => {
            oa_cmd_browser::browser_highlight_command(&oa_cmd_browser::BrowserHighlightArgs {
                r#ref: args.ref_id,
                target_id: args.target_id,
                browser_profile: args.browser_profile,
            })
            .await
        }
        // -- Trace --
        BrowserAction::Trace(cmd) => match cmd.action {
            BrowserTraceAction::Start(args) => {
                oa_cmd_browser::browser_trace_start_command(
                    &oa_cmd_browser::BrowserTraceStartArgs {
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
            BrowserTraceAction::Stop(args) => {
                oa_cmd_browser::browser_trace_stop_command(&oa_cmd_browser::BrowserTraceStopArgs {
                    browser_profile: args.browser_profile,
                })
                .await
            }
        },
        // -- Cookies --
        BrowserAction::Cookies(cmd) => match cmd.action {
            None => {
                oa_cmd_browser::browser_cookies_command(&oa_cmd_browser::BrowserCookiesArgs {
                    json: json || cmd.json,
                    browser_profile: cmd.browser_profile,
                })
                .await
            }
            Some(BrowserCookiesAction::Set(args)) => {
                oa_cmd_browser::browser_cookies_set_command(
                    &oa_cmd_browser::BrowserCookiesSetArgs {
                        name: args.name,
                        value: args.value,
                        url: args.url,
                        browser_profile: args.browser_profile.or(cmd.browser_profile),
                    },
                )
                .await
            }
            Some(BrowserCookiesAction::Clear(args)) => {
                oa_cmd_browser::browser_cookies_clear_command(
                    &oa_cmd_browser::BrowserCookiesClearArgs {
                        browser_profile: args.browser_profile.or(cmd.browser_profile),
                    },
                )
                .await
            }
        },
        // -- Storage --
        BrowserAction::Storage(cmd) => match cmd.action {
            BrowserStorageAction::Get(args) => {
                oa_cmd_browser::browser_storage_get_command(
                    &oa_cmd_browser::BrowserStorageGetArgs {
                        kind: args.kind,
                        json: json || args.json,
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
            BrowserStorageAction::Set(args) => {
                oa_cmd_browser::browser_storage_set_command(
                    &oa_cmd_browser::BrowserStorageSetArgs {
                        kind: args.kind,
                        key: args.key,
                        value: args.value,
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
            BrowserStorageAction::Clear(args) => {
                oa_cmd_browser::browser_storage_clear_command(
                    &oa_cmd_browser::BrowserStorageClearArgs {
                        kind: args.kind,
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
        },
        // -- Set --
        BrowserAction::Set(cmd) => match cmd.action {
            BrowserSetAction::Offline(args) => {
                oa_cmd_browser::browser_set_offline_command(
                    &oa_cmd_browser::BrowserSetOfflineArgs {
                        state: args.state,
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
            BrowserSetAction::Headers(args) => {
                oa_cmd_browser::browser_set_headers_command(
                    &oa_cmd_browser::BrowserSetHeadersArgs {
                        json: args.json,
                        clear: args.clear,
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
            BrowserSetAction::Credentials(args) => {
                oa_cmd_browser::browser_set_credentials_command(
                    &oa_cmd_browser::BrowserSetCredentialsArgs {
                        username: args.username,
                        password: args.password,
                        clear: args.clear,
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
            BrowserSetAction::Geo(args) => {
                oa_cmd_browser::browser_set_geo_command(&oa_cmd_browser::BrowserSetGeoArgs {
                    latitude: args.latitude,
                    longitude: args.longitude,
                    origin: args.origin,
                    clear: args.clear,
                    browser_profile: args.browser_profile,
                })
                .await
            }
            BrowserSetAction::Media(args) => {
                oa_cmd_browser::browser_set_media_command(&oa_cmd_browser::BrowserSetMediaArgs {
                    scheme: args.scheme,
                    browser_profile: args.browser_profile,
                })
                .await
            }
            BrowserSetAction::Timezone(args) => {
                oa_cmd_browser::browser_set_timezone_command(
                    &oa_cmd_browser::BrowserSetTimezoneArgs {
                        timezone: args.timezone,
                        browser_profile: args.browser_profile,
                    },
                )
                .await
            }
            BrowserSetAction::Locale(args) => {
                oa_cmd_browser::browser_set_locale_command(&oa_cmd_browser::BrowserSetLocaleArgs {
                    locale: args.locale,
                    browser_profile: args.browser_profile,
                })
                .await
            }
            BrowserSetAction::Device(args) => {
                oa_cmd_browser::browser_set_device_command(&oa_cmd_browser::BrowserSetDeviceArgs {
                    device: args.device,
                    browser_profile: args.browser_profile,
                })
                .await
            }
        },
    }
}
