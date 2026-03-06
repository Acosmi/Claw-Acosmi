/// Configuration management commands for Claw Acosmi CLI.
///
/// Provides the `configure` command and its sub-wizards for managing
/// gateway, auth, channels, skills, daemon, and workspace settings.
///
/// Source: `src/commands/configure*.ts`

pub mod shared;
pub mod gateway_auth;
pub mod gateway;
pub mod channels;
pub mod daemon;
pub mod wizard;

use anyhow::Result;

use crate::shared::WizardSection;

/// Execute the configure command with all sections available interactively.
///
/// Source: `src/commands/configure.commands.ts` - `configureCommand`
pub async fn execute() -> Result<()> {
    wizard::run_configure_wizard(wizard::ConfigureWizardParams {
        command: wizard::WizardCommand::Configure,
        sections: None,
    })
    .await
}

/// Execute the configure command with specific sections pre-selected.
///
/// Source: `src/commands/configure.commands.ts` - `configureCommandWithSections`
pub async fn execute_with_sections(sections: Vec<WizardSection>) -> Result<()> {
    wizard::run_configure_wizard(wizard::ConfigureWizardParams {
        command: wizard::WizardCommand::Configure,
        sections: Some(sections),
    })
    .await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wizard_sections_are_complete() {
        let sections = shared::CONFIGURE_WIZARD_SECTIONS;
        assert_eq!(sections.len(), 8);
        assert!(sections.contains(&WizardSection::Workspace));
        assert!(sections.contains(&WizardSection::Model));
        assert!(sections.contains(&WizardSection::Web));
        assert!(sections.contains(&WizardSection::Gateway));
        assert!(sections.contains(&WizardSection::Daemon));
        assert!(sections.contains(&WizardSection::Channels));
        assert!(sections.contains(&WizardSection::Skills));
        assert!(sections.contains(&WizardSection::Health));
    }

    #[test]
    fn section_options_have_labels_and_hints() {
        for option in shared::CONFIGURE_SECTION_OPTIONS {
            assert!(!option.label.is_empty());
            assert!(!option.hint.is_empty());
        }
    }
}
