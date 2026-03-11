import { html, svg, type TemplateResult } from "lit";

// Lucide-style SVG icons
// All icons use currentColor for stroke

function navAccentIcon(
  symbol: ReturnType<typeof svg>,
  detail: ReturnType<typeof svg> | string = "",
): ReturnType<typeof svg> {
  // Nested SVG fragments must use lit's `svg` template or child nodes can end up in the HTML namespace.
  return svg`
    <svg class="nav-accent-icon" viewBox="0 0 24 24" fill="none">
      ${symbol}
      ${detail}
    </svg>
  `;
}

export const icons = {
  // Navigation icons
  messageSquare: html`
    <svg viewBox="0 0 24 24">
      <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
    </svg>
  `,
  barChart: html`
    <svg viewBox="0 0 24 24">
      <line x1="12" x2="12" y1="20" y2="10" />
      <line x1="18" x2="18" y1="20" y2="4" />
      <line x1="6" x2="6" y1="20" y2="16" />
    </svg>
  `,
  link: html`
    <svg viewBox="0 0 24 24">
      <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
      <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
    </svg>
  `,
  radio: html`
    <svg viewBox="0 0 24 24">
      <circle cx="12" cy="12" r="2" />
      <path
        d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14"
      />
    </svg>
  `,
  fileText: html`
    <svg viewBox="0 0 24 24">
      <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
      <polyline points="14 2 14 8 20 8" />
      <line x1="16" x2="8" y1="13" y2="13" />
      <line x1="16" x2="8" y1="17" y2="17" />
      <line x1="10" x2="8" y1="9" y2="9" />
    </svg>
  `,
  zap: html`
    <svg viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" /></svg>
  `,
  monitor: html`
    <svg viewBox="0 0 24 24">
      <rect width="20" height="14" x="2" y="3" rx="2" />
      <line x1="8" x2="16" y1="21" y2="21" />
      <line x1="12" x2="12" y1="17" y2="21" />
    </svg>
  `,
  settings: html`
    <svg viewBox="0 0 24 24">
      <path
        d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"
      />
      <circle cx="12" cy="12" r="3" />
    </svg>
  `,
  bug: html`
    <svg viewBox="0 0 24 24">
      <path d="m8 2 1.88 1.88" />
      <path d="M14.12 3.88 16 2" />
      <path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1" />
      <path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6" />
      <path d="M12 20v-9" />
      <path d="M6.53 9C4.6 8.8 3 7.1 3 5" />
      <path d="M6 13H2" />
      <path d="M3 21c0-2.1 1.7-3.9 3.8-4" />
      <path d="M20.97 5c0 2.1-1.6 3.8-3.5 4" />
      <path d="M22 13h-4" />
      <path d="M17.2 17c2.1.1 3.8 1.9 3.8 4" />
    </svg>
  `,
  scrollText: html`
    <svg viewBox="0 0 24 24">
      <path d="M8 21h12a2 2 0 0 0 2-2v-2H10v2a2 2 0 1 1-4 0V5a2 2 0 1 0-4 0v3h4" />
      <path d="M19 17V5a2 2 0 0 0-2-2H4" />
      <path d="M15 8h-5" />
      <path d="M15 12h-5" />
    </svg>
  `,
  folder: html`
    <svg viewBox="0 0 24 24">
      <path
        d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"
      />
    </svg>
  `,
  chatSpark: navAccentIcon(
    svg`
      <path
        d="M6.95 8.55A2.05 2.05 0 0 1 9 6.5h5.8a2.05 2.05 0 0 1 2.05 2.05v4.1A2.05 2.05 0 0 1 14.8 14.7h-3.15L8.8 16.9v-2.2H9a2.05 2.05 0 0 1-2.05-2.05Z"
        stroke="#2563eb"
        stroke-width="1.65"
        stroke-linejoin="round"
      />
      <circle cx="10.25" cy="10.6" r="0.72" fill="#2563eb" />
      <circle cx="12" cy="10.6" r="0.72" fill="#0ea5e9" />
      <circle cx="13.75" cy="10.6" r="0.72" fill="#22d3ee" />
    `,
    svg`
      <path d="M17.05 6.9l.4.96.96.4-.96.4-.4.96-.4-.96-.96-.4.96-.4.4-.96Z" fill="#f59e0b" />
    `,
  ),
  taskOrbit: navAccentIcon(
    svg`
      <circle cx="11.8" cy="12" r="4.8" stroke="#0891b2" stroke-width="1.55" />
      <path d="m9.65 12.05 1.5 1.55 2.85-3.05" stroke="#22c55e" stroke-width="1.85" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M11.8 6a6.1 6.1 0 0 1 5.4 3.15" stroke="#1d4ed8" stroke-width="1.2" stroke-linecap="round" />
      <circle cx="17.35" cy="9.2" r="0.96" fill="#f59e0b" />
    `,
  ),
  automationHub: navAccentIcon(
    svg`
      <rect x="7.1" y="8.05" width="3.35" height="3.35" rx="0.95" stroke="#0ea5e9" stroke-width="1.3" />
      <rect x="13.55" y="8.05" width="3.35" height="3.35" rx="0.95" stroke="#22c55e" stroke-width="1.3" />
      <rect x="10.35" y="13.25" width="3.35" height="3.35" rx="0.95" stroke="#f97316" stroke-width="1.3" />
      <path d="M10.45 9.7h3.1M12 11.5v1.55" stroke="#64748b" stroke-width="1.3" stroke-linecap="round" />
      <circle cx="17.35" cy="8.15" r="0.82" fill="#f59e0b" />
    `,
  ),
  agentSwarm: navAccentIcon(
    svg`
      <path d="M8.2 9.05 12 11.55M15.8 9.05 12 11.55M12 13.2v2.7" stroke="#64748b" stroke-width="1.4" stroke-linecap="round" />
      <circle cx="8.2" cy="8.8" r="1.7" fill="#0ea5e9" />
      <circle cx="15.8" cy="8.8" r="1.7" fill="#22c55e" />
      <circle cx="12" cy="16.05" r="1.95" fill="#f97316" />
      <circle cx="12" cy="11.55" r="0.72" fill="#0f172a" />
    `,
  ),
  nodeMesh: navAccentIcon(
    svg`
      <path d="M9.1 9.25h5.8M10.65 10.65l2.4 2.5M13.05 10.65l-2.4 2.5" stroke="#64748b" stroke-width="1.35" stroke-linecap="round" />
      <rect x="6.45" y="6.8" width="3.45" height="3.45" rx="1.02" stroke="#2563eb" stroke-width="1.35" />
      <rect x="14.1" y="6.8" width="3.45" height="3.45" rx="1.02" stroke="#06b6d4" stroke-width="1.35" />
      <rect x="10.28" y="13.45" width="3.45" height="3.45" rx="1.02" stroke="#475569" stroke-width="1.35" />
    `,
  ),
  navMedia: navAccentIcon(
    svg`
      <rect x="6.95" y="10.1" width="2" height="5.35" rx="1" fill="#7c3aed" />
      <rect x="11" y="7.95" width="2" height="9.7" rx="1" fill="#0ea5e9" />
      <rect x="15.05" y="9.1" width="2" height="7.4" rx="1" fill="#14b8a6" />
      <path d="M6.7 14.15c1.15-2.2 2.3-2.2 3.5 0 1.15 2.1 2.25 2.05 3.35-.2 1-1.95 1.95-2.2 3.2-.7" stroke="#f59e0b" stroke-width="1.35" stroke-linecap="round" />
    `,
  ),
  wizardDashboard: navAccentIcon(
    svg`
      <path d="M7 14.9a5 5 0 0 1 10 0" stroke="#1d4ed8" stroke-width="1.65" stroke-linecap="round" />
      <path d="M12 11.15 15 9.4" stroke="#06b6d4" stroke-width="1.8" stroke-linecap="round" />
      <circle cx="12" cy="11.15" r="1.35" fill="#0ea5e9" />
      <circle cx="8.1" cy="14.9" r="0.78" fill="#22c55e" />
      <circle cx="15.9" cy="14.9" r="0.78" fill="#f97316" />
    `,
    svg`
      <path d="M17.35 7.3l.35.85.85.35-.85.35-.35.85-.35-.85-.85-.35.85-.35.35-.85Z" fill="#f59e0b" />
    `,
  ),
  channelBridge: navAccentIcon(
    svg`
      <rect x="6.05" y="7.95" width="4.35" height="3.35" rx="1.05" stroke="#0f766e" stroke-width="1.25" />
      <rect x="13.6" y="12.7" width="4.35" height="3.35" rx="1.05" stroke="#8b5cf6" stroke-width="1.25" />
      <path d="M10.75 9.65h1.8c1.35 0 2.45 1.1 2.45 2.45v.2" stroke="#0ea5e9" stroke-width="1.55" stroke-linecap="round" />
      <path d="m13.45 9.15 1.8.55-.9 1.55" stroke="#0ea5e9" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round" />
      <circle cx="8.2" cy="9.62" r="0.62" fill="#0ea5e9" />
      <circle cx="15.78" cy="14.35" r="0.62" fill="#8b5cf6" />
    `,
  ),
  pluginCircuit: navAccentIcon(
    svg`
      <rect x="8.95" y="8.95" width="6.1" height="6.1" rx="1.55" stroke="#7c3aed" stroke-width="1.45" />
      <path d="M12 6.6v2M12 15.4v2M6.6 12h2M15.4 12h2" stroke="#4338ca" stroke-width="1.35" stroke-linecap="round" />
      <circle cx="12" cy="12" r="1.02" fill="#ec4899" />
      <circle cx="16.75" cy="12" r="0.58" fill="#4338ca" />
    `,
  ),
  mcpBridge: navAccentIcon(
    svg`
      <rect x="7.15" y="7.8" width="8.75" height="3.2" rx="1.02" stroke="#2563eb" stroke-width="1.3" />
      <rect x="8.1" y="12.95" width="8.75" height="3.2" rx="1.02" stroke="#14b8a6" stroke-width="1.3" />
      <path d="M15.9 9.4h1.35a1.4 1.4 0 0 1 0 2.8h-.4M8.1 14.55H6.8" stroke="#475569" stroke-width="1.3" stroke-linecap="round" />
      <circle cx="8.85" cy="9.4" r="0.52" fill="#2563eb" />
      <circle cx="9.8" cy="14.55" r="0.52" fill="#14b8a6" />
    `,
  ),
  memoryVault: navAccentIcon(
    svg`
      <circle cx="12" cy="12" r="4.85" stroke="#0891b2" stroke-width="1.45" />
      <circle cx="12" cy="12" r="2.1" stroke="#38bdf8" stroke-width="1.35" />
      <circle cx="12" cy="12" r="0.92" fill="#0ea5e9" />
      <path d="M12 7.2v1.45M16.8 12h-1.45M12 16.8v-1.45M7.2 12h1.45" stroke="#0f766e" stroke-width="1.2" stroke-linecap="round" />
      <circle cx="16.15" cy="8.75" r="0.8" fill="#f59e0b" />
    `,
  ),
  cronOrbit: navAccentIcon(
    svg`
      <circle cx="12" cy="12" r="4.8" stroke="#14b8a6" stroke-width="1.55" />
      <path d="M12 9.15v2.95l2.2 1.45" stroke="#84cc16" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M7.1 10.05A6 6 0 0 1 17.8 8.6" stroke="#0f766e" stroke-width="1.15" stroke-linecap="round" />
      <circle cx="17.25" cy="9.35" r="0.86" fill="#84cc16" />
    `,
  ),
  securityPulse: navAccentIcon(
    svg`
      <path
        d="M12 7.05c1.45 1.15 3 1.8 4.7 1.95v3.05c0 2.8-1.65 4.65-4.7 5.7-3.05-1.05-4.7-2.9-4.7-5.7V9c1.7-.15 3.25-.8 4.7-1.95Z"
        stroke="#2563eb"
        stroke-width="1.55"
        stroke-linejoin="round"
      />
      <path d="m10.1 12.15 1.35 1.35 2.55-2.7" stroke="#22c55e" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" />
      <circle cx="16.25" cy="8.75" r="0.76" fill="#7c3aed" />
    `,
  ),
  configSliders: navAccentIcon(
    svg`
      <path d="M7.25 8.8h9.5M7.25 12h9.5M7.25 15.2h9.5" stroke="#64748b" stroke-width="1.35" stroke-linecap="round" />
      <circle cx="10.15" cy="8.8" r="1.22" fill="#1f2937" stroke="#0ea5e9" stroke-width="1.05" />
      <circle cx="13.9" cy="12" r="1.22" fill="#334155" stroke="#38bdf8" stroke-width="1.05" />
      <circle cx="11.55" cy="15.2" r="1.22" fill="#475569" stroke="#60a5fa" stroke-width="1.05" />
    `,
  ),
  debugRadar: navAccentIcon(
    svg`
      <path d="M12 7.15a4.85 4.85 0 0 1 4.85 4.85" stroke="#1d4ed8" stroke-width="1.55" stroke-linecap="round" />
      <path d="M12 9.65a2.35 2.35 0 0 1 2.35 2.35" stroke="#8b5cf6" stroke-width="1.45" stroke-linecap="round" />
      <circle cx="12" cy="12" r="1.25" fill="#8b5cf6" />
      <path d="M12 12 16.05 9.6" stroke="#1d4ed8" stroke-width="1.65" stroke-linecap="round" />
      <circle cx="16.2" cy="9.45" r="0.76" fill="#f472b6" />
      <path d="M7.3 16.7a7 7 0 0 0 9.4 0" stroke="#111827" stroke-width="1.15" stroke-linecap="round" />
    `,
  ),
  logStack: navAccentIcon(
    svg`
      <path d="M8.2 7.8h6l2 2.05v6.45A1.5 1.5 0 0 1 14.7 17.8H8.2a1.5 1.5 0 0 1-1.5-1.5V9.3A1.5 1.5 0 0 1 8.2 7.8Z" stroke="#475569" stroke-width="1.45" stroke-linejoin="round" />
      <path d="M14.2 7.8v2.25h2.2" stroke="#0ea5e9" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M9.1 11.35h5.1M9.1 13.4h5.55M9.1 15.45h3.7" stroke="#64748b" stroke-width="1.3" stroke-linecap="round" />
    `,
  ),
  brandGlobe: navAccentIcon(
    svg`
      <circle cx="11.15" cy="12.45" r="4.55" stroke="#0ea5e9" stroke-width="1.5" />
      <path d="M6.6 12.45h9.1M11.15 7.9c1.35 1.3 2.05 2.8 2.05 4.55S12.5 15.7 11.15 17c-1.35-1.3-2.05-2.8-2.05-4.55s.7-3.25 2.05-4.55Z" stroke="#22c55e" stroke-width="1.2" />
      <path d="M15.95 7.2h2.7v2.7" stroke="#f59e0b" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M18.5 7.35 14.95 10.9" stroke="#f59e0b" stroke-width="1.35" stroke-linecap="round" />
    `,
  ),

  // UI icons
  menu: html`
    <svg viewBox="0 0 24 24">
      <line x1="4" x2="20" y1="12" y2="12" />
      <line x1="4" x2="20" y1="6" y2="6" />
      <line x1="4" x2="20" y1="18" y2="18" />
    </svg>
  `,
  bell: html`
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"></path>
      <path d="M13.73 21a2 2 0 0 1-3.46 0"></path>
    </svg>
  `,
  x: html`
    <svg viewBox="0 0 24 24">
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  `,
  check: html`
    <svg viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5" /></svg>
  `,
  arrowDown: html`
    <svg viewBox="0 0 24 24">
      <path d="M12 5v14" />
      <path d="m19 12-7 7-7-7" />
    </svg>
  `,
  copy: html`
    <svg viewBox="0 0 24 24">
      <rect width="14" height="14" x="8" y="8" rx="2" ry="2" />
      <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
    </svg>
  `,
  search: html`
    <svg viewBox="0 0 24 24">
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.3-4.3" />
    </svg>
  `,
  brain: html`
    <svg viewBox="0 0 24 24">
      <path d="M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Z" />
      <path d="M12 5a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18Z" />
      <path d="M15 13a4.5 4.5 0 0 1-3-4 4.5 4.5 0 0 1-3 4" />
      <path d="M17.599 6.5a3 3 0 0 0 .399-1.375" />
      <path d="M6.003 5.125A3 3 0 0 0 6.401 6.5" />
      <path d="M3.477 10.896a4 4 0 0 1 .585-.396" />
      <path d="M19.938 10.5a4 4 0 0 1 .585.396" />
      <path d="M6 18a4 4 0 0 1-1.967-.516" />
      <path d="M19.967 17.484A4 4 0 0 1 18 18" />
    </svg>
  `,
  book: html`
    <svg viewBox="0 0 24 24">
      <path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20" />
    </svg>
  `,
  loader: html`
    <svg viewBox="0 0 24 24">
      <path d="M12 2v4" />
      <path d="m16.2 7.8 2.9-2.9" />
      <path d="M18 12h4" />
      <path d="m16.2 16.2 2.9 2.9" />
      <path d="M12 18v4" />
      <path d="m4.9 19.1 2.9-2.9" />
      <path d="M2 12h4" />
      <path d="m4.9 4.9 2.9 2.9" />
    </svg>
  `,

  // Tool icons
  wrench: html`
    <svg viewBox="0 0 24 24">
      <path
        d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"
      />
    </svg>
  `,
  fileCode: html`
    <svg viewBox="0 0 24 24">
      <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
      <polyline points="14 2 14 8 20 8" />
      <path d="m10 13-2 2 2 2" />
      <path d="m14 17 2-2-2-2" />
    </svg>
  `,
  edit: html`
    <svg viewBox="0 0 24 24">
      <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
      <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
    </svg>
  `,
  penLine: html`
    <svg viewBox="0 0 24 24">
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
    </svg>
  `,
  paperclip: html`
    <svg viewBox="0 0 24 24">
      <path
        d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48"
      />
    </svg>
  `,
  globe: html`
    <svg viewBox="0 0 24 24">
      <circle cx="12" cy="12" r="10" />
      <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
      <path d="M2 12h20" />
    </svg>
  `,
  image: html`
    <svg viewBox="0 0 24 24">
      <rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
      <circle cx="9" cy="9" r="2" />
      <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
    </svg>
  `,
  smartphone: html`
    <svg viewBox="0 0 24 24">
      <rect width="14" height="20" x="5" y="2" rx="2" ry="2" />
      <path d="M12 18h.01" />
    </svg>
  `,
  channelSwitch: html`
    <svg viewBox="0 0 24 24" fill="none">
      <defs>
        <linearGradient id="channel-grad" x1="2" y1="2" x2="22" y2="22" gradientUnits="userSpaceOnUse">
          <stop offset="0%" stop-color="#06b6d4"/>
          <stop offset="40%" stop-color="#10b981"/>
          <stop offset="100%" stop-color="#f59e0b"/>
        </linearGradient>
      </defs>
      <rect x="2" y="4" width="10" height="8" rx="1.5" stroke="url(#channel-grad)" stroke-width="1.8" fill="rgba(6,182,212,0.1)"/>
      <rect x="12" y="12" width="10" height="8" rx="1.5" stroke="url(#channel-grad)" stroke-width="1.8" fill="rgba(245,158,11,0.1)"/>
      <path d="M15 8l2-2 2 2" stroke="url(#channel-grad)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
      <path d="M9 18l-2-2-2 2" stroke="url(#channel-grad)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" transform="rotate(180 7 17)"/>
      <circle cx="7" cy="8" r="1.2" fill="#06b6d4"/>
      <circle cx="17" cy="16" r="1.2" fill="#f59e0b"/>
    </svg>
  `,
  plug: html`
    <svg viewBox="0 0 24 24">
      <path d="M12 22v-5" />
      <path d="M9 8V2" />
      <path d="M15 8V2" />
      <path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z" />
    </svg>
  `,
  circle: html`
    <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" /></svg>
  `,
  puzzle: html`
    <svg viewBox="0 0 24 24">
      <path
        d="M19.439 7.85c-.049.322.059.648.289.878l1.568 1.568c.47.47.706 1.087.706 1.704s-.235 1.233-.706 1.704l-1.611 1.611a.98.98 0 0 1-.837.276c-.47-.07-.802-.48-.968-.925a2.501 2.501 0 1 0-3.214 3.214c.446.166.855.497.925.968a.979.979 0 0 1-.276.837l-1.61 1.61a2.404 2.404 0 0 1-1.705.707 2.402 2.402 0 0 1-1.704-.706l-1.568-1.568a1.026 1.026 0 0 0-.877-.29c-.493.074-.84.504-1.02.968a2.5 2.5 0 1 1-3.237-3.237c.464-.18.894-.527.967-1.02a1.026 1.026 0 0 0-.289-.877l-1.568-1.568A2.402 2.402 0 0 1 1.998 12c0-.617.236-1.234.706-1.704L4.23 8.77c.24-.24.581-.353.917-.303.515.076.874.54 1.02 1.02a2.5 2.5 0 1 0 3.237-3.237c-.48-.146-.944-.505-1.02-1.02a.98.98 0 0 1 .303-.917l1.526-1.526A2.402 2.402 0 0 1 11.998 2c.617 0 1.234.236 1.704.706l1.568 1.568c.23.23.556.338.877.29.493-.074.84-.504 1.02-.968a2.5 2.5 0 1 1 3.236 3.236c-.464.18-.894.527-.967 1.02Z"
      />
    </svg>
  `,
  shield: html`
    <svg viewBox="0 0 24 24">
      <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" />
    </svg>
  `,
  memoryChip: html`
    <svg viewBox="0 0 24 24">
      <rect x="5" y="4" width="14" height="16" rx="2" />
      <line x1="9" x2="9" y1="4" y2="2" />
      <line x1="15" x2="15" y1="4" y2="2" />
      <line x1="9" x2="9" y1="22" y2="20" />
      <line x1="15" x2="15" y1="22" y2="20" />
      <line x1="5" x2="3" y1="9" y2="9" />
      <line x1="5" x2="3" y1="15" y2="15" />
      <line x1="21" x2="19" y1="9" y2="9" />
      <line x1="21" x2="19" y1="15" y2="15" />
      <line x1="8" x2="16" y1="8" y2="8" />
      <line x1="8" x2="16" y1="12" y2="12" />
      <line x1="8" x2="16" y1="16" y2="16" />
    </svg>
  `,
  mic: html`
    <svg viewBox="0 0 24 24">
      <path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z" />
      <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
      <line x1="12" x2="12" y1="19" y2="22" />
    </svg>
  `,
  mediaPulse: html`
    <svg viewBox="0 0 24 24" fill="none">
      <path
        d="M4.5 12c0-4.142 3.358-7.5 7.5-7.5 2.38 0 4.501 1.108 5.875 2.836"
        stroke="#67e8f9"
        stroke-width="1.6"
        stroke-linecap="round"
      />
      <path
        d="M19.5 12c0 4.142-3.358 7.5-7.5 7.5-2.38 0-4.501-1.108-5.875-2.836"
        stroke="#c084fc"
        stroke-width="1.6"
        stroke-linecap="round"
      />
      <rect x="6.1" y="10.1" width="2.7" height="3.8" rx="1.35" fill="#38bdf8" />
      <rect x="10.65" y="7.1" width="2.7" height="9.8" rx="1.35" fill="#818cf8" />
      <rect x="15.2" y="8.85" width="2.7" height="6.3" rx="1.35" fill="#fb7185" />
      <circle cx="18.85" cy="6.15" r="1.2" fill="#f59e0b" />
    </svg>
  `,
  stop: html`
    <svg viewBox="0 0 24 24">
      <rect x="6" y="6" width="12" height="12" rx="2" />
    </svg>
  `,
  brainMemory: html`
    <svg viewBox="0 0 24 24" fill="none" stroke-width="1.5">
      <path d="M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Z" fill="#f8d7da" stroke="#e57373" />
      <path d="M12 5a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18Z" fill="#f8d7da" stroke="#e57373" />
      <path d="M15 13a4.5 4.5 0 0 1-3-4 4.5 4.5 0 0 1-3 4" stroke="#e57373" />
      <path d="M17.599 6.5a3 3 0 0 0 .399-1.375" stroke="#e57373" />
      <path d="M6.003 5.125A3 3 0 0 0 6.401 6.5" stroke="#e57373" />
      <path d="M3.477 10.896a4 4 0 0 1 .585-.396" stroke="#e57373" />
      <path d="M19.938 10.5a4 4 0 0 1 .585.396" stroke="#e57373" />
      <path d="M6 18a4 4 0 0 1-1.967-.516" stroke="#e57373" />
      <path d="M19.967 17.484A4 4 0 0 1 18 18" stroke="#e57373" />
    </svg>
  `,
  memoryCore: html`
    <svg class="memory-core-icon" viewBox="0 0 24 24" fill="none">
      <path
        class="memory-core-icon__shell"
        d="M12 4.7a3 3 0 0 0-5.96.58 4.13 4.13 0 0 0-2.06 5.68 4.1 4.1 0 0 0 .58 6.14A4.02 4.02 0 0 0 12 18.62Z"
        style="fill: rgba(191, 219, 254, 0.78); stroke: #60a5fa;"
        stroke-width="1.35"
        stroke-linejoin="round"
      />
      <path
        class="memory-core-icon__shell memory-core-icon__shell--right"
        d="M12 4.7a3 3 0 0 1 5.96.58 4.13 4.13 0 0 1 2.06 5.68 4.1 4.1 0 0 1-.58 6.14A4.02 4.02 0 0 1 12 18.62Z"
        style="fill: rgba(96, 165, 250, 0.24); stroke: #2563eb;"
        stroke-width="1.35"
        stroke-linejoin="round"
      />
      <g class="memory-core-icon__orbit">
        <circle
          class="memory-core-icon__ring"
          cx="12"
          cy="12"
          r="4.45"
          style="stroke: #1d4ed8; stroke-opacity: 0.52; fill: none;"
          stroke-width="1.15"
        />
        <circle class="memory-core-icon__node" cx="16.45" cy="12" r="1.1" style="fill: #1d4ed8;" />
      </g>
      <circle class="memory-core-icon__core" cx="12" cy="12" r="2.15" style="fill: #22d3ee;" />
      <path
        class="memory-core-icon__spark"
        d="M12 9.6v4.8"
        style="stroke: #eff6ff;"
        stroke-width="1.2"
        stroke-linecap="round"
      />
      <path
        class="memory-core-icon__spark"
        d="M9.6 12h4.8"
        style="stroke: #eff6ff;"
        stroke-width="1.2"
        stroke-linecap="round"
      />
    </svg>
  `,
  skillBolt: html`
    <svg viewBox="0 0 24 24" fill="none">
      <defs>
        <linearGradient id="skill-grad" x1="0" y1="0" x2="24" y2="24" gradientUnits="userSpaceOnUse">
          <stop offset="0%" stop-color="#38bdf8"/>
          <stop offset="50%" stop-color="#a855f7"/>
          <stop offset="100%" stop-color="#ec4899"/>
        </linearGradient>
      </defs>
      <path d="M4 6.5C4 5.12 5.12 4 6.5 4H8" stroke="url(#skill-grad)" stroke-width="2" stroke-linecap="round"/>
      <path d="M4 17.5C4 18.88 5.12 20 6.5 20H8" stroke="url(#skill-grad)" stroke-width="2" stroke-linecap="round"/>
      <path d="M16 4h1.5C18.88 4 20 5.12 20 6.5" stroke="url(#skill-grad)" stroke-width="2" stroke-linecap="round"/>
      <path d="M16 20h1.5c1.38 0 2.5-1.12 2.5-2.5" stroke="url(#skill-grad)" stroke-width="2" stroke-linecap="round"/>
      <path d="M13 7l-3 5h4l-3 5" stroke="url(#skill-grad)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
      <circle cx="7" cy="10" r="1" fill="#38bdf8"/>
      <circle cx="17" cy="14" r="1" fill="#ec4899"/>
      <circle cx="9" cy="16" r="0.8" fill="#a855f7"/>
      <circle cx="15" cy="8" r="0.8" fill="#a855f7"/>
    </svg>
  `,
  compressContext: html`
    <svg viewBox="0 0 24 24" fill="none">
      <defs>
        <linearGradient id="compress-grad" x1="0" y1="0" x2="24" y2="24" gradientUnits="userSpaceOnUse">
          <stop offset="0%" stop-color="#06b6d4"/>
          <stop offset="50%" stop-color="#8b5cf6"/>
          <stop offset="100%" stop-color="#f43f5e"/>
        </linearGradient>
      </defs>
      <path d="M4 4l5 5M20 4l-5 5M4 20l5-5M20 20l-5-5" stroke="url(#compress-grad)" stroke-width="2" stroke-linecap="round"/>
      <rect x="8" y="8" width="8" height="8" rx="2" stroke="url(#compress-grad)" stroke-width="2" fill="rgba(139,92,246,0.15)"/>
      <circle cx="12" cy="12" r="1.5" fill="url(#compress-grad)"/>
    </svg>
  `,
} as const;

export type IconName = keyof typeof icons;

export function icon(name: IconName): TemplateResult {
  return icons[name];
}

export function isAccentIcon(name: IconName): boolean {
  return [
    "chatSpark",
    "taskOrbit",
    "automationHub",
    "agentSwarm",
    "nodeMesh",
    "navMedia",
    "wizardDashboard",
    "channelBridge",
    "pluginCircuit",
    "mcpBridge",
    "memoryVault",
    "cronOrbit",
    "securityPulse",
    "configSliders",
    "debugRadar",
    "logStack",
    "brandGlobe",
  ].includes(name);
}

export function renderIcon(name: IconName, className = "nav-item__icon"): TemplateResult {
  return html`<span class=${className} aria-hidden="true">${icons[name]}</span>`;
}

// Legacy function for compatibility
export function renderEmojiIcon(
  iconContent: string | TemplateResult,
  className: string,
): TemplateResult {
  return html`<span class=${className} aria-hidden="true">${iconContent}</span>`;
}

export function setEmojiIcon(target: HTMLElement | null, icon: string): void {
  if (!target) {
    return;
  }
  target.textContent = icon;
}
