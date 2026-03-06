/// Authentication and OAuth commands for Claw Acosmi CLI.
///
/// This crate implements the auth-choice wizard flows, API key management,
/// OAuth flows (OpenAI Codex, Chutes, plugin providers), and provider-specific
/// configuration. It mirrors the TypeScript implementation in:
///
/// - `src/commands/auth-choice*.ts`
/// - `src/commands/auth-token.ts`
/// - `src/commands/oauth-env.ts`
/// - `src/commands/oauth-flow.ts`
/// - `src/commands/chutes-oauth.ts`
///
/// Source: `src/commands/auth-choice.ts`

/// Auth choice types: the `AuthChoice` enum and `AuthChoiceGroupId`.
///
/// Source: `src/commands/onboard-types.ts`, `src/commands/auth-choice-options.ts`
pub mod auth_choice;

/// API key normalization, validation, and preview formatting.
///
/// Source: `src/commands/auth-choice.api-key.ts`
pub mod api_key;

/// Auth token validation and profile ID construction.
///
/// Source: `src/commands/auth-token.ts`
pub mod auth_token;

/// OAuth environment detection (remote/VPS vs local).
///
/// Source: `src/commands/oauth-env.ts`
pub mod oauth_env;

/// OAuth flow helpers (VPS-aware handlers).
///
/// Source: `src/commands/oauth-flow.ts`
pub mod oauth_flow;

/// Auth choice options and groups (provider menus).
///
/// Source: `src/commands/auth-choice-options.ts`
pub mod options;

/// Preferred provider mapping for each auth choice.
///
/// Source: `src/commands/auth-choice.preferred-provider.ts`
pub mod preferred_provider;

/// Default model choice application logic.
///
/// Source: `src/commands/auth-choice.default-model.ts`
pub mod default_model;

/// Model configuration validation and warnings.
///
/// Source: `src/commands/auth-choice.model-check.ts`
pub mod model_check;

/// Apply auth choice dispatcher and per-provider handlers.
///
/// Source: `src/commands/auth-choice.apply.ts` and `auth-choice.apply.*.ts`
pub mod apply;

use anyhow::Result;

/// Execute the auth command.
///
/// Source: `src/commands/auth-choice.ts`
pub async fn execute() -> Result<()> {
    tracing::info!("auth command invoked");
    Ok(())
}
