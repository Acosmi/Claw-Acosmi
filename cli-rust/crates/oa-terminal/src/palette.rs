/// Color palette definitions for Claw Acosmi CLI.
///
/// Source: `src/terminal/palette.ts`

/// Lobster palette — primary CLI/UI theming constants.
///
/// All values are hex color strings (without `#` prefix for internal use).
pub struct LobsterPalette {
    /// Primary orange accent
    pub accent: &'static str,
    /// Bright orange variant
    pub accent_bright: &'static str,
    /// Dimmed orange variant
    pub accent_dim: &'static str,
    /// Info tone (lighter orange)
    pub info: &'static str,
    /// Green success color
    pub success: &'static str,
    /// Yellow/gold warning color
    pub warn: &'static str,
    /// Red error color
    pub error: &'static str,
    /// Gray muted color
    pub muted: &'static str,
}

/// The default Claw Acosmi lobster palette.
pub static LOBSTER_PALETTE: LobsterPalette = LobsterPalette {
    accent: "#FF5A2D",
    accent_bright: "#FF7A3D",
    accent_dim: "#D14A22",
    info: "#FF8A5B",
    success: "#2FBF71",
    warn: "#FFB020",
    error: "#E23D2D",
    muted: "#8B7F77",
};

/// TUI palette — for interactive terminal UI surfaces.
///
/// Source: `src/tui/theme/theme.ts`
pub struct TuiPalette {
    pub text: &'static str,
    pub dim: &'static str,
    pub accent: &'static str,
    pub accent_soft: &'static str,
    pub border: &'static str,
    pub user_bg: &'static str,
    pub user_text: &'static str,
    pub system_text: &'static str,
    pub tool_pending_bg: &'static str,
    pub tool_success_bg: &'static str,
    pub tool_error_bg: &'static str,
    pub tool_title: &'static str,
    pub tool_output: &'static str,
    pub quote: &'static str,
    pub quote_border: &'static str,
    pub code: &'static str,
    pub code_block: &'static str,
    pub code_border: &'static str,
    pub link: &'static str,
    pub error: &'static str,
    pub success: &'static str,
}

/// The default TUI palette.
pub static TUI_PALETTE: TuiPalette = TuiPalette {
    text: "#E8E3D5",
    dim: "#7B7F87",
    accent: "#F6C453",
    accent_soft: "#F2A65A",
    border: "#3C414B",
    user_bg: "#2B2F36",
    user_text: "#F3EEE0",
    system_text: "#9BA3B2",
    tool_pending_bg: "#1F2A2F",
    tool_success_bg: "#1E2D23",
    tool_error_bg: "#2F1F1F",
    tool_title: "#F6C453",
    tool_output: "#E1DACB",
    quote: "#8CC8FF",
    quote_border: "#3B4D6B",
    code: "#F0C987",
    code_block: "#1E232A",
    code_border: "#343A45",
    link: "#7DD3A5",
    error: "#F97066",
    success: "#7DD3A5",
};
