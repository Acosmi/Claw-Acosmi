import { html, nothing } from "lit";
import { keyed } from "lit/directives/keyed.js";
import type { AppViewState } from "../app-view-state.ts";
import type { ConfigSnapshot } from "../types.ts";
import { needsInitialSetup } from "../controllers/config.ts";
import { normalizeBasePath } from "../navigation.ts";
import { type WizardProvider, mergeWithUI, getFallbackProviders } from "./wizard-v2-providers.ts";

// 动态 provider 列表（会话级缓存，首次打开时从后端获取）
let providers: WizardProvider[] = [];
type WizardSkillGroupDef = {
   key: string;
   defaultOn: boolean;
   tools: string[];
};
let skillGroups: WizardSkillGroupDef[] = [];
type WizardMode = "setup" | "recovery";
type WizardStepKey =
   | "welcome"
   | "primary"
   | "fallback"
   | "skills"
   | "channels"
   | "subagents"
   | "memory"
   | "security"
   | "done";
type WizardStepDef = {
   key: WizardStepKey;
   label: string;
};
type WizardRecoveryBackupEntry = {
   index: number;
   path: string;
   size: number;
   modTime: string;
   valid: boolean;
};
type WizardCompletionKind = "setup" | "recovery-reconfigure" | "recovery-restore";

// ─── Constants & Types ───

const SETUP_STEPS: WizardStepDef[] = [
   { key: "welcome", label: "欢迎" },
   { key: "primary", label: "主模型" },
   { key: "fallback", label: "备用模型" },
   { key: "skills", label: "技能" },
   { key: "channels", label: "频道" },
   { key: "subagents", label: "子智能体" },
   { key: "memory", label: "记忆系统" },
   { key: "security", label: "安全与设置" },
   { key: "done", label: "完成" },
];

const RECOVERY_STEPS: WizardStepDef[] = [
   { key: "welcome", label: "恢复" },
   { key: "primary", label: "最小配置" },
   { key: "done", label: "完成" },
];

// Local prototype state (isolated from main app state for safety)
let stepIndex = 0;
let primaryConfig: Record<string, string> = {};
let fallbackConfig: Record<string, string> = {};
let wizardMode: WizardMode = "setup";
let recoveryBackups: WizardRecoveryBackupEntry[] = [];
let recoveryBackupsLoading = false;
let recoveryRestoreError: string | null = null;
let openSecuritySettingsAfterFinish = false;
let completionKind: WizardCompletionKind = "setup";

const DEFAULT_MEMORY_CONFIG = {
   enableVector: false,
   hostingType: "local",
   apiEndpoint: "",
   llmProvider: "",
   llmModel: "",
   llmApiKey: "",
   llmBaseUrl: "",
};
const DEFAULT_SECURITY_LEVEL = "sandboxed";

// Mock states for UI interactivity
let securityAck = false;
let selectedSkills: Record<string, boolean> = {};
let channelConfig: any = {
   feishu: { appId: "", appSecret: "" },
   wecom: { appId: "", appSecret: "" },
   dingtalk: { appKey: "", appSecret: "" },
   telegram: { botToken: "" }
};
let subAgentsConfig: Record<string, { enabled: boolean }> = {};
let memoryConfig = { ...DEFAULT_MEMORY_CONFIG };
let securityLevelConfig = DEFAULT_SECURITY_LEVEL;
let isRestarting = false;
let restartProgress = 0;
let stepTransitioning = false;
let providerSelections: Record<string, { model: string, authMode: string }> = {}; // Store selected model/authMode per provider
let customBaseUrls: Record<string, string> = {}; // Store base URLs for custom providers
let pendingOauthSelection: string | null = null; // Track which provider is waiting for app selection
// Device Code OAuth state
let deviceCodeState: Record<string, { userCode: string, verificationUri: string, sessionId: string, polling: boolean, error?: string }> = {};

const FALLBACK_SKILL_GROUPS: WizardSkillGroupDef[] = [
   { key: "fs", defaultOn: true, tools: ["read", "write", "list_dir", "apply_patch"] },
   { key: "runtime", defaultOn: true, tools: ["exec"] },
   { key: "ui", defaultOn: true, tools: ["canvas"] },
   { key: "web", defaultOn: true, tools: ["browser", "web_search", "web_fetch"] },
   { key: "memory", defaultOn: true, tools: ["memory_search", "memory_get"] },
   { key: "sessions", defaultOn: true, tools: ["sessions_list", "sessions_history", "sessions_send", "sessions_spawn", "session_status"] },
   { key: "ai", defaultOn: true, tools: ["image"] },
   { key: "system", defaultOn: true, tools: ["nodes", "cron", "gateway"] },
   { key: "messaging", defaultOn: true, tools: ["message"] },
];

const SKILL_GROUP_META: Record<string, { name: string; icon: string; desc: string }> = {
   fs: { name: "文件系统", icon: "📁", desc: "读取、写入、列目录与补丁应用" },
   runtime: { name: "命令执行", icon: "⚡", desc: "终端命令与运行时控制" },
   ui: { name: "画布", icon: "🖼️", desc: "画布展示与界面交互" },
   web: { name: "网页与浏览器", icon: "🌐", desc: "搜索、抓取与浏览器自动化" },
   memory: { name: "记忆调用", icon: "🧠", desc: "长期记忆搜索与读取" },
   sessions: { name: "会话管理", icon: "💬", desc: "查看、发送和派生会话" },
   ai: { name: "多模态 AI", icon: "🧩", desc: "图像理解等多模态能力" },
   system: { name: "系统管理", icon: "⚙️", desc: "节点、定时任务与网关控制" },
   messaging: { name: "消息推送", icon: "📤", desc: "主动发送消息到外部频道" },
};

// ─── Controller functions ───

const DRAFT_KEY = "openacosmi_wizard_v2_draft";
const DRAFT_RESUME_KEY = "openacosmi_wizard_v2_resume";

function normalizeWizardSecurityLevel(value: unknown): string {
   switch (value) {
      case "deny":
      case "allowlist":
      case "sandboxed":
      case "full":
         return value;
      default:
         return DEFAULT_SECURITY_LEVEL;
   }
}

function publicAssetUrl(basePath: string, assetPath: string): string {
   const base = normalizeBasePath(basePath);
   return base ? `${base}${assetPath}` : assetPath;
}

function getWizardSteps(): WizardStepDef[] {
   return wizardMode === "recovery" ? RECOVERY_STEPS : SETUP_STEPS;
}

function clampWizardStepIndex(): void {
   const steps = getWizardSteps();
   stepIndex = Math.min(stepIndex, Math.max(steps.length - 1, 0));
}

function clearWizardDraftStorage(): void {
   try {
      localStorage.removeItem(DRAFT_KEY);
      localStorage.removeItem(DRAFT_RESUME_KEY);
   } catch { }
}

function hasValidRecoveryBackup(entries: WizardRecoveryBackupEntry[] | null | undefined): boolean {
   return Array.isArray(entries) && entries.some((entry) => entry.valid);
}

function hasRecoveryIssues(snapshot: Pick<ConfigSnapshot, "valid" | "issues"> | null | undefined): boolean {
   if (snapshot?.valid === false) {
      return true;
   }
   return Array.isArray(snapshot?.issues) && snapshot.issues.length > 0;
}

export function resolveWizardV2Mode(
   snapshot: ConfigSnapshot | null | undefined,
   opts?: {
      onboarding?: boolean;
      recoveryBackups?: WizardRecoveryBackupEntry[] | null;
      draftMode?: WizardMode | null;
   },
): WizardMode {
   if (opts?.onboarding) {
      return "setup";
   }
   if (hasRecoveryIssues(snapshot)) {
      return "recovery";
   }
   if (snapshot?.exists === false) {
      return hasValidRecoveryBackup(opts?.recoveryBackups) ? "recovery" : "setup";
   }
   if (needsInitialSetup(snapshot) && hasValidRecoveryBackup(opts?.recoveryBackups)) {
      return "recovery";
   }
   return opts?.draftMode === "recovery" ? "recovery" : "setup";
}

function describeRecoveryHints(snapshot: ConfigSnapshot | null | undefined): string[] {
   const hints: string[] = [];
   if (snapshot?.exists === false) {
      hints.push("未检测到当前配置文件。");
   }
   if (snapshot?.valid === false) {
      hints.push("当前配置无法解析或内容已损坏。");
   }
   const issues = Array.isArray(snapshot?.issues) ? snapshot.issues : [];
   for (const issue of issues.slice(0, 2)) {
      if (!issue?.message) {
         continue;
      }
      const path = typeof issue.path === "string" && issue.path.trim().length > 0 ? `${issue.path}: ` : "";
      hints.push(`${path}${issue.message}`);
   }
   if (hints.length === 0 && hasValidRecoveryBackup(recoveryBackups)) {
      hints.push("检测到历史可用备份，可直接回滚到最近一次可用配置。");
   }
   if (hints.length === 0) {
      hints.push("当前配置不可继续使用，建议先恢复到最小可用状态。");
   }
   return hints;
}

function resolveWizardSkillGroups(): WizardSkillGroupDef[] {
   return skillGroups.length > 0 ? skillGroups : FALLBACK_SKILL_GROUPS;
}

function defaultSelectedSkillsFromGroups(): Record<string, boolean> {
   const next: Record<string, boolean> = {};
   for (const group of resolveWizardSkillGroups()) {
      next[group.key] = group.defaultOn;
   }
   return next;
}

function mergeSelectedSkillsWithGroups(existing: Record<string, boolean> | null | undefined): Record<string, boolean> {
   const merged = defaultSelectedSkillsFromGroups();
   if (!existing) {
      return merged;
   }
   for (const group of resolveWizardSkillGroups()) {
      if (typeof existing[group.key] === "boolean") {
         merged[group.key] = existing[group.key];
      }
   }
   return merged;
}

export function shouldResumeWizardV2Draft(
   state: Pick<AppViewState, "onboarding">,
   opts?: { resumeDraft?: boolean },
): boolean {
   if (opts?.resumeDraft === true) {
      return true;
   }
   if (state.onboarding) {
      return false;
   }
   return localStorage.getItem(DRAFT_RESUME_KEY) === "1";
}

async function ensureWizardSkillGroups(state: AppViewState): Promise<void> {
   if (skillGroups.length > 0 || !state.client) {
      return;
   }
   try {
      const res = await state.client.request<{ groups?: WizardSkillGroupDef[] }>("wizard.v2.skill-groups.list", {});
      const groups = Array.isArray(res?.groups) ? res.groups : [];
      skillGroups = groups.length > 0 ? groups : FALLBACK_SKILL_GROUPS;
   } catch {
      skillGroups = FALLBACK_SKILL_GROUPS;
   }
}

async function loadWizardRecoveryBackups(state: AppViewState): Promise<void> {
   recoveryBackupsLoading = false;
   recoveryRestoreError = null;
   recoveryBackups = [];
   if (!state.client) {
      return;
   }
   recoveryBackupsLoading = true;
   try {
      const res = await state.client.request<{ backups?: WizardRecoveryBackupEntry[] }>("system.backup.list", {});
      recoveryBackups = Array.isArray(res?.backups) ? res.backups : [];
   } catch {
      recoveryBackups = [];
   } finally {
      recoveryBackupsLoading = false;
   }
}

function finalizeWizardRestart(state: AppViewState): void {
   const interval = setInterval(() => {
      restartProgress += Math.floor(Math.random() * 15) + 5;
      if (restartProgress >= 100) {
         restartProgress = 100;
         isRestarting = false;
         stepTransitioning = false;
         clearInterval(interval);
         clearWizardDraftStorage();
      }
      state.requestUpdate();
   }, 300);
}

async function runWizardRestartSequence(
   state: AppViewState,
   task: () => Promise<unknown>,
): Promise<void> {
   isRestarting = true;
   restartProgress = 0;
   state.requestUpdate();

   try {
      restartProgress = 20;
      state.requestUpdate();

      const timeoutPromise = new Promise((resolve) =>
         setTimeout(() => resolve({ ok: true, timeout: true }), 5000),
      );
      await Promise.race([
         task().catch(() => ({ ok: true, disconnected: true })),
         timeoutPromise,
      ]);

      restartProgress = 60;
      state.requestUpdate();
   } catch (err) {
      console.warn("wizard restart flow error:", err);
      restartProgress = 60;
      state.requestUpdate();
   }

   finalizeWizardRestart(state);
}

function exitOnboardingMode(state: AppViewState): void {
   state.onboarding = false;
   if (typeof window === "undefined") {
      return;
   }
   const url = new URL(window.location.href);
   if (!url.searchParams.has("onboarding")) {
      return;
   }
   url.searchParams.delete("onboarding");
   window.history.replaceState({}, "", url.toString());
}

export function saveWizardV2Draft(): void {
   const draft = {
      wizardMode,
      stepIndex, primaryConfig, fallbackConfig, securityAck,
      selectedSkills, channelConfig, subAgentsConfig, memoryConfig,
      securityLevelConfig, providerSelections, customBaseUrls,
      openSecuritySettingsAfterFinish,
   };
   localStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
}

function resetWizardV2State(): void {
   stepTransitioning = false;
   stepIndex = 0;
   wizardMode = "setup";
   primaryConfig = {};
   fallbackConfig = {};
   securityAck = false;
   selectedSkills = defaultSelectedSkillsFromGroups();
   channelConfig = { feishu: { appId: "", appSecret: "" }, wecom: { appId: "", appSecret: "" }, dingtalk: { appKey: "", appSecret: "" }, telegram: { botToken: "" } };
   subAgentsConfig = {};
   memoryConfig = { ...DEFAULT_MEMORY_CONFIG };
   securityLevelConfig = DEFAULT_SECURITY_LEVEL;
   providerSelections = {};
   customBaseUrls = {};
   pendingOauthSelection = null;
   deviceCodeState = {};
   recoveryBackups = [];
   recoveryBackupsLoading = false;
   recoveryRestoreError = null;
   openSecuritySettingsAfterFinish = false;
   completionKind = "setup";
}

function ensureProviderSelections(): void {
   providers.forEach(p => {
      if (!providerSelections[p.id]) {
         providerSelections[p.id] = { model: p.models[0]?.id ?? "", authMode: p.authModes[0] };
      }
   });
}

export async function startWizardV2(state: AppViewState, opts?: { resumeDraft?: boolean }): Promise<void> {
   let draftMode: WizardMode | null = null;

   // 从后端动态获取 provider 目录（会话级缓存）
   if (providers.length === 0) {
      try {
         const res = await state.client!.request<any>("wizard.v2.providers.list", {});
         providers = res?.providers?.length ? mergeWithUI(res.providers, state.basePath ?? "") : getFallbackProviders(state.basePath ?? "");
      } catch {
         providers = getFallbackProviders(state.basePath ?? "");
      }
   }
   await ensureWizardSkillGroups(state);

   // 默认全新启动向导，避免旧密钥/配置自动回填。
   // 仅在显式 resumeDraft=true 时恢复草稿。
   const shouldResumeDraft = shouldResumeWizardV2Draft(state, opts);
   const saved = shouldResumeDraft ? localStorage.getItem(DRAFT_KEY) : null;
   if (saved) {
      try {
         const draft = JSON.parse(saved);
         stepIndex = draft.stepIndex || 0;
         draftMode = draft.wizardMode === "recovery" ? "recovery" : "setup";
         primaryConfig = draft.primaryConfig || {};
         fallbackConfig = draft.fallbackConfig || {};
         securityAck = draft.securityAck ?? false;
         selectedSkills = mergeSelectedSkillsWithGroups(draft.selectedSkills);
         channelConfig = draft.channelConfig || { feishu: { appId: "", appSecret: "" }, wecom: { appId: "", appSecret: "" }, dingtalk: { appKey: "", appSecret: "" }, telegram: { botToken: "" } };
         subAgentsConfig = draft.subAgentsConfig || {};
         memoryConfig = { ...DEFAULT_MEMORY_CONFIG, ...(draft.memoryConfig || {}) };
         securityLevelConfig = normalizeWizardSecurityLevel(draft.securityLevelConfig);
         providerSelections = draft.providerSelections || {};
         customBaseUrls = draft.customBaseUrls || {};
         openSecuritySettingsAfterFinish = draft.openSecuritySettingsAfterFinish === true;
      } catch (e) {
         console.error("Failed to parse wizard draft", e);
         resetWizardV2State();
         ensureProviderSelections();
         clearWizardDraftStorage();
      }
   } else {
      resetWizardV2State();
      ensureProviderSelections();
      clearWizardDraftStorage();
   }

    await loadWizardRecoveryBackups(state);

   wizardMode = resolveWizardV2Mode(state.configSnapshot, {
      onboarding: state.onboarding,
      recoveryBackups,
      draftMode,
   });
   completionKind = wizardMode === "recovery" ? "recovery-reconfigure" : "setup";
   if (wizardMode === "recovery") {
      securityAck = true;
      openSecuritySettingsAfterFinish = false;
   }
   clampWizardStepIndex();

   isRestarting = false;
   restartProgress = 0;

   // Ensure all providers have a selection entry, preserving draft if it exists
   ensureProviderSelections();

   state.wizardV2Open = true;
   state.requestUpdate();
}

export function closeWizardV2(state: AppViewState, saveDraft: boolean = false): void {
   stepTransitioning = false;
   if (saveDraft) {
      saveWizardV2Draft();
      try { localStorage.setItem(DRAFT_RESUME_KEY, "1"); } catch { }
   } else {
      clearWizardDraftStorage();
   }
   exitOnboardingMode(state);
   state.wizardV2Open = false;
   state.requestUpdate();
}

async function nextStep(state: AppViewState) {
   const steps = getWizardSteps();
   if (stepTransitioning) {
      return;
   }
   stepTransitioning = true;
   if (stepIndex === steps.length - 2) {
      // 进入最后"完成"步骤前，先将配置发送到后端持久化
      stepIndex++;
      state.requestUpdate();

      completionKind = wizardMode === "recovery" ? "recovery-reconfigure" : "setup";

      // 构建 WizardV2Payload — 与后端 WizardV2Payload 结构对齐
      const payload = {
         primaryConfig,
         fallbackConfig,
         providerSelections,
         customBaseUrls,
         securityAck: wizardMode === "recovery" ? true : securityAck,
         selectedSkills,
         channelConfig,
         subAgentsConfig,
         memoryConfig,
         securityLevelConfig
      };

      await runWizardRestartSequence(state, async () => {
         await state.client!.request<any>("wizard.v2.apply", payload);
      });
   } else if (stepIndex < steps.length - 1) {
      stepIndex++;
      state.requestUpdate();
      window.setTimeout(() => {
         stepTransitioning = false;
      }, 220);
      return;
   }
   stepTransitioning = false;
}

async function restoreLatestRecoveryBackup(state: AppViewState): Promise<void> {
   if (stepTransitioning || recoveryBackupsLoading || !state.client) {
      return;
   }
   const backup = recoveryBackups.find((entry) => entry.valid);
   if (!backup) {
      recoveryRestoreError = "未找到可直接恢复的有效备份，请继续手动补齐最小配置。";
      state.requestUpdate();
      return;
   }

   stepTransitioning = true;
   recoveryRestoreError = null;
   completionKind = "recovery-restore";
   stepIndex = getWizardSteps().length - 1;
   state.requestUpdate();

   await runWizardRestartSequence(state, async () => {
      await state.client!.request("system.backup.restore", { index: backup.index });
      await state.client!.request("system.restart", { reason: "wizard.recovery.restore", delayMs: 100 });
   });
}

function prevStep(state: AppViewState) {
   if (stepIndex > 0) {
      stepIndex--;
      state.requestUpdate();
   } else {
      closeWizardV2(state);
   }
}

// Bug#11: 按 provider 返回推荐记忆提取模型
function getDefaultMemoryModel(provider: string): string {
   switch (provider) {
      case "deepseek": return "deepseek-chat";
      case "openai": return "gpt-4o-mini";
      case "anthropic": return "claude-haiku-4-5-20251001";
      case "ollama": return "llama3.2";
      default: return "";
   }
}

function getDefaultMemoryBaseUrl(provider: string): string {
   switch (provider) {
      case "deepseek": return "https://api.deepseek.com";
      case "openai": return "https://api.openai.com/v1";
      case "anthropic": return "https://api.anthropic.com";
      case "ollama": return "http://localhost:11434";
      default: return "";
   }
}

// ─── Renderers ───

function renderProviders(state: AppViewState, configMap: Record<string, string>, isRequired: boolean) {
   return html`
    <div class="wizard-v2-providers-grid">
      ${providers.map((p) => {
      const value = configMap[p.id] || "";
      const selection = providerSelections[p.id];
      const isSelected = value.length > 0;
      return html`
          <div class="wizard-v2-provider-card ${isSelected ? 'wizard-v2-provider-card-selected' : ''}">
            ${isSelected ? html`<div class="wizard-v2-provider-selected-badge"></div>` : nothing}
            <div class="wizard-v2-provider-header">
                <div class="wizard-v2-provider-icon" style="background: ${p.bg}; color: ${p.color};">${p.icon}</div>
                <div class="wizard-v2-provider-info">
                  <div class="wizard-v2-provider-name">${p.name}</div>
                  <div class="wizard-v2-provider-desc">${p.desc}</div>
                </div>
            </div>
            
            <div class="wizard-v2-provider-input-group" style="margin-bottom: 12px;">
                <label>${p.requiresBaseUrl ? "输入自定义模型 ID" : "选择模型版本"}</label>
                ${p.requiresBaseUrl ? html`
                   <input type="text" class="wizard-v2-input" placeholder="例如: qwen-max, minicpm-v" .value=${selection.model === 'custom' || selection.model.includes('自定义') ? '' : selection.model} @input=${(e: Event) => {
               providerSelections[p.id].model = (e.target as HTMLInputElement).value;
               state.requestUpdate();
            }} />
                ` : html`
                   <select class="wizard-v2-input" .value=${selection.model} @change=${(e: Event) => {
               providerSelections[p.id].model = (e.target as HTMLSelectElement).value;
               state.requestUpdate();
            }}>
                      ${p.models.map(m => html`<option value="${m.id}">${m.name}</option>`)}
                   </select>
                `}
            </div>
            
            ${p.customBaseUrlAllowed ? html`
                <div class="wizard-v2-provider-input-group" style="margin-bottom: 12px;">
                   <label>Base URL 代理地址</label>
                   <input type="text" class="wizard-v2-input" placeholder="例如: https://api.openrouter.ai/v1" .value=${customBaseUrls[p.id] || ""} @input=${(e: Event) => {
               customBaseUrls[p.id] = (e.target as HTMLInputElement).value;
               state.requestUpdate();
            }} />
                </div>
            ` : nothing}
            
            ${p.authModes.length > 1 ? html`
              <div class="wizard-v2-provider-input-group" style="margin-bottom: 12px; flex-direction: row; gap: 16px; flex-wrap: wrap;">
                  ${p.authModes.map(mode => html`
                     <label style="display:flex; align-items:center; gap:4px; font-weight:normal; cursor:pointer;">
                        <input type="radio" name="auth-${p.id}" .checked=${selection.authMode === mode} @change=${() => { providerSelections[p.id].authMode = mode; state.requestUpdate(); }} />
                        ${mode === "oauth" ? "OAuth 浏览器授权" : mode === "deviceCode" ? "设备码授权" : mode === "none" ? "免密" : "API Key 密钥"}
                     </label>
                  `)}
              </div>
            ` : nothing}

            ${selection.authMode === "none" ? html`
               <div class="wizard-v2-provider-input-group" style="margin-top: 8px;">
                   <div style="font-size:12px; color:#52C41A; text-align:center; padding:8px; background:#F6FFED; border: 1px solid #B7EB8F; border-radius:6px;">环境检测中... (无需配置密钥)</div>
               </div>
            ` : selection.authMode === "deviceCode" ? html`
               <div style="text-align:center; padding: 12px 0;">
                  ${value ? html`
                     <button class="wizard-v2-btn wizard-v2-btn-secondary" style="width:100%; display:flex; align-items:center; justify-content:center; gap:8px;" @click=${() => {
                  delete configMap[p.id];
                  delete deviceCodeState[p.id];
                  state.requestUpdate();
               }}>
                        <span style="color:#52C41A;">✓ 设备码授权成功</span> <span style="color:#999;font-size:12px;">(点击取消)</span>
                     </button>
                  ` : deviceCodeState[p.id] ? html`
                     <div style="background: #F6FFED; border: 1px solid #B7EB8F; border-radius: 8px; padding: 16px; text-align: center;">
                        <div style="font-size:13px; color:#333; font-weight:600; margin-bottom:8px;">请在浏览器中访问以下链接并输入验证码：</div>
                        <a href="${deviceCodeState[p.id].verificationUri}" target="_blank" style="color:#1890FF; font-size:13px; word-break:break-all;">
                           ${deviceCodeState[p.id].verificationUri}
                        </a>
                        <div style="font-size:28px; font-weight:bold; letter-spacing:4px; color:#1890FF; margin:12px 0; font-family:monospace;">
                           ${deviceCodeState[p.id].userCode}
                        </div>
                        ${deviceCodeState[p.id].error ? html`
                           <div style="color:#FF4D4F; font-size:12px; margin-bottom:8px;">${deviceCodeState[p.id].error}</div>
                        ` : html`
                           <div style="color:#999; font-size:12px; display:flex; align-items:center; justify-content:center; gap:6px;">
                              <span class="wizard-v2-spinner" style="width:14px; height:14px; border-width:2px;"></span>
                              等待授权确认中...
                           </div>
                        `}
                        <div style="font-size:12px; color:#999; cursor:pointer; margin-top:8px;" @click=${() => { delete deviceCodeState[p.id]; state.requestUpdate(); }}>取消</div>
                     </div>
                  ` : html`
                     <button class="wizard-v2-btn wizard-v2-btn-secondary" style="width:100%; display:flex; align-items:center; justify-content:center; gap:8px;" @click=${async (e: Event) => {
                  const btn = e.currentTarget as HTMLButtonElement;
                  btn.disabled = true; btn.textContent = "⏳ 正在获取设备码...";
                  try {
                     const res = await state.client!.request<any>("wizard.v2.oauth.device.start", { provider: p.id });
                     if (res?.userCode && res?.verificationUri) {
                        deviceCodeState[p.id] = {
                           userCode: res.userCode,
                           verificationUri: res.verificationUri,
                           sessionId: res.sessionId,
                           polling: true
                        };
                        state.requestUpdate();
                        // Start polling
                        const pollInterval = setInterval(async () => {
                           try {
                              const pollRes = await state.client!.request<any>("wizard.v2.oauth.device.poll", { sessionId: res.sessionId });
                              if (pollRes?.status === "completed") {
                                 configMap[p.id] = "device-code-" + p.id;
                                 delete deviceCodeState[p.id];
                                 clearInterval(pollInterval);
                                 state.requestUpdate();
                              } else if (pollRes?.status === "expired" || pollRes?.status === "error") {
                                 deviceCodeState[p.id] = { ...deviceCodeState[p.id], polling: false, error: pollRes?.error || "授权已过期，请重试" };
                                 clearInterval(pollInterval);
                                 state.requestUpdate();
                              }
                           } catch {
                              // polling error, continue
                           }
                        }, 5000);
                        // Auto-stop after 10 minutes
                        setTimeout(() => {
                           clearInterval(pollInterval);
                           if (deviceCodeState[p.id]?.polling) {
                              deviceCodeState[p.id] = { ...deviceCodeState[p.id], polling: false, error: "授权超时，请重试" };
                              state.requestUpdate();
                           }
                        }, 600000);
                     }
                  } catch (err: any) {
                     console.error("Device code start failed:", err);
                  } finally {
                     btn.disabled = false;
                     btn.textContent = "";
                     state.requestUpdate();
                  }
               }}>
                        ↗ 获取设备授权码
                     </button>
                  `}
               </div>
            ` : selection.authMode === "oauth" ? html`
               <div style="text-align:center; padding: 12px 0;">
                  ${value ? html`
                     <button class="wizard-v2-btn wizard-v2-btn-secondary" style="width:100%; display:flex; align-items:center; justify-content:center; gap:8px;" @click=${() => {
                  delete configMap[p.id];
                  state.requestUpdate();
               }}>
                        <span style="color:#52C41A;">✓ 授权成功</span> <span style="color:#999;font-size:12px;">(点击取消)</span>
                     </button>
                  ` : (pendingOauthSelection === p.id ? html`
                     <div style="display:flex; flex-direction:column; gap:8px; background: #fff; border: 1px solid #1890FF; border-radius: 8px; padding: 12px; box-shadow: 0 4px 12px rgba(24,144,255,0.15);">
                        <div style="font-size:13px; color:#333; font-weight:600; text-align: left; margin-bottom:4px;">请选择要挂载授权的目标应用：</div>
                        <button class="wizard-v2-btn wizard-v2-btn-secondary" style="width:100%; justify-content:flex-start; font-weight: 500;" @click=${() => {
                  configMap[p.id] = "oauth-authorized";
                  pendingOauthSelection = null; state.requestUpdate();
               }}>
                            ${p.name.split(' ')[0]} Antigravity Desktop
                        </button>
                        <button class="wizard-v2-btn wizard-v2-btn-secondary" style="width:100%; justify-content:flex-start; color: #666;" @click=${() => {
                  configMap[p.id] = "oauth-authorized";
                  pendingOauthSelection = null; state.requestUpdate();
               }}>
                            ${p.name.split(' ')[0]} Headless CLI 工具
                        </button>
                        <div style="font-size:12px; color:#999; cursor:pointer; margin-top:4px;" @click=${() => { pendingOauthSelection = null; state.requestUpdate(); }}>取消本次授权</div>
                     </div>
                  ` : html`
                     <button class="wizard-v2-btn wizard-v2-btn-secondary" style="width:100%; display:flex; align-items:center; justify-content:center; gap:8px;" @click=${async (e: Event) => {
                  const btn = e.currentTarget as HTMLButtonElement;
                  btn.disabled = true; btn.textContent = "⏳ 正在拉起安全浏览器鉴权...";
                  try {
                     await state.client!.request<any>("wizard.v2.oauth", { provider: p.id }).catch(() => null);
                     pendingOauthSelection = p.id;
                  } catch (err: any) {
                     console.error("OAuth failed:", err);
                     pendingOauthSelection = p.id;
                  } finally {
                     btn.disabled = false;
                     state.requestUpdate();
                  }
               }}>
                        ↗ 跳转本地安全浏览器网页授权
                     </button>
                  `)}
               </div>
            ` : html`
               <div class="wizard-v2-provider-input-group">
                   <input
                     type="password"
                     placeholder="输入 API_KEY"
                     class="wizard-v2-input"
                     .value=${value}
                     @input=${(e: Event) => {
               const val = (e.target as HTMLInputElement).value;
               configMap[p.id] = val;
               state.requestUpdate();
            }}
                   />
               </div>
            `}
          </div>
        `;
   })}
    </div>
  `;
}

// ─── Core Render ───

export function renderWizardV2(state: AppViewState) {
   if (!state.wizardV2Open) return nothing;

   const steps = getWizardSteps();
   const currentStepKey = steps[stepIndex]?.key ?? steps[0]?.key ?? "welcome";
   const logoSrc = publicAssetUrl(state.basePath ?? "", "/logo1.png");
   const recoveryHints = describeRecoveryHints(state.configSnapshot);
   const latestValidBackup = recoveryBackups.find((entry) => entry.valid) ?? null;

   // Conditionally disable the primary next button if requirement not met
   let canNext = true;
   if (currentStepKey === "welcome" && wizardMode === "setup") {
      canNext = securityAck;
   } else if (currentStepKey === "primary") {
      canNext = Object.values(primaryConfig).some(val => val.trim().length > 0);
   }

   const nextButtonLabel =
      currentStepKey === "security"
         ? "应用配置"
         : currentStepKey === "primary" && wizardMode === "recovery"
            ? "覆盖当前坏配置"
            : stepIndex === steps.length - 2
               ? "完成"
               : "下一步";

   return html`
    <div class="wizard-v2-overlay">
      <div class="wizard-v2-card">
        
        <!-- Header / Progress Bar -->
        <div class="wizard-v2-header">
           <div class="wizard-v2-step-indicator">
              ${steps.map((step, idx) => {
      const isActive = idx === stepIndex;
      const isCompleted = idx < stepIndex;
      let cls = "wizard-v2-step pending";
      if (isActive) cls = "wizard-v2-step active";
      else if (isCompleted) cls = "wizard-v2-step completed";

      return html`
                  <div class="${cls}">
                    <div class="wizard-v2-step-circle">${isCompleted ? "✓" : idx + 1}</div>
                    <div class="wizard-v2-step-label">${step.label}</div>
                  </div>
                  ${idx < steps.length - 1 ? html`<div class="wizard-v2-step-connector ${isCompleted ? 'completed' : ''}"></div>` : nothing}
                `;
   })}
           </div>
        </div>

        <!-- Body / Content -->
        <div class="wizard-v2-body">
            ${keyed(stepIndex, html`
            ${currentStepKey === "welcome" ? html`
              ${wizardMode === "recovery" ? html`
                <div style="text-align:center; margin-bottom: 24px;">
                  <img src=${logoSrc} alt="Crab Claw（蟹爪） Logo" style="width: 80px; height: auto;" />
                </div>
                <h2 class="wizard-v2-title" style="text-align:center; margin-top: 0; margin-bottom: 8px;">恢复到可用配置</h2>
                <p class="wizard-v2-subtitle" style="text-align:center; margin-top: 0; margin-bottom: 24px;">
                  当前检测到配置缺失、损坏或不可继续使用。恢复模式会优先用最少步骤把系统拉回可用状态。
                </p>

                <div class="wizard-v2-provider-card" style="text-align:left; margin-bottom: 16px;">
                  <h3 style="margin-top:0; margin-bottom:10px; font-size:15px;">恢复模式会做什么</h3>
                  <div style="display:grid; gap:8px; color:#5C6B77; font-size:13px; line-height:1.6;">
                    <div>1. 优先恢复最近一次可用配置（如果检测到有效备份）。</div>
                    <div>2. 如果不能直接恢复，就只补最小必要配置。</div>
                    <div>3. 恢复完成后会直接覆盖当前坏配置。</div>
                  </div>
                </div>

                <div class="wizard-v2-provider-card" style="text-align:left; margin-bottom: 16px; border-left: 4px solid #FAAD14; background: #FFFBE6;">
                  <h3 style="margin-top:0; margin-bottom:10px; font-size:15px; color:#D48806;">当前检测结果</h3>
                  <div style="display:grid; gap:8px; color:#666; font-size:13px; line-height:1.6;">
                    ${recoveryHints.map((hint) => html`<div>• ${hint}</div>`)}
                  </div>
                </div>

                ${latestValidBackup ? html`
                  <div class="wizard-v2-provider-card wizard-v2-provider-card-selected" style="text-align:left; margin-bottom: 16px;">
                    <div style="display:flex; justify-content:space-between; gap:16px; align-items:flex-start; flex-wrap:wrap;">
                      <div>
                        <div style="font-weight:600; margin-bottom:6px;">恢复最近可用配置</div>
                        <div style="font-size:13px; color:#666; line-height:1.6;">
                          检测到最近一份有效备份，恢复后会直接覆盖当前坏配置并触发网关重载。<br/>
                          备份时间：${latestValidBackup.modTime}
                        </div>
                      </div>
                      <button
                        class="wizard-v2-btn wizard-v2-btn-primary"
                        ?disabled=${recoveryBackupsLoading || stepTransitioning}
                        @click=${() => restoreLatestRecoveryBackup(state)}
                      >
                        直接恢复最近配置
                      </button>
                    </div>
                  </div>
                ` : html`
                  <div class="wizard-v2-provider-card" style="text-align:left; margin-bottom: 16px;">
                    <div style="font-weight:600; margin-bottom:6px;">没有可直接回滚的有效备份</div>
                    <div style="font-size:13px; color:#666; line-height:1.6;">
                      继续下一步，重新补齐主模型等最小必要配置。
                    </div>
                  </div>
                `}

                ${recoveryRestoreError ? html`
                  <div class="wizard-v2-provider-card" style="text-align:left; margin-bottom: 16px; border-left: 4px solid #FF4D4F; background: #FFF1F0;">
                    <div style="font-size:13px; color:#A8071A; line-height:1.6;">${recoveryRestoreError}</div>
                  </div>
                ` : nothing}

                <div class="wizard-v2-provider-card" style="text-align:left; background:#F8F9FA;">
                  <div style="font-weight:600; margin-bottom:6px;">安全与权限设置已移到设置层</div>
                  <div style="font-size:13px; color:#666; line-height:1.6;">
                    恢复流程只处理最小可用配置。详细安全级别、权限审批和高级策略，请在恢复完成后到“设置 &gt; 安全”调整。
                  </div>
                </div>
              ` : html`
                <div style="text-align:center; margin-bottom: 24px;">
                  <img src=${logoSrc} alt="Crab Claw（蟹爪） Logo" style="width: 80px; height: auto;" />
                </div>
                <h2 class="wizard-v2-title" style="text-align:center; margin-top: 0; margin-bottom: 8px;">首次初始化 Crab Claw</h2>
                <p class="wizard-v2-subtitle" style="text-align:center; margin-top: 0; margin-bottom: 24px;">
                  本向导会帮助你完成最小可用配置。初始化完成后，详细安全和权限设置统一在设置层管理。
                </p>

                <div style="display:grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-bottom: 24px; text-align: left;">
                  <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #1890FF; margin-bottom: 4px;">主模型</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">先补齐一个可运行的主模型，确保系统能立即使用。</div>
                  </div>
                  <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #52C41A; margin-bottom: 4px;">常用能力</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">后续可按需接入备用模型、技能、频道与记忆能力。</div>
                  </div>
                  <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #FAAD14; margin-bottom: 4px;">安全设置</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">默认采用推荐安全配置；详细策略在“设置 &gt; 安全”中调整。</div>
                  </div>
                </div>

                <div class="wizard-v2-provider-card" style="border-left: 4px solid #FAAD14; background: #FFFBE6; text-align: left;">
                  <h3 style="color:#D48806; margin-top:0; font-size: 15px;">初始化前请确认</h3>
                  <p style="color:#666; line-height: 1.6; font-size: 13px; margin-bottom: 12px;">
                    • 本向导会把当前配置写入桌面应用；<br/>
                    • API Key 等敏感信息请只填写你准备交给本机使用的配置；<br/>
                    • 详细安全级别、权限审批与高级策略，完成后可到“设置 &gt; 安全”继续配置。
                  </p>
                  <label style="display: flex; align-items: center; gap: 8px; font-weight: 500; cursor: pointer; font-size: 14px;">
                     <input type="checkbox" id="v2-security-ack" .checked=${securityAck} @change=${(e: Event) => {
               securityAck = (e.target as HTMLInputElement).checked;
               state.requestUpdate();
            }} />
                     我已了解并准备开始初始化
                  </label>
                </div>
              `}
            ` : nothing}

            ${currentStepKey === "primary" ? html`
              <!-- 2. 系统主模型选择（必填） -->
              <h2 class="wizard-v2-title">
                ${wizardMode === "recovery" ? "重新补齐最小可用主模型" : "配置系统主模型"} <span style="color:#FF4D4F; font-size:14px;">*必填</span>
              </h2>
              <p class="wizard-v2-subtitle">
                 ${wizardMode === "recovery"
         ? html`恢复模式只要求补齐最小必要配置。选择一个可用主模型后，系统会直接覆盖当前坏配置。`
         : html`所有主流 AI 服务商已预配置完成，您只需填入对应的 API Key 即可启用。<br/>
                 <span class="wizard-v2-highlight">由于这是主节点系统，至少需要配置一个服务商作为骨干大脑。</span>`}
              </p>
              ${renderProviders(state, primaryConfig, true)}
            ` : nothing}

            ${currentStepKey === "fallback" ? html`
              <!-- 3. 系统备用模型（选填） -->
              <h2 class="wizard-v2-title">配置系统备用模型 <span style="color:#999; font-size:14px;">(选填)</span></h2>
              <p class="wizard-v2-subtitle">
                 当主模型 API 触发限流或出现网络抖动不稳定时，系统会自动 fallback 到备用模型节点。<br/>
                 推荐配置一个不同于主模型的服务商以确保最高可用性。
              </p>
              ${renderProviders(state, fallbackConfig, false)}
            ` : nothing}

            ${currentStepKey === "skills" ? html`
              <!-- 4. 技能 -->
              <h2 class="wizard-v2-title">技能池预置与加载</h2>
              
              <div style="background: rgba(250, 173, 20, 0.1); border-left: 4px solid #FAAD14; padding: 12px 16px; margin-bottom: 24px; border-radius: 4px;">
                 <div style="color: #FAAD14; font-weight: 600; margin-bottom: 4px;">⚠️ 外部技能风险提示</div>
                 <div style="color: #666; font-size: 13px; line-height: 1.5;">
                    勾选启用的技能将被系统全局使用。请谨慎连接和加载来历不明的外部技能接口（如第三方的 MCP 源），
                    以防止 AI 读取或利用隐藏在不可信来源中的恶意链接进行钓鱼或破坏。
                 </div>
              </div>
              
              <p class="wizard-v2-subtitle">系统检测到如下核心技能。如果有需要额外 API Key 的技能，请在下方填入。</p>
              
              <div class="wizard-v2-providers-grid">
                 ${resolveWizardSkillGroups().map((group) => {
            const meta = SKILL_GROUP_META[group.key] ?? {
               name: group.key,
               icon: "🧩",
               desc: group.tools.join(" / "),
            };
            const toolSummary = group.tools.length > 0 ? `(${group.tools.join(", ")})` : "";
            return html`
                    <label class="wizard-v2-provider-card" style="display:flex; align-items:center; gap: 12px; cursor: pointer;">
                       <input type="checkbox" .checked=${selectedSkills[group.key] ?? group.defaultOn} @change=${(e: Event) => { selectedSkills[group.key] = (e.target as HTMLInputElement).checked; state.requestUpdate(); }}>
                       <div>
                         <div style="font-weight:600;">${meta.icon} ${meta.name}</div>
                         <div style="font-size:12px; color:#888;">${meta.desc}${toolSummary ? ` ${toolSummary}` : ""}</div>
                       </div>
                    </label>
                 `;
         })}
              </div>
            ` : nothing}

            ${currentStepKey === "channels" ? html`
              <!-- 5. 频道 -->
              <h2 class="wizard-v2-title">接入通讯频道</h2>
              <p class="wizard-v2-subtitle">将 Crab Claw（蟹爪） 接入您的 IM 矩阵，让智能体主动触达业务一线。</p>
              
              <div class="wizard-v2-providers-grid">
                 <div class="wizard-v2-provider-card">
                    <div style="font-weight:600; margin-bottom:8px;">飞书 (Feishu / Lark)</div>
                    <div class="wizard-v2-provider-input-group">
                       <input type="text" placeholder="APP_ID" class="wizard-v2-input" .value=${channelConfig.feishu.appId} @input=${(e: Event) => { channelConfig.feishu.appId = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                       <input type="password" placeholder="APP_SECRET" class="wizard-v2-input" .value=${channelConfig.feishu.appSecret} @input=${(e: Event) => { channelConfig.feishu.appSecret = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                    </div>
                 </div>
                 <div class="wizard-v2-provider-card">
                    <div style="font-weight:600; margin-bottom:8px;">企微 (WeCom)</div>
                    <div class="wizard-v2-provider-input-group">
                       <input type="text" placeholder="CORP_ID" class="wizard-v2-input" .value=${channelConfig.wecom.appId} @input=${(e: Event) => { channelConfig.wecom.appId = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                       <input type="password" placeholder="CORP_SECRET" class="wizard-v2-input" .value=${channelConfig.wecom.appSecret} @input=${(e: Event) => { channelConfig.wecom.appSecret = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                    </div>
                 </div>
                 <div class="wizard-v2-provider-card">
                    <div style="font-weight:600; margin-bottom:8px;">钉钉 (DingTalk)</div>
                    <div class="wizard-v2-provider-input-group">
                       <input type="text" placeholder="AppKey" class="wizard-v2-input" .value=${channelConfig.dingtalk.appKey} @input=${(e: Event) => { channelConfig.dingtalk.appKey = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                       <input type="password" placeholder="AppSecret" class="wizard-v2-input" .value=${channelConfig.dingtalk.appSecret} @input=${(e: Event) => { channelConfig.dingtalk.appSecret = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                    </div>
                 </div>
                 <div class="wizard-v2-provider-card">
                    <div style="font-weight:600; margin-bottom:8px;">Telegram <span style="font-size:12px;color:#888;font-weight:normal;">(国际端)</span></div>
                    <div class="wizard-v2-provider-input-group">
                       <input type="password" placeholder="Bot Token" class="wizard-v2-input" .value=${channelConfig.telegram.botToken} @input=${(e: Event) => { channelConfig.telegram.botToken = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                    </div>
                 </div>
              </div>
            ` : nothing}

            ${currentStepKey === "subagents" ? html`
              <!-- 6. 子智能选择 (纯展示) -->
              <h2 class="wizard-v2-title">认识子智能体 (Sub-Agents) <span style="color:#999; font-size:14px;">(信息展示)</span></h2>
              <p class="wizard-v2-subtitle">
                 子智能体可能依赖特定强度的多模态大模型和独立的工具沙箱，因此不在配置向导中直接挂载。<br/>
                 系统采用先进的 <b>三级指挥与门控体系 (Three-Tier Command Architecture)</b> 对它们进行管理：<br/>
                 <span style="font-size:13px; color:#666;">
                    <b>Level 1. 方案确认门控</b> (发起时) ➔ <b>Level 2. 质量审核门控</b> (执行中) ➔ <b>Level 3. 最终交付门控</b> (完成时)
                 </span>
              </p>
              
              <div class="wizard-v2-providers-grid" style="display: block;">
                 <div class="wizard-v2-provider-card" style="display:flex; align-items:flex-start; gap: 12px; margin-bottom: 16px;">
                    <div style="font-size:20px; line-height:1; margin-top:2px;">👨‍💻</div>
                    <div>
                        <div style="font-weight:600; color: #096DD9;">编程引擎 (OpenCoder)</div>
                        <div style="font-size:13px; color:#666; line-height:1.5; margin-top: 4px;">
                            <b>核心能力：</b>专为代码审计、架构重构、全栈开发而设计。由于可能涉及最高级别的 Bash 修改权，需确保挂载最高安全等级并使用如 Claude 3.6/GPT-5 等最强推理引擎。在三级指挥系统中，主智能体将作为“站长”对其代码变更进行预审。
                        </div>
                    </div>
                 </div>
                 
                 <div class="wizard-v2-provider-card" style="display:flex; align-items:flex-start; gap: 12px; margin-bottom: 16px;">
                    <div style="font-size:20px; line-height:1; margin-top:2px;">👁️</div>
                    <div>
                        <div style="font-weight:600; color: #531DAB;">视觉控制流 (灵瞳)</div>
                        <div style="font-size:13px; color:#666; line-height:1.5; margin-top: 4px;">
                            <b>核心能力：</b>专注于多模态图片的像素级识别和电脑屏幕 UI 的自动化分析执行。配置该智能体时必须关联可支持图像输入的专用多模态大模型。它独立共享主系统的三级指挥管线以保障视觉操作的安全边界。
                        </div>
                    </div>
                 </div>
                 
                 <div class="wizard-v2-provider-card" style="display:flex; align-items:flex-start; gap: 12px; margin-bottom: 16px;">
                    <div style="font-size:20px; line-height:1; margin-top:2px;">📈</div>
                    <div>
                        <div style="font-weight:600; color: #D48806;">全媒体运营 (Media Ops)</div>
                        <div style="font-size:13px; color:#666; line-height:1.5; margin-top: 4px;">
                            <b>核心能力：</b>全链路的营销生命周期托管。包含：互联网爬虫热点获取、内容洗稿与自动化配文、文章生成以及最后的跨平台自动投递发布。作为三级指挥体系中的功能子智能体，由主节点分发宏指令后异步并发执行。
                        </div>
                    </div>
                 </div>
              </div>
            ` : nothing}

            ${currentStepKey === "memory" ? html`
              <!-- 7. 记忆系统 -->
              <h2 class="wizard-v2-title">配置记忆系统 <span style="color:#999; font-size:14px;">(选填 - 高级特性)</span></h2>
              <p class="wizard-v2-subtitle">
                 <b>太虚永忆 (UHMS)</b> 采用字节跳动同源的分布式流式记忆与 OpenViking 引擎架构，支持 IVPQ 压缩与对海量上下文的在线学习。<br/>
                 默认采用本地轻量级 <b>L0/L1/L2 三级跨会话 VFS (虚拟文件系统)</b> 进行防失忆存储；若数据量达十亿级以上，强烈建议开启下方的高性能向量模式以确保低延迟精确语义召回。
              </p>
              
              <div class="wizard-v2-providers-grid" style="display: block;">
                 <label class="wizard-v2-provider-card" style="display:flex; align-items:center; gap: 12px; cursor: pointer; margin-bottom: 16px;">
                    <input type="checkbox" .checked=${memoryConfig.enableVector} @change=${(e: Event) => { memoryConfig.enableVector = (e.target as HTMLInputElement).checked; state.requestUpdate(); }}>
                    <div>
                      <div style="font-weight:600;">启用向量数据库模式 (Vector Mode)</div>
                      <div style="font-size:12px; color:#888;">推荐在长周期上下文任务中启用，可大幅提升历史记忆召回的语义精确度</div>
                    </div>
                 </label>
                 
                 ${memoryConfig.enableVector ? html`
                     <div class="wizard-v2-provider-card" style="animation: slideFadeIn 0.3s ease-out forwards;">
                         <div style="background: rgba(250, 173, 20, 0.1); border-left: 4px solid #FAAD14; padding: 12px 16px; margin-bottom: 16px; border-radius: 4px;">
                            <div style="color: #FAAD14; font-weight: 600; margin-bottom: 4px;">📌 性能与部署预警</div>
                            <div style="color: #666; font-size: 13px; line-height: 1.5;">
                               向量数据库索引与重绘会大量占用本地机器的内存与 CPU 资源。<br/>
                               当前向导不支持一键安装容器，请确保您已手动部署好本地容器或采用第三方云端向量托管 API。
                            </div>
                         </div>
                         
                         <div style="margin-bottom:12px;">
                             <div style="font-weight:600; margin-bottom:8px;">数据库宿主选项</div>
                             <div style="display:flex; gap: 16px;">
                                <label style="display:flex; align-items:center; gap:4px; font-weight:normal; cursor:pointer;">
                                   <input type="radio" name="vector-hosting" .checked=${memoryConfig.hostingType === "local"} @change=${() => { memoryConfig.hostingType = "local"; state.requestUpdate(); }} /> 本地 Docker 容器部署
                                </label>
                                <label style="display:flex; align-items:center; gap:4px; font-weight:normal; cursor:pointer;">
                                   <input type="radio" name="vector-hosting" .checked=${memoryConfig.hostingType === "cloud"} @change=${() => { memoryConfig.hostingType = "cloud"; state.requestUpdate(); }} /> 云端托管型 Vector DB
                                </label>
                             </div>
                         </div>
                         
                         <div class="wizard-v2-provider-input-group">
                            <label>向量库连接地址 (${memoryConfig.hostingType === 'local' ? '例如 http://localhost:8000' : '填写您的第三方云服务 Endpoint 连接串'})</label>
                            <input 
                               type="text" 
                               placeholder=${memoryConfig.hostingType === 'local' ? "http://127.0.0.1:8000" : "https://[YOUR_INSTANCE].region.cloud.qdrant.io"}
                               class="wizard-v2-input" 
                               .value=${memoryConfig.apiEndpoint} 
                               @input=${(e: Event) => { memoryConfig.apiEndpoint = (e.target as HTMLInputElement).value; state.requestUpdate(); }} 
                            />
                         </div>
                     </div>
                 ` : nothing}
              </div>

              <!-- Bug#11: 记忆提取 LLM 配置 -->
              <div style="margin-top: 24px; padding-top: 16px; border-top: 1px solid rgba(255,255,255,0.08);">
                <h3 style="font-size:15px; font-weight:600; margin-bottom:8px;">记忆提取 LLM 配置 <span style="font-size:12px; color:#8C8C8C; font-weight:normal;">（可选 — 不单独配置时默认跟随主模型）</span></h3>
                <p style="font-size:13px; color:#8C8C8C; margin-bottom:12px;">可为记忆提取单独指定一套 LLM；如不单独配置，系统会默认复用主模型配置。只有在最终没有可用 LLM 时，才会退化到基于关键词的启发式提取。</p>
                <div style="margin-bottom:12px; padding:10px 12px; border-radius:10px; border:1px solid rgba(250,173,20,0.35); background:rgba(250,173,20,0.08); color:#FAAD14; font-size:13px; line-height:1.5;">
                  提示：默认使用主模型进行记忆分级提取；仅当你希望记忆提取走独立模型时，才需要在这里单独填写。
                </div>

                <div style="display:grid; grid-template-columns: 1fr 1fr; gap:12px;">
                  <div class="wizard-v2-provider-input-group">
                    <label>LLM Provider</label>
                    <select class="wizard-v2-input" .value=${memoryConfig.llmProvider} @change=${(e: Event) => {
            memoryConfig.llmProvider = (e.target as HTMLSelectElement).value;
            if (!memoryConfig.llmProvider) {
               memoryConfig.llmModel = "";
               memoryConfig.llmApiKey = "";
               memoryConfig.llmBaseUrl = "";
            } else if (!memoryConfig.llmModel) {
               memoryConfig.llmModel = getDefaultMemoryModel(memoryConfig.llmProvider);
            }
            state.requestUpdate();
         }}>
                      <option value="">跟随主模型（默认）</option>
                      <option value="deepseek">DeepSeek</option>
                      <option value="openai">OpenAI</option>
                      <option value="anthropic">Anthropic</option>
                      <option value="ollama">Ollama（本地）</option>
                    </select>
                  </div>
                  <div class="wizard-v2-provider-input-group">
                    <label>Model</label>
                    <input type="text" class="wizard-v2-input" placeholder="空=按 provider 默认" .value=${memoryConfig.llmModel} @input=${(e: Event) => { memoryConfig.llmModel = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                  </div>
                  <div class="wizard-v2-provider-input-group">
                    <label>API Key</label>
                    <input type="password" class="wizard-v2-input" placeholder="独立 API Key（可留空复用主模型 / Provider 配置）" .value=${memoryConfig.llmApiKey} @input=${(e: Event) => { memoryConfig.llmApiKey = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                  </div>
                  <div class="wizard-v2-provider-input-group">
                    <label>模型 URL 端点</label>
                    <input type="text" class="wizard-v2-input" placeholder=${getDefaultMemoryBaseUrl(memoryConfig.llmProvider) || "空=跟随主模型或使用 provider 默认 URL"} .value=${memoryConfig.llmBaseUrl} @input=${(e: Event) => { memoryConfig.llmBaseUrl = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                  </div>
                </div>
              </div>

              <div style="text-align:right; margin-top: 32px;">
                 <button class="wizard-v2-btn wizard-v2-btn-primary" style="background:#FAAD14; border-color:#FAAD14;" @click=${() => nextStep(state)}> 确定配置与继续 </button>
              </div>
            ` : nothing}

            ${currentStepKey === "security" ? html`
              <h2 class="wizard-v2-title">安全与权限入口</h2>
              <p class="wizard-v2-subtitle" style="margin-bottom:24px;">
                 初始化向导只保留默认安全配置。完整的安全级别、权限审批和风险策略统一在“设置 &gt; 安全”中继续配置。
              </p>

              <div class="wizard-v2-provider-card wizard-v2-provider-card-selected" style="text-align:left; margin-bottom: 16px;">
                 <div style="font-weight:600; margin-bottom:8px;">默认写入推荐安全级别</div>
                 <div style="font-size:13px; color:#666; line-height:1.6;">
                    当前默认使用 <b>L2 沙箱全权限（sandboxed）</b>，优先保证初始化后的可用性。<br/>
                    如果你需要更严格或更开放的策略，请在完成后到“设置 &gt; 安全”调整。
                 </div>
              </div>

              <label class="wizard-v2-provider-card" style="display:flex; align-items:center; gap: 12px; cursor: pointer;">
                 <input type="checkbox" .checked=${openSecuritySettingsAfterFinish} @change=${(e: Event) => {
            openSecuritySettingsAfterFinish = (e.target as HTMLInputElement).checked;
            state.requestUpdate();
         }}>
                 <div>
                    <div style="font-weight:600;">完成后自动打开“设置 &gt; 安全”</div>
                    <div style="font-size:12px; color:#888;">只做入口映射，不在初始化流程里展开复杂教学</div>
                 </div>
              </label>
            ` : nothing}

            ${currentStepKey === "done" ? html`
              <!-- 9. 完成 -->
              <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; height:100%; padding-bottom: 32px; animation: wizardV2FadeIn 0.5s ease-out;">
                 ${isRestarting ? html`
                     <div class="wizard-v2-restarting-container" style="text-align: center;">
                        <!-- Custom CSS Spinners added to stylesheet -->
                        <div class="wizard-v2-spinner"></div>
                        <h2 class="wizard-v2-title" style="margin-top:24px;">
                           ${completionKind === "recovery-restore"
         ? "正在恢复最近可用配置..."
         : completionKind === "recovery-reconfigure"
            ? "正在重建最小可用配置..."
            : "Crab Claw 引擎启动中..."}
                        </h2>
                        
                        <div style="width: 300px; height: 6px; background: #e8e8e8; border-radius: 4px; margin: 20px auto; overflow: hidden;">
                           <div style="width: ${restartProgress}%; height: 100%; background: #1890FF; transition: width 0.3s ease;"></div>
                        </div>
                        <div style="color: #666; font-size: 14px; margin-bottom: 32px; font-weight: bold;">
                           构建进度 ${restartProgress}%
                        </div>
                        
                        <div style="height: 48px; position:relative; overflow:hidden;">
                           ${completionKind === "recovery-restore"
         ? html`<div class="wizard-v2-advantage-text">正在用最近一次可用配置覆盖当前坏配置…</div>`
         : completionKind === "recovery-reconfigure"
            ? html`<div class="wizard-v2-advantage-text">正在写入最小必要配置并重新拉起运行时…</div>`
            : restartProgress < 30
               ? html`<div class="wizard-v2-advantage-text">💎 万物互联: 支持 25+ 主流服务商自动发现接入</div>`
               : (restartProgress < 60
                  ? html`<div class="wizard-v2-advantage-text">🚄 高效协同: 搭载 Code Auditor 与翻译专家子矩阵串联引擎</div>`
                  : (restartProgress < 90
                     ? html`<div class="wizard-v2-advantage-text">🛡️ 默认安全策略与核心服务装载中</div>`
                     : html`<div class="wizard-v2-advantage-text">🧠 核心记忆链唤醒...完成。</div>`))}
                        </div>
                     </div>
                 ` : html`
                     <div style="font-size: 64px; color: #52C41A; margin-bottom:16px;">✓</div>
                     <h2 class="wizard-v2-title" style="margin-bottom:8px;">
                        ${completionKind === "recovery-restore"
         ? "最近可用配置已恢复"
         : completionKind === "recovery-reconfigure"
            ? "恢复配置已重建"
            : "系统初始化完成"}
                     </h2>
                     <p class="wizard-v2-subtitle" style="text-align:center;">
                        ${completionKind === "recovery-restore"
         ? html`已使用最近一次可用备份覆盖当前坏配置，你现在可以继续回到主界面。`
         : completionKind === "recovery-reconfigure"
            ? html`最小必要配置已经重新写入，并覆盖当前不可用配置。详细安全与高级能力可稍后在设置中补充。`
            : html`主模型与基础能力已完成初始化。详细安全与权限设置可在“设置 &gt; 安全”继续调整。`}
                     </p>
                     
                     <div style="display:flex; gap:16px; margin-top:16px;">
                        <button class="wizard-v2-btn wizard-v2-btn-secondary" @click=${() => {
               isRestarting = false;
               restartProgress = 0;
               stepIndex = 0;
               if (wizardMode === "setup") {
                  securityAck = false;
               }
               recoveryRestoreError = null;
               state.requestUpdate();
            }} style="padding: 12px 32px; font-size:16px;">
                           重新配置
                        </button>
                        
                        <button class="wizard-v2-btn wizard-v2-btn-primary" @click=${() => {
               closeWizardV2(state);
               if (wizardMode === "setup") {
                  const targetTab = openSecuritySettingsAfterFinish ? "security" : "chat";
                  state.setTab(targetTab);
                  if (targetTab === "chat") {
                     const waitForConnection = (attempts: number) => {
                        if (state.connected || attempts <= 0) {
                           state.handleSendChat("/new");
                           setTimeout(() => {
                              state.handleSendChat("系统启动完成！请向我全面介绍一下 Crab Claw（蟹爪）系统的各项优势与核心能力，并做个简短的欢迎致辞。");
                           }, 300);
                        } else {
                           setTimeout(() => waitForConnection(attempts - 1), 500);
                        }
                     };
                     setTimeout(() => waitForConnection(10), 500);
                  }
                  return;
               }
               state.setTab("overview");
            }} style="padding: 12px 32px; font-size:16px;">
                           ${wizardMode === "setup"
         ? (openSecuritySettingsAfterFinish ? "前往安全设置" : "进入主界面")
         : "返回主界面"}
                        </button>
                     </div>
                 `}
              </div>
            ` : nothing}
            `)
      }
</div>
   
   <!-- Footer / Actions -->
   ${stepIndex < steps.length - 1 && !isRestarting ? html`
          <div class="wizard-v2-footer">
             <div style="display:flex; gap:12px;">
                <button class="wizard-v2-btn wizard-v2-btn-secondary" @click=${() => prevStep(state)}>
                   ${stepIndex === 0 ? "取消" : "上一步"}
                </button>
                <button class="wizard-v2-btn wizard-v2-btn-secondary" @click=${() => closeWizardV2(state, true)}>
                   稍后继续 (保存草稿并关闭)
                </button>
             </div>
             <button 
                class="wizard-v2-btn wizard-v2-btn-primary" 
                @click=${() => nextStep(state)}
                ?disabled=${!canNext}
                style="opacity: ${canNext ? 1 : 0.5}; cursor: ${canNext ? 'pointer' : 'not-allowed'};"
             >
                 ${nextButtonLabel}
             </button>
          </div>
        ` : nothing
      }

</div>
   </div>
      `;
}
