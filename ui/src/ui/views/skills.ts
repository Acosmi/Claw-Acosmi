import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type { SkillMessageMap } from "../controllers/skills.ts";
import type { SkillStatusEntry, SkillStatusReport, SkillToolBindingEntry } from "../types.ts";

type SkillGroup = {
  id: string;
  label: string;
  skills: SkillStatusEntry[];
};

function getSkillSourceGroups(): Array<{ id: string; label: string; sources: string[] }> {
  return [
    { id: "workspace", label: t("agents.skillGroup.workspace"), sources: ["openacosmi-workspace"] },
    { id: "built-in", label: t("agents.skillGroup.builtIn"), sources: ["openacosmi-bundled"] },
    { id: "installed", label: t("agents.skillGroup.installed"), sources: ["openacosmi-managed"] },
    { id: "extra", label: t("agents.skillGroup.extra"), sources: ["openacosmi-extra"] },
  ];
}

function groupSkills(skills: SkillStatusEntry[]): SkillGroup[] {
  const SKILL_SOURCE_GROUPS = getSkillSourceGroups();
  const groups = new Map<string, SkillGroup>();
  for (const def of SKILL_SOURCE_GROUPS) {
    groups.set(def.id, { id: def.id, label: def.label, skills: [] });
  }
  const builtInGroup = SKILL_SOURCE_GROUPS.find((group) => group.id === "built-in");
  const other: SkillGroup = { id: "other", label: t("agents.skillGroup.other"), skills: [] };
  for (const skill of skills) {
    const match = skill.bundled
      ? builtInGroup
      : SKILL_SOURCE_GROUPS.find((group) => group.sources.includes(skill.source));
    if (match) {
      groups.get(match.id)?.skills.push(skill);
    } else {
      other.skills.push(skill);
    }
  }
  const ordered = SKILL_SOURCE_GROUPS.map((group) => groups.get(group.id)).filter(
    (group): group is SkillGroup => Boolean(group && group.skills.length > 0),
  );
  if (other.skills.length > 0) {
    ordered.push(other);
  }
  return ordered;
}

export type SkillsProps = {
  loading: boolean;
  report: SkillStatusReport | null;
  error: string | null;
  filter: string;
  edits: Record<string, string>;
  busyKey: string | null;
  messages: SkillMessageMap;
  distributeLoading: boolean;
  distributeResult: string | null;
  onFilterChange: (next: string) => void;
  onRefresh: () => void;
  onToggle: (skillKey: string, enabled: boolean) => void;
  onEdit: (skillKey: string, value: string) => void;
  onSaveKey: (skillKey: string) => void;
  onInstall: (skillKey: string, name: string, installId: string) => void;
  onDistribute: () => void;
  requestUpdate?: () => void;
};

let activeSkillDetail: SkillStatusEntry | null = null;

function openSkillDetail(skill: SkillStatusEntry, requestUpdate?: () => void) {
  activeSkillDetail = skill;
  requestUpdate?.();
}

function closeSkillDetail(requestUpdate?: () => void) {
  activeSkillDetail = null;
  requestUpdate?.();
}

function translateSkillName(skill: SkillStatusEntry): string {
  const key = `skillName.${skill.name.replace(/[^a-zA-Z0-9]/g, '')}`;
  const translated = t(key);
  return translated === key ? skill.name : translated;
}

function translateSkillDesc(skill: SkillStatusEntry): string {
  const key = `skillDesc.${skill.name.replace(/[^a-zA-Z0-9]/g, '')}`;
  const translated = t(key);
  return translated === key ? skill.description : translated;
}

export function renderSkills(props: SkillsProps) {
  const skills = props.report?.skills ?? [];
  const toolBindings = props.report?.toolBindings ?? [];
  const filter = props.filter.trim().toLowerCase();
  const filtered = filter
    ? skills.filter((skill) =>
      [skill.name, skill.description, skill.source].join(" ").toLowerCase().includes(filter),
    )
    : skills;
  const groups = groupSkills(filtered);

  return html`
    <section class="card">
      <div class="row" style="justify-content: space-between;">
        <div>
          <div class="card-title">${t("skills.title")}</div>
          <div class="card-sub">${t("skills.sub")}</div>
        </div>
        <div class="row" style="gap: 8px;">
          <button class="btn" ?disabled=${props.loading} @click=${props.onRefresh}>
            ${props.loading ? t("common.loading") : t("common.refresh")}
          </button>
          <button
            class="btn primary"
            ?disabled=${props.distributeLoading || props.loading}
            @click=${props.onDistribute}
          >
            ${props.distributeLoading ? "分级中..." : "一键 VFS 分级"}
          </button>
        </div>
      </div>

      <div class="filters" style="margin-top: 14px;">
        <label class="field" style="flex: 1;">
          <span>${t("agents.skills.filter")}</span>
          <input
            .value=${props.filter}
            @input=${(e: Event) => props.onFilterChange((e.target as HTMLInputElement).value)}
            placeholder=${t("agents.skills.searchPlaceholder")}
          />
        </label>
        <div class="muted">${t("agents.skills.shown", { count: filtered.length })}</div>
      </div>

      ${props.error
      ? html`<div class="callout danger" style="margin-top: 12px;">${props.error}</div>`
      : nothing
    }

      ${props.distributeResult
      ? html`<div class="callout" style="margin-top: 12px; background: var(--success-bg, #e6f9f0); color: var(--success-color, #0a7f5a);">${props.distributeResult}</div>`
      : nothing
    }

      ${toolBindings.length > 0 ? renderToolBindings(toolBindings) : nothing}

      ${filtered.length === 0
      ? html`
              <div class="muted" style="margin-top: 16px">${t("agents.skills.noSkills")}</div>
            `
      : html`
            <div class="agent-skills-groups" style="margin-top: 16px;">
              ${groups.map((group) => {
        return html`
                  <details class="agent-skills-group" open>
                    <summary class="agent-skills-header">
                      <span>${group.label}</span>
                      <span class="muted">${group.skills.length}</span>
                    </summary>
                    <div class="list skills-grid">
                      ${group.skills.map((skill) => renderSkill(skill, props))}
                    </div>
                  </details>
                `;
      })}
            </div>
          `
    }
      ${renderSkillDetailModal(props)}
    </section>
  `;
}

function renderToolBindings(toolBindings: SkillToolBindingEntry[]) {
  return html`
    <section class="card" style="margin-top: 16px; background: rgba(15, 23, 42, 0.03);">
      <div class="card-title">${t("skills.bindings.title")}</div>
      <div class="card-sub">${t("skills.bindings.sub")}</div>
      <div class="list" style="margin-top: 12px;">
        ${toolBindings.map((binding) => html`
          <div class="list-item">
            <div class="list-main">
              <div class="list-title"><code>${binding.toolName}</code></div>
              <div class="chip-row" style="margin-top: 8px;">
                ${binding.skills.map((skill) => html`<span class="chip">${skill}</span>`)}
              </div>
              ${binding.guidance
              ? html`<div class="muted" style="margin-top: 8px;">${binding.guidance}</div>`
              : nothing}
            </div>
          </div>
        `)}
      </div>
    </section>
  `;
}

function renderSkill(skill: SkillStatusEntry, props: SkillsProps) {
  const busy = props.busyKey === skill.skillKey;
  const apiKey = props.edits[skill.skillKey] ?? "";
  const message = props.messages[skill.skillKey] ?? null;
  const canInstall = skill.install.length > 0 && skill.missing.bins.length > 0;
  const missing = [
    ...skill.missing.bins.map((b) => `bin:${b}`),
    ...skill.missing.env.map((e) => `env:${e}`),
    ...skill.missing.config.map((c) => `config:${c}`),
    ...skill.missing.os.map((o) => `os:${o}`),
  ];
  const reasons: string[] = [];
  if (skill.disabled) {
    reasons.push(t("agents.skill.disabled"));
  }
  if (skill.blockedByAllowlist) {
    reasons.push(t("agents.skill.blockedByAllowlist"));
  }
  const isTextIcon = skill.emoji && /[a-zA-Z]{2,}/.test(skill.emoji);

  return html`
    <div
      class="skill-card"
      role="button"
      tabindex="0"
      @click=${() => openSkillDetail(skill, props.requestUpdate)}
      @keydown=${(event: KeyboardEvent) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openSkillDetail(skill, props.requestUpdate);
        }
      }}
    >
      <div class="skill-card-icon" style="${isTextIcon ? 'font-size: 10px; font-weight: 600; line-height: 1.2; text-align: center; word-break: break-word; padding: 2px;' : ''}">
        ${skill.emoji ? skill.emoji : html`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"></path><path d="M2 17l10 5 10-5"></path><path d="M2 12l10 5 10-5"></path></svg>`}
      </div>
      
      <div class="skill-card-content">
        <div class="skill-card-title">${translateSkillName(skill)}</div>
        <div class="skill-card-desc">${translateSkillDesc(skill)}</div>
      </div>
      
      <div class="skill-card-actions">
        ${canInstall
      ? html`<button
              class="skill-btn-install"
              title="${skill.install[0].label}"
              ?disabled=${busy}
              @click=${(event: Event) => {
                event.stopPropagation();
                props.onInstall(skill.skillKey, skill.name, skill.install[0].id);
              }}
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>
            </button>`
      : nothing
    }
        
        <label class="skill-toggle" title="${skill.disabled ? t('skills.enable') : t('skills.disable')}" @click=${(event: Event) => event.stopPropagation()}>
          <input 
            type="checkbox" 
            ?checked=${!skill.disabled} 
            ?disabled=${busy}
            @click=${(event: Event) => event.stopPropagation()}
            @change=${() => props.onToggle(skill.skillKey, skill.disabled)}
          >
          <span class="skill-toggle-slider"></span>
        </label>
      </div>

      ${skill.primaryEnv || message || missing.length > 0 || reasons.length > 0
      ? html`
          <div class="skill-card-meta" style="flex-basis: 100%; margin-top: 12px; margin-left: -64px;">
            ${missing.length > 0
          ? html`
                  <div class="muted" style="font-size: 12px; margin-bottom: 6px;">
                    ${t("agents.skill.missing", { list: missing.join(", ") })}
                  </div>
                `
          : nothing
        }
            ${reasons.length > 0
          ? html`
                  <div class="muted" style="font-size: 12px; margin-bottom: 6px;">
                    ${t("agents.skill.reason", { list: reasons.join(", ") })}
                  </div>
                `
          : nothing
        }
            ${message
          ? html`<div
                  class="muted"
                  style="font-size: 12px; margin-bottom: 8px; color: ${message.kind === "error"
              ? "var(--danger-color, #d14343)"
              : "var(--success-color, #0a7f5a)"
            };"
                >
                  ${message.message}
                </div>`
          : nothing
        }
            ${skill.primaryEnv
          ? html`
                  <div class="row" style="gap: 8px; align-items: center;">
                    <div class="field" style="flex: 1; margin: 0;">
                      <input
                        type="password"
                        placeholder="${t('skills.apiKey')}"
                        .value=${apiKey}
                        @click=${(event: Event) => event.stopPropagation()}
                        @input=${(e: Event) =>
              props.onEdit(skill.skillKey, (e.target as HTMLInputElement).value)}
                        style="padding: 4px 8px; font-size: 12px;"
                      />
                    </div>
                    <button
                      class="btn primary"
                      style="padding: 4px 10px; font-size: 12px;"
                      ?disabled=${busy}
                      @click=${(event: Event) => {
                        event.stopPropagation();
                        props.onSaveKey(skill.skillKey);
                      }}
                    >
                      ${t("skills.saveKey")}
                    </button>
                  </div>
                `
          : nothing
        }
          </div>
        `
      : nothing
    }
    </div>
  `;
}

function renderSkillDetailModal(props: SkillsProps) {
  const skill = activeSkillDetail;
  if (!skill) {
    return nothing;
  }

  const requirementList = [
    ...skill.requirements.bins.map((entry) => `bin:${entry}`),
    ...skill.requirements.env.map((entry) => `env:${entry}`),
    ...skill.requirements.config.map((entry) => `config:${entry}`),
    ...skill.requirements.os.map((entry) => `os:${entry}`),
  ];
  const missingList = [
    ...skill.missing.bins.map((entry) => `bin:${entry}`),
    ...skill.missing.env.map((entry) => `env:${entry}`),
    ...skill.missing.config.map((entry) => `config:${entry}`),
    ...skill.missing.os.map((entry) => `os:${entry}`),
  ];
  const issues = [
    skill.disabled ? t("agents.skill.disabled") : "",
    skill.blockedByAllowlist ? t("agents.skill.blockedByAllowlist") : "",
    !skill.eligible ? t("skills.detail.notEligible") : "",
  ].filter(Boolean);

  return html`
    <div
      class="modal-overlay"
      style="position: fixed; inset: 0; background: rgba(15, 23, 42, 0.45); backdrop-filter: blur(6px); display: flex; align-items: center; justify-content: center; padding: 20px; z-index: 80;"
      @click=${(event: Event) => {
        if (event.target === event.currentTarget) {
          closeSkillDetail(props.requestUpdate);
        }
      }}
    >
      <div class="card" style="width: min(760px, 100%); max-height: 88vh; overflow: auto; padding: 20px; display: flex; flex-direction: column; gap: 16px;">
        <div class="row" style="justify-content: space-between; align-items: flex-start; gap: 16px;">
          <div>
            <div class="card-title" style="margin-bottom: 4px;">${translateSkillName(skill)}</div>
            <div class="card-sub">${skill.skillKey}</div>
          </div>
          <button class="btn btn-sm" @click=${() => closeSkillDetail(props.requestUpdate)}>${t("wizard.close")}</button>
        </div>

        <div style="font-size: 14px; line-height: 1.7;">${translateSkillDesc(skill)}</div>

        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px;">
          <div>
            <div style="font-size: 12px; opacity: 0.55; margin-bottom: 4px;">${t("skills.detail.source")}</div>
            <div>${skill.source}</div>
          </div>
          <div>
            <div style="font-size: 12px; opacity: 0.55; margin-bottom: 4px;">${t("skills.detail.primaryEnv")}</div>
            <div>${skill.primaryEnv || t("skills.detail.none")}</div>
          </div>
          <div>
            <div style="font-size: 12px; opacity: 0.55; margin-bottom: 4px;">${t("skills.detail.distribution")}</div>
            <div>${skill.distributed ? t("skills.detail.distributed") : t("skills.detail.notDistributed")}</div>
          </div>
        </div>

        <div>
          <div style="font-size: 12px; opacity: 0.55; margin-bottom: 4px;">${t("skills.detail.filePath")}</div>
          <code style="font-size: 12px; white-space: pre-wrap; word-break: break-word;">${skill.filePath}</code>
        </div>

        <div>
          <div style="font-size: 12px; opacity: 0.55; margin-bottom: 4px;">${t("skills.detail.baseDir")}</div>
          <code style="font-size: 12px; white-space: pre-wrap; word-break: break-word;">${skill.baseDir}</code>
        </div>

        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px;">
          <div>
            <div style="font-size: 12px; opacity: 0.55; margin-bottom: 8px;">${t("skills.detail.requirements")}</div>
            <div style="display: flex; gap: 8px; flex-wrap: wrap;">
              ${(requirementList.length > 0 ? requirementList : [t("skills.detail.none")]).map((entry) => html`<span class="chip">${entry}</span>`)}
            </div>
          </div>
          <div>
            <div style="font-size: 12px; opacity: 0.55; margin-bottom: 8px;">${t("skills.detail.missing")}</div>
            <div style="display: flex; gap: 8px; flex-wrap: wrap;">
              ${(missingList.length > 0 ? missingList : [t("skills.detail.none")]).map((entry) => html`<span class="chip">${entry}</span>`)}
            </div>
          </div>
        </div>

        <div>
          <div style="font-size: 12px; opacity: 0.55; margin-bottom: 8px;">${t("skills.detail.status")}</div>
          <div style="display: flex; gap: 8px; flex-wrap: wrap;">
            ${(issues.length > 0 ? issues : [t("skills.detail.ready")]).map((entry) => html`<span class="chip">${entry}</span>`)}
          </div>
        </div>
      </div>
    </div>
  `;
}
