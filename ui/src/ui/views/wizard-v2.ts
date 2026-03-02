import { html, nothing } from "lit";
import type { AppViewState } from "../app-view-state.ts";

// ─── Controller functions ───

export function startWizardV2(state: AppViewState): void {
    state.wizardV2Open = true;
}

export function closeWizardV2(state: AppViewState): void {
    state.wizardV2Open = false;
}

// ─── Render ───

export function renderWizardV2(state: AppViewState) {
    if (!state.wizardV2Open) return nothing;

    // Render a completely separate prototype overlay that won't touch existing logic
    return html`
    <div class="wizard-v2-overlay" @click=${(e: Event) => {
            if (e.target === e.currentTarget) {
                closeWizardV2(state);
            }
        }}>
      <div class="wizard-v2-card">
        
        <!-- Header / Progress Bar -->
        <div class="wizard-v2-header">
           <div class="wizard-v2-step-indicator">
              <div class="wizard-v2-step completed">
                <div class="wizard-v2-step-circle">1</div>
                <div class="wizard-v2-step-label">欢迎</div>
              </div>
              <div class="wizard-v2-step-connector completed"></div>
              
              <div class="wizard-v2-step active">
                <div class="wizard-v2-step-circle">2</div>
                <div class="wizard-v2-step-label">AI 服务商</div>
              </div>
              <div class="wizard-v2-step-connector"></div>
              
              <div class="wizard-v2-step pending">
                <div class="wizard-v2-step-circle">3</div>
                <div class="wizard-v2-step-label">技能</div>
              </div>
              <div class="wizard-v2-step-connector"></div>
              
              <div class="wizard-v2-step pending">
                <div class="wizard-v2-step-circle">4</div>
                <div class="wizard-v2-step-label">环境变量</div>
              </div>
              <div class="wizard-v2-step-connector"></div>
              
              <div class="wizard-v2-step pending">
                <div class="wizard-v2-step-circle">5</div>
                <div class="wizard-v2-step-label">完成</div>
              </div>
           </div>
        </div>

        <!-- Body / Content -->
        <div class="wizard-v2-body">
            <!-- 占位符标题 -->
            <h2 class="wizard-v2-title">配置 AI 服务商</h2>
            <p class="wizard-v2-subtitle">
               所有主流 AI 服务商已预配置完成，您只需填入对应的 API Key 即可启用。
               <span class="wizard-v2-highlight">至少需要配置一个服务商。</span>
            </p>

            <div class="wizard-v2-providers-grid">
               <!-- Card 1 -->
               <div class="wizard-v2-provider-card">
                  <div class="wizard-v2-provider-header">
                     <div class="wizard-v2-provider-icon" style="background: #FFF1F0; color: #FF4D4F;">🧠</div>
                     <div class="wizard-v2-provider-info">
                        <div class="wizard-v2-provider-name">Anthropic</div>
                        <div class="wizard-v2-provider-desc">Claude 系列模型，推荐用于代码和复杂推理</div>
                     </div>
                     <div class="wizard-v2-provider-badge">未配置</div>
                  </div>
                  <div class="wizard-v2-provider-recommendation">
                     推荐模型: <code>anthropic/claude-sonnet-4-20250514</code>
                  </div>
                  <div class="wizard-v2-provider-input-group">
                     <label>API Key</label>
                     <input type="password" placeholder="输入 ANTHROPIC_API_KEY" class="wizard-v2-input" />
                  </div>
               </div>

               <!-- Card 2 -->
               <div class="wizard-v2-provider-card">
                  <div class="wizard-v2-provider-header">
                     <div class="wizard-v2-provider-icon" style="background: #F6FFED; color: #52C41A;">🤖</div>
                     <div class="wizard-v2-provider-info">
                        <div class="wizard-v2-provider-name">OpenAI</div>
                        <div class="wizard-v2-provider-desc">GPT 系列模型，通用能力强</div>
                     </div>
                     <div class="wizard-v2-provider-badge">未配置</div>
                  </div>
                  <div class="wizard-v2-provider-recommendation">
                     推荐模型: <code>openai/gpt-4o</code>
                  </div>
                  <div class="wizard-v2-provider-input-group">
                     <label>API Key</label>
                     <input type="password" placeholder="输入 OPENAI_API_KEY" class="wizard-v2-input" />
                  </div>
               </div>
            </div>
        </div>

        <!-- Footer / Actions -->
        <div class="wizard-v2-footer">
           <button class="wizard-v2-btn wizard-v2-btn-secondary" @click=${() => closeWizardV2(state)}>上一步/关闭</button>
           <button class="wizard-v2-btn wizard-v2-btn-primary">下一步</button>
        </div>

      </div>
    </div>
  `;
}
