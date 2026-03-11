import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AppViewState } from "../app-view-state.ts";
import { closeWizardV2, resolveWizardV2Mode, shouldResumeWizardV2Draft } from "./wizard-v2.ts";

function installLocalStorageMock() {
  const store = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store.set(key, value);
    }),
    removeItem: vi.fn((key: string) => {
      store.delete(key);
    }),
    clear: vi.fn(() => {
      store.clear();
    }),
  });
}

describe("closeWizardV2", () => {
  beforeEach(() => {
    installLocalStorageMock();
    localStorage.clear();
    window.history.replaceState({}, "", "/");
  });

  it("exits onboarding mode and clears the onboarding query flag", () => {
    window.history.replaceState({}, "", "/chat?onboarding=true&session=main");
    const replaceState = vi.spyOn(window.history, "replaceState");
    const state = {
      onboarding: true,
      wizardV2Open: true,
      requestUpdate: vi.fn(),
    } as unknown as AppViewState;

    closeWizardV2(state);

    expect(state.onboarding).toBe(false);
    expect(state.wizardV2Open).toBe(false);
    expect(state.requestUpdate).toHaveBeenCalledTimes(1);
    expect(replaceState).toHaveBeenCalledWith({}, "", expect.stringContaining("/chat?session=main"));
  });
});

describe("shouldResumeWizardV2Draft", () => {
  beforeEach(() => {
    installLocalStorageMock();
    localStorage.clear();
  });

  it("ignores stale draft flags during onboarding", () => {
    localStorage.setItem("openacosmi_wizard_v2_resume", "1");

    const result = shouldResumeWizardV2Draft({ onboarding: true });

    expect(result).toBe(false);
  });

  it("allows resume outside onboarding when the draft flag is present", () => {
    localStorage.setItem("openacosmi_wizard_v2_resume", "1");

    const result = shouldResumeWizardV2Draft({ onboarding: false });

    expect(result).toBe(true);
  });

  it("respects explicit resume requests even during onboarding", () => {
    const result = shouldResumeWizardV2Draft({ onboarding: true }, { resumeDraft: true });

    expect(result).toBe(true);
  });
});

describe("resolveWizardV2Mode", () => {
  it("keeps explicit onboarding in setup mode", () => {
    expect(resolveWizardV2Mode({ exists: false }, { onboarding: true })).toBe("setup");
  });

  it("switches to recovery when the current config is invalid", () => {
    expect(
      resolveWizardV2Mode(
        {
          exists: true,
          valid: false,
          issues: [{ path: "models.primary", message: "missing provider" }],
        },
        {},
      ),
    ).toBe("recovery");
  });

  it("switches to recovery when the config is missing but a valid backup exists", () => {
    expect(
      resolveWizardV2Mode(
        { exists: false },
        {
          recoveryBackups: [{ index: 0, path: "/tmp/config.json.bak", size: 123, modTime: "2026-03-11T00:00:00Z", valid: true }],
        },
      ),
    ).toBe("recovery");
  });

  it("stays in setup mode for a clean first run without backups", () => {
    expect(resolveWizardV2Mode({ exists: false }, {})).toBe("setup");
  });

  it("respects a recovery draft when there is no stronger signal", () => {
    expect(resolveWizardV2Mode(null, { draftMode: "recovery" })).toBe("recovery");
  });
});
