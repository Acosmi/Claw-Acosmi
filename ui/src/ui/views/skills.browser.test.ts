import { render } from "lit";
import { describe, expect, it, vi } from "vitest";
import { initLocale } from "../i18n.ts";
import { renderSkills, type SkillsProps } from "./skills.ts";

initLocale("en");

function createProps(overrides: Partial<SkillsProps> = {}): SkillsProps {
  return {
    loading: false,
    report: {
      workspaceDir: "/tmp/workspace",
      managedSkillsDir: "/tmp/managed",
      skills: [
        {
          name: "browser-automation",
          description: "Automate browser flows",
          source: "openacosmi-bundled",
          filePath: "/tmp/workspace/docs/skills/browser-automation/SKILL.md",
          baseDir: "/tmp/workspace/docs/skills/browser-automation",
          skillKey: "browser-automation",
          bundled: true,
          primaryEnv: "BROWSER_API_KEY",
          emoji: "🌐",
          always: false,
          disabled: false,
          blockedByAllowlist: false,
          eligible: true,
          requirements: {
            bins: ["chrome"],
            env: ["BROWSER_API_KEY"],
            config: [],
            os: [],
          },
          missing: {
            bins: [],
            env: [],
            config: [],
            os: [],
          },
          configChecks: [],
          install: [],
          distributed: true,
        },
      ],
    },
    error: null,
    filter: "",
    edits: {},
    busyKey: null,
    messages: {},
    distributeLoading: false,
    distributeResult: null,
    onFilterChange: vi.fn(),
    onRefresh: vi.fn(),
    onToggle: vi.fn(),
    onEdit: vi.fn(),
    onSaveKey: vi.fn(),
    onInstall: vi.fn(),
    onDistribute: vi.fn(),
    requestUpdate: vi.fn(),
    ...overrides,
  };
}

describe("skills view", () => {
  it("opens a detail modal when clicking a skill card", () => {
    const container = document.createElement("div");
    const props = createProps();
    props.requestUpdate = () => render(renderSkills(props), container);
    render(renderSkills(props), container);

    const card = container.querySelector<HTMLElement>(".skill-card");
    expect(card).not.toBeNull();
    card?.click();

    expect(container.textContent).toContain("browser-automation");
    expect(container.textContent).toContain("/tmp/workspace/docs/skills/browser-automation/SKILL.md");
    expect(container.textContent).toContain("Distributed to VFS");
  });
});
