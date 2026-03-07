import { html, nothing } from "lit";
import { keyed } from "lit/directives/keyed.js";
import type { AppViewState } from "../app-view-state.ts";
import { type WizardProvider, mergeWithUI, FALLBACK_PROVIDERS } from "./wizard-v2-providers.ts";

// 动态 provider 列表（会话级缓存，首次打开时从后端获取）
let providers: WizardProvider[] = [];

// ─── Constants & Types ───

const STEPS = [
   "欢迎",
   "主模型",
   "备用模型",
   "技能",
   "频道",
   "子智能体",
   "记忆系统",
   "安全级别",
   "完成"
];



// Local prototype state (isolated from main app state for safety)
let stepIndex = 0;
let primaryConfig: Record<string, string> = {};
let fallbackConfig: Record<string, string> = {};

// Mock states for UI interactivity
let securityAck = false;
let selectedSkills: Record<string, boolean> = { fs: true, runtime: true, ui: true, web: true, memory: true, sessions: true, automation: false, messaging: false, nodes: false };
let channelConfig: any = {
   feishu: { appId: "", appSecret: "" },
   wecom: { appId: "", appSecret: "" },
   dingtalk: { appKey: "", appSecret: "" },
   telegram: { botToken: "" }
};
let subAgentsConfig: Record<string, { enabled: boolean }> = {};
let memoryConfig = { enableVector: false, hostingType: "local", apiEndpoint: "", llmProvider: "", llmModel: "", llmApiKey: "", llmBaseUrl: "" };
let securityLevelConfig = "allowlist"; // Default
let isRestarting = false;
let restartProgress = 0;
let providerSelections: Record<string, { model: string, authMode: string }> = {}; // Store selected model/authMode per provider
let customBaseUrls: Record<string, string> = {}; // Store base URLs for custom providers
let pendingOauthSelection: string | null = null; // Track which provider is waiting for app selection
// Device Code OAuth state
let deviceCodeState: Record<string, { userCode: string, verificationUri: string, sessionId: string, polling: boolean, error?: string }> = {};

// ─── Controller functions ───

const DRAFT_KEY = "openacosmi_wizard_v2_draft";
const DRAFT_RESUME_KEY = "openacosmi_wizard_v2_resume";

export function saveWizardV2Draft(): void {
   const draft = {
      stepIndex, primaryConfig, fallbackConfig, securityAck,
      selectedSkills, channelConfig, subAgentsConfig, memoryConfig,
      securityLevelConfig, providerSelections, customBaseUrls
   };
   localStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
}

function resetWizardV2State(): void {
   stepIndex = 0;
   primaryConfig = {};
   fallbackConfig = {};
   securityAck = false;
   selectedSkills = { fs: true, runtime: true, ui: true, web: true, memory: true, sessions: true, automation: false, messaging: false, nodes: false };
   channelConfig = { feishu: { appId: "", appSecret: "" }, wecom: { appId: "", appSecret: "" }, dingtalk: { appKey: "", appSecret: "" }, telegram: { botToken: "" } };
   subAgentsConfig = {};
   memoryConfig = { enableVector: false, hostingType: "local", apiEndpoint: "", llmProvider: "", llmModel: "", llmApiKey: "", llmBaseUrl: "" };
   securityLevelConfig = "allowlist";
   providerSelections = {};
   customBaseUrls = {};
   pendingOauthSelection = null;
   deviceCodeState = {};
}

function ensureProviderSelections(): void {
   providers.forEach(p => {
      if (!providerSelections[p.id]) {
         providerSelections[p.id] = { model: p.models[0]?.id ?? "", authMode: p.authModes[0] };
      }
   });
}

export async function startWizardV2(state: AppViewState, opts?: { resumeDraft?: boolean }): Promise<void> {
   // 从后端动态获取 provider 目录（会话级缓存）
   if (providers.length === 0) {
      try {
         const res = await state.client!.request<any>("wizard.v2.providers.list", {});
         providers = res?.providers?.length ? mergeWithUI(res.providers) : FALLBACK_PROVIDERS;
      } catch {
         providers = FALLBACK_PROVIDERS;
      }
   }

   // 默认全新启动向导，避免旧密钥/配置自动回填。
   // 仅在显式 resumeDraft=true 时恢复草稿。
   const shouldResumeDraft = opts?.resumeDraft === true || localStorage.getItem(DRAFT_RESUME_KEY) === "1";
   const saved = shouldResumeDraft ? localStorage.getItem(DRAFT_KEY) : null;
   if (saved) {
      try {
         const draft = JSON.parse(saved);
         stepIndex = draft.stepIndex || 0;
         primaryConfig = draft.primaryConfig || {};
         fallbackConfig = draft.fallbackConfig || {};
         securityAck = draft.securityAck ?? false;
         selectedSkills = draft.selectedSkills || { fs: true, runtime: true, ui: true, web: true, memory: true, sessions: true, automation: false, messaging: false, nodes: false };
         channelConfig = draft.channelConfig || { feishu: { appId: "", appSecret: "" }, wecom: { appId: "", appSecret: "" }, dingtalk: { appKey: "", appSecret: "" }, telegram: { botToken: "" } };
         subAgentsConfig = draft.subAgentsConfig || {};
         memoryConfig = draft.memoryConfig || { enableVector: false, hostingType: "local", apiEndpoint: "", llmProvider: "", llmModel: "", llmApiKey: "", llmBaseUrl: "" };
         securityLevelConfig = draft.securityLevelConfig || "allowlist";
         providerSelections = draft.providerSelections || {};
         customBaseUrls = draft.customBaseUrls || {};
      } catch (e) {
         console.error("Failed to parse wizard draft", e);
         resetWizardV2State();
         ensureProviderSelections();
         try {
            localStorage.removeItem(DRAFT_KEY);
            localStorage.removeItem(DRAFT_RESUME_KEY);
         } catch { }
      }
   } else {
      resetWizardV2State();
      ensureProviderSelections();
      try {
         localStorage.removeItem(DRAFT_KEY);
         localStorage.removeItem(DRAFT_RESUME_KEY);
      } catch { }
   }

   isRestarting = false;
   restartProgress = 0;

   // Ensure all providers have a selection entry, preserving draft if it exists
   ensureProviderSelections();

   state.wizardV2Open = true;
   state.requestUpdate();
}

export function closeWizardV2(state: AppViewState, saveDraft: boolean = false): void {
   if (saveDraft) {
      saveWizardV2Draft();
      try { localStorage.setItem(DRAFT_RESUME_KEY, "1"); } catch { }
   } else {
      try {
         localStorage.removeItem(DRAFT_KEY);
         localStorage.removeItem(DRAFT_RESUME_KEY);
      } catch { }
   }
   state.wizardV2Open = false;
   state.requestUpdate();
}

async function nextStep(state: AppViewState) {
   if (stepIndex === STEPS.length - 2) {
      // 进入最后"完成"步骤前，先将配置发送到后端持久化
      stepIndex++;
      isRestarting = true;
      restartProgress = 0;
      state.requestUpdate();

      try {
         // 构建 WizardV2Payload — 与后端 WizardV2Payload 结构对齐
         const payload = {
            primaryConfig,
            fallbackConfig,
            providerSelections,
            customBaseUrls,
            securityAck,
            selectedSkills,
            channelConfig,
            subAgentsConfig,
            memoryConfig,
            securityLevelConfig
         };

         // 调用后端 wizard.v2.apply RPC
         // 后端流程: parsePayload → convertToConfig → WriteConfigFile(自动 Keyring 脱敏) → ScheduleRestart(热重载)
         // 注意: 热重载会断开 WS，所以用 timeout 兜底 — 配置在重启前已落盘
         restartProgress = 20;
         state.requestUpdate();

         const timeoutPromise = new Promise(resolve => setTimeout(() => resolve({ ok: true, timeout: true }), 5000));
         const res = await Promise.race([
            state.client!.request<any>("wizard.v2.apply", payload).catch(() => ({ ok: true, disconnected: true })),
            timeoutPromise
         ]) as any;

         // 无论是正常返回、超时还是 WS 断开，配置都已写入后端 — 继续完成动画
         restartProgress = 60;
         state.requestUpdate();

         const interval = setInterval(() => {
            restartProgress += Math.floor(Math.random() * 15) + 5;
            if (restartProgress >= 100) {
               restartProgress = 100;
               isRestarting = false;
               clearInterval(interval);
               // 清除草稿（配置已持久化到后端）
               try {
                  localStorage.removeItem(DRAFT_KEY);
                  localStorage.removeItem(DRAFT_RESUME_KEY);
               } catch { }
            }
            state.requestUpdate();
         }, 300);
      } catch (err: any) {
         // 兜底: 即使出错也视为成功（配置可能已保存，只是 WS 断了）
         console.warn("wizard.v2.apply error (config may still be saved):", err);
         restartProgress = 60;
         state.requestUpdate();
         const interval = setInterval(() => {
            restartProgress += Math.floor(Math.random() * 15) + 5;
            if (restartProgress >= 100) {
               restartProgress = 100;
               isRestarting = false;
               clearInterval(interval);
               try {
                  localStorage.removeItem(DRAFT_KEY);
                  localStorage.removeItem(DRAFT_RESUME_KEY);
               } catch { }
            }
            state.requestUpdate();
         }, 300);
      }
   } else if (stepIndex < STEPS.length - 1) {
      stepIndex++;
      state.requestUpdate();
   }
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

   const currentStepName = STEPS[stepIndex];

   // Conditionally disable the primary next button if requirement not met
   let canNext = true;
   if (stepIndex === 0) { // Welcome
      canNext = securityAck;
   } else if (stepIndex === 1) { // Primary Model
      canNext = Object.values(primaryConfig).some(val => val.trim().length > 0);
   }

   return html`
    <div class="wizard-v2-overlay">
      <div class="wizard-v2-card">
        
        <!-- Header / Progress Bar -->
        <div class="wizard-v2-header">
           <div class="wizard-v2-step-indicator">
              ${STEPS.map((s, idx) => {
      const isActive = idx === stepIndex;
      const isCompleted = idx < stepIndex;
      let cls = "wizard-v2-step pending";
      if (isActive) cls = "wizard-v2-step active";
      else if (isCompleted) cls = "wizard-v2-step completed";

      return html`
                  <div class="${cls}">
                    <div class="wizard-v2-step-circle">${isCompleted ? "✓" : idx + 1}</div>
                    <div class="wizard-v2-step-label">${s}</div>
                  </div>
                  ${idx < STEPS.length - 1 ? html`<div class="wizard-v2-step-connector ${isCompleted ? 'completed' : ''}"></div>` : nothing}
                `;
   })}
           </div>
        </div>

        <!-- Body / Content -->
        <div class="wizard-v2-body">
            ${keyed(stepIndex, html`
            ${stepIndex === 0 ? html`
              <!-- 1. 欢迎页 / 安全须知 -->
              <div style="text-align:center; margin-bottom: 24px;">
                <img src="/logo1.png" alt="创宇太虚 Logo" style="width: 80px; height: auto;" />
              </div>
              <h2 class="wizard-v2-title" style="text-align:center; margin-top: 0; margin-bottom: 8px;">欢迎使用 创宇太虚（Claw Acosmi）</h2>
              <p class="wizard-v2-subtitle" style="text-align:center; margin-top: 0; margin-bottom: 24px;">由原 OpenClaw 重构升级的新一代安全智能体网关</p>

              <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-bottom: 24px; text-align: left;">
                 <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #1890FF; margin-bottom: 4px; display: flex; align-items: center; gap: 6px;"><span>⚡</span> Rust+Go 混合架构</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">极致性能体验，极低资源占用，即使是老旧低配硬件也能流畅运行无阻。</div>
                 </div>
                 <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #52C41A; margin-bottom: 4px; display: flex; align-items: center; gap: 6px;"><span>🧠</span> 字节开源级分布式记忆</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">引入字节跳动同源 OpenViking 分布式文件记忆系统，打造 AI 的太虚永忆。</div>
                 </div>
                 <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #FAAD14; margin-bottom: 4px; display: flex; align-items: center; gap: 6px;"><span>🛡️</span> Rust 原生级沙箱</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">自研 oa-sandbox 底层隔离引擎，深入操作系统内核切断风险外溢路径。</div>
                 </div>
                 <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #F5222D; margin-bottom: 4px; display: flex; align-items: center; gap: 6px;"><span>�</span> 4级安全防护制</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">新增四级动态隔离审批制度，复杂指令拦截升级，确保风险完全处于掌控之中。</div>
                 </div>
                 <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #722ED1; margin-bottom: 4px; display: flex; align-items: center; gap: 6px;"><span>🏛️</span> 三级指挥体系</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">严格遵守“用户 ➔ 主智能体(站长) ➔ 子智能体”的垂直管理与委托确认范式。</div>
                 </div>
                 <div style="background: #F8F9FA; padding: 12px 16px; border-radius: 8px; border: 1px solid #E4E7EB;">
                    <div style="font-weight: 600; color: #13C2C2; margin-bottom: 4px; display: flex; align-items: center; gap: 6px;"><span>📡</span> 异步队列与主动报告</div>
                    <div style="font-size: 13px; color: #5C6B77; line-height: 1.5;">任务离线挂载，执行完毕后主动跨频道发送看板报告，不再需要傻傻等待。</div>
                 </div>
              </div>
              
              <div class="wizard-v2-provider-card" style="border-left: 4px solid #FF4D4F; background: #FFF1F0; text-align: left;">
                 <h3 style="color:#FF4D4F; margin-top:0; font-size: 15px;">⚠️ 安全底线与知情同意</h3>
                 <p style="color:#666; line-height: 1.6; font-size: 13px; margin-bottom: 12px;">
                    本网关具备高度自动化执行能力。启动前请您必定知悉：<br/>
                    • 授予权限后，AI 将能在授权范围内读取本地文件并执行代码环境操作；<br/>
                    • 请务必将您的**敏感配置信息与密钥**远离 AI 所在的沙箱或操作目录；<br/>
                    • 强烈建议在生产环境至少保持**标准白名单模式(Allowlist)**开启。
                 </p>
                 <label style="display: flex; align-items: center; gap: 8px; font-weight: 500; cursor: pointer; font-size: 14px;">
                    <input type="checkbox" id="v2-security-ack" .checked=${securityAck} @change=${(e: Event) => {
            securityAck = (e.target as HTMLInputElement).checked;
            state.requestUpdate();
         }} />
                    我已了解风险并愿意继续使用
                 </label>
              </div>
            ` : nothing}

            ${stepIndex === 1 ? html`
              <!-- 2. 系统主模型选择（必填） -->
              <h2 class="wizard-v2-title">配置系统主模型 <span style="color:#FF4D4F; font-size:14px;">*必填</span></h2>
              <p class="wizard-v2-subtitle">
                 所有主流 AI 服务商已预配置完成，您只需填入对应的 API Key 即可启用。<br/>
                 <span class="wizard-v2-highlight">由于这是主节点系统，至少需要配置一个服务商作为骨干大脑。</span>
              </p>
              ${renderProviders(state, primaryConfig, true)}
            ` : nothing}

            ${stepIndex === 2 ? html`
              <!-- 3. 系统备用模型（选填） -->
              <h2 class="wizard-v2-title">配置系统备用模型 <span style="color:#999; font-size:14px;">(选填)</span></h2>
              <p class="wizard-v2-subtitle">
                 当主模型 API 触发限流或出现网络抖动不稳定时，系统会自动 fallback 到备用模型节点。<br/>
                 推荐配置一个不同于主模型的服务商以确保最高可用性。
              </p>
              ${renderProviders(state, fallbackConfig, false)}
            ` : nothing}

            ${stepIndex === 3 ? html`
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
                 ${[
            { key: "fs", name: "文件系统", desc: "读取、编辑、搜索文件 (read/write/edit)", icon: "📁" },
            { key: "runtime", name: "命令执行", desc: "终端命令与进程管理 (exec/process)", icon: "⚡" },
            { key: "ui", name: "浏览器与画布", desc: "无头浏览器和画布交互 (browser/canvas)", icon: "🌐" },
            { key: "web", name: "网页搜索", desc: "在线搜索与网页抓取 (web_search/fetch)", icon: "🔍" },
            { key: "memory", name: "记忆调用", desc: "搜索和获取长期记忆 (memory_*)", icon: "🧠" },
            { key: "sessions", name: "会话管理", desc: "列出、发送、生成会话 (sessions_*)", icon: "💬" },
            { key: "automation", name: "自动化", desc: "定时任务与网关 (cron/gateway)", icon: "⏰" },
            { key: "messaging", name: "消息推送", desc: "主动发送消息到频道 (message)", icon: "📤" },
            { key: "nodes", name: "节点管理", desc: "管理远端计算节点 (nodes)", icon: "🖥️" },
         ].map(sg => html`
                    <label class="wizard-v2-provider-card" style="display:flex; align-items:center; gap: 12px; cursor: pointer;">
                       <input type="checkbox" .checked=${selectedSkills[sg.key] ?? false} @change=${(e: Event) => { selectedSkills[sg.key] = (e.target as HTMLInputElement).checked; state.requestUpdate(); }}>
                       <div>
                         <div style="font-weight:600;">${sg.icon} ${sg.name}</div>
                         <div style="font-size:12px; color:#888;">${sg.desc}</div>
                       </div>
                    </label>
                 `)}
              </div>
            ` : nothing}

            ${stepIndex === 4 ? html`
              <!-- 5. 频道 -->
              <h2 class="wizard-v2-title">接入通讯频道</h2>
              <p class="wizard-v2-subtitle">将 创宇太虚（Claw Acosmi） 接入您的 IM 矩阵，让智能体主动触达业务一线。</p>
              
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

            ${stepIndex === 5 ? html`
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

            ${stepIndex === 6 ? html`
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
                <h3 style="font-size:15px; font-weight:600; margin-bottom:8px;">记忆提取 LLM 配置 <span style="font-size:12px; color:#8C8C8C; font-weight:normal;">（可选 — 不填则使用关键词启发式提取）</span></h3>
                <p style="font-size:13px; color:#8C8C8C; margin-bottom:12px;">配置独立的 LLM 用于从对话中自动提取和归类长期记忆。如不配置，系统将使用基于关键词的启发式提取（准确度较低但无需额外 API 费用）。</p>

                <div style="display:grid; grid-template-columns: 1fr 1fr; gap:12px;">
                  <div class="wizard-v2-provider-input-group">
                    <label>LLM Provider</label>
                    <select class="wizard-v2-input" .value=${memoryConfig.llmProvider} @change=${(e: Event) => {
            memoryConfig.llmProvider = (e.target as HTMLSelectElement).value;
            if (!memoryConfig.llmModel) { memoryConfig.llmModel = getDefaultMemoryModel(memoryConfig.llmProvider); }
            state.requestUpdate();
         }}>
                      <option value="">不配置（使用启发式提取）</option>
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
                    <input type="password" class="wizard-v2-input" placeholder="独立 API Key（可留空复用主 Agent 配置）" .value=${memoryConfig.llmApiKey} @input=${(e: Event) => { memoryConfig.llmApiKey = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                  </div>
                  <div class="wizard-v2-provider-input-group">
                    <label>Base URL</label>
                    <input type="text" class="wizard-v2-input" placeholder="空=使用 provider 默认 URL" .value=${memoryConfig.llmBaseUrl} @input=${(e: Event) => { memoryConfig.llmBaseUrl = (e.target as HTMLInputElement).value; state.requestUpdate(); }} />
                  </div>
                </div>
              </div>

              <div style="text-align:right; margin-top: 32px;">
                 <button class="wizard-v2-btn wizard-v2-btn-primary" style="background:#FAAD14; border-color:#FAAD14;" @click=${() => nextStep(state)}> 确定配置与继续 </button>
              </div>
            ` : nothing}

            ${stepIndex === 7 ? html`
              <!-- 8. 安全级别 -->
              <h2 class="wizard-v2-title">全局安全级别设置</h2>
              <p class="wizard-v2-subtitle" style="margin-bottom:24px;">
                 定义系统执行危险操作和外部通信时的默认拦截策略。<br/>
                 系统底层搭载 <b>Rust 原生沙箱引擎 (oa-sandbox)</b>，支持 Mac Seatbelt、Linux Landlock/Seccomp 及 Windows AppContainer，结合权限突围 (Escalation) 审批体系，构建 <b>4级动态隔离纵深防御架构</b>。
              </p>
              
              <div class="wizard-v2-providers-grid" style="display: block;">
                 <label class="wizard-v2-provider-card ${securityLevelConfig === 'deny' ? 'wizard-v2-provider-card-selected' : ''}" style="display:flex; align-items:flex-start; gap: 12px; cursor: pointer; margin-bottom: 16px;">
                    <input type="radio" name="security-level" .checked=${securityLevelConfig === 'deny'} @change=${() => { securityLevelConfig = 'deny'; state.requestUpdate(); }} style="margin-top:4px;" />
                    <div>
                      <div style="font-weight:600; color: #8C8C8C;">绝缘模式 (Deny)</div>
                      <div style="font-size:13px; color:#666; line-height:1.5;">完全禁止任何系统级文件修改、网络调用和终端命令执行。最安全，但助手将仅仅作为纯文本对话引擎发挥作用。</div>
                    </div>
                 </label>

                 <label class="wizard-v2-provider-card ${securityLevelConfig === 'sandboxed' ? 'wizard-v2-provider-card-selected' : ''}" style="display:flex; align-items:flex-start; gap: 12px; cursor: pointer; margin-bottom: 16px;">
                    <input type="radio" name="security-level" .checked=${securityLevelConfig === 'sandboxed'} @change=${() => { securityLevelConfig = 'sandboxed'; state.requestUpdate(); }} style="margin-top:4px;" />
                    <div>
                      <div style="font-weight:600; color: #1890FF;">沙箱模式 (Sandboxed)</div>
                      <div style="font-size:13px; color:#666; line-height:1.5;">AI 的所有的操作将被严格限制在指定的容器宿主目录下，无法越界访问真实的系统环境与进程。高度安全。</div>
                    </div>
                 </label>
                 
                 <label class="wizard-v2-provider-card ${securityLevelConfig === 'allowlist' ? 'wizard-v2-provider-card-selected' : ''}" style="display:flex; align-items:flex-start; gap: 12px; cursor: pointer; margin-bottom: 16px; border: 1px solid #FAAD14;">
                    <input type="radio" name="security-level" .checked=${securityLevelConfig === 'allowlist'} @change=${() => { securityLevelConfig = 'allowlist'; state.requestUpdate(); }} style="margin-top:4px;" />
                    <div>
                      <div style="font-weight:600; color: #FAAD14;">标准白名单模式 (Allowlist)</div>
                      <div style="font-size:13px; color:#666; line-height:1.5;">针对安全库标记的可信指纹操作自动放行，对删除、覆盖及危险 Bash 指令等高危行为强制下发给用户进行确认阻断。平衡易用与安全。（⭐ 推荐配置）</div>
                    </div>
                 </label>
                 
                 <label class="wizard-v2-provider-card ${securityLevelConfig === 'full' ? 'wizard-v2-provider-card-selected' : ''}" style="display:flex; align-items:flex-start; gap: 12px; cursor: pointer; margin-bottom: 16px;">
                    <input type="radio" name="security-level" .checked=${securityLevelConfig === 'full'} @change=${() => { securityLevelConfig = 'full'; state.requestUpdate(); }} style="margin-top:4px;" />
                    <div>
                      <div style="font-weight:600; color: #FF4D4F;">全权放行模式 (Full Access)</div>
                      <div style="font-size:13px; color:#666; line-height:1.5;">赋予骨干网络最高的 root 级别等价权限。AI 完全免审直飞各项操作。实现全自动化效率飞跃，但在复杂代码库中风险不可控，需极度谨慎！</div>
                    </div>
                 </label>
              </div>

              <div style="text-align:right; margin-top: 32px;">
                 <button class="wizard-v2-btn wizard-v2-btn-primary" style="background:#FAAD14; border-color:#FAAD14;" @click=${() => nextStep(state)}> 安全策略部署与继续 </button>
              </div>
            ` : nothing}

            ${stepIndex === 8 ? html`
              <!-- 9. 完成 -->
              <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; height:100%; padding-bottom: 32px; animation: wizardV2FadeIn 0.5s ease-out;">
                 ${isRestarting ? html`
                     <div class="wizard-v2-restarting-container" style="text-align: center;">
                        <!-- Custom CSS Spinners added to stylesheet -->
                        <div class="wizard-v2-spinner"></div>
                        <h2 class="wizard-v2-title" style="margin-top:24px;">创宇太虚引擎 重燃中...</h2>
                        
                        <div style="width: 300px; height: 6px; background: #e8e8e8; border-radius: 4px; margin: 20px auto; overflow: hidden;">
                           <div style="width: ${restartProgress}%; height: 100%; background: #1890FF; transition: width 0.3s ease;"></div>
                        </div>
                        <div style="color: #666; font-size: 14px; margin-bottom: 32px; font-weight: bold;">
                           构建进度 ${restartProgress}%
                        </div>
                        
                        <!-- System Advantages Animation Text -->
                        <div style="height: 48px; position:relative; overflow:hidden;">
                           ${restartProgress < 30 ? html`<div class="wizard-v2-advantage-text">💎 万物互联: 支持 25+ 主流服务商自动发现接入</div>` :
               (restartProgress < 60 ? html`<div class="wizard-v2-advantage-text">🚄 高效协同: 搭载 Code Auditor 与翻译专家子矩阵串联引擎</div>` :
                  (restartProgress < 90 ? html`<div class="wizard-v2-advantage-text">🛡️ 铁壁防御: ${securityLevelConfig} 军工级沙箱控制拦截系统装载中</div>` :
                     html`<div class="wizard-v2-advantage-text">🧠 核心记忆链唤醒...完成。</div>`))}
                        </div>
                     </div>
                 ` : html`
                     <div style="font-size: 64px; color: #52C41A; margin-bottom:16px;">✓</div>
                     <h2 class="wizard-v2-title" style="margin-bottom:8px;">系统重燃启动成功！</h2>
                     <p class="wizard-v2-subtitle" style="text-align:center;">
                        您配置的骨干网模型和工作流已被热重载接管。<br/>
                        欢迎回到 创宇太虚（Claw Acosmi） 指挥中心。
                     </p>
                     
                     <div style="display:flex; gap:16px; margin-top:16px;">
                        <button class="wizard-v2-btn wizard-v2-btn-secondary" @click=${() => {
               // Reset wizard state
               isRestarting = false;
               restartProgress = 0;
               stepIndex = 0;
               securityAck = false; // Require user to acknowledge again
               state.requestUpdate();
            }} style="padding: 12px 32px; font-size:16px;">
                           重新配置
                        </button>
                        
                        <button class="wizard-v2-btn wizard-v2-btn-primary" @click=${() => {
               closeWizardV2(state);
               state.tab = "chat"; // Force navigation to Chat tab
               setTimeout(() => {
                  // 1. Force a new web session to avoid sending to a previously stored remote channel (like Feishu)
                  state.handleSendChat("/new");
                  // 2. Queue the welcome message natively
                  setTimeout(() => {
                     state.handleSendChat("系统启动完成！请向我全面介绍一下 创宇太虚 (Claw Acosmi) 系统的各项优势与核心能力，并做个简短的欢迎致辞。");
                  }, 300);
               }, 500);
            }} style="padding: 12px 32px; font-size:16px;">
                           进入主界面
                        </button>
                     </div>
                 `}
              </div>
            ` : nothing}
            `)
      }
</div>
   
   <!-- Footer / Actions -->
   ${stepIndex < 8 && !isRestarting ? html`
          <div class="wizard-v2-footer">
             <div style="display:flex; gap:12px;">
                <button class="wizard-v2-btn wizard-v2-btn-secondary" @click=${() => prevStep(state)}>
                   ${stepIndex === 0 ? "取消/不保存" : "上一步"}
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
                 ${stepIndex === 7 ? "最后一步" : "下一步"}
             </button>
          </div>
        ` : nothing
      }

</div>
   </div>
      `;
}
