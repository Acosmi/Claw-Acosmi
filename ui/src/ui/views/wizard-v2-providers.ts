// wizard-v2-providers.ts
// Provider UI 视觉映射 + 后端数据合并函数。
// 数据来源：后端 wizard.v2.providers.list RPC（唯一数据源）。
// 前端仅保留 icon/color/bg 等视觉属性。

import { html } from "lit";
import { normalizeBasePath } from "../navigation.ts";

// ---------- 后端返回的类型定义 ----------

export interface WizardModelEntry {
    id: string;
    name: string;
    reasoning: boolean;
    input: string[];
    contextWindow: number;
    maxTokens: number;
}

export interface WizardProvider {
    id: string;
    name: string;
    desc: string;
    category: string;
    sortOrder: number;
    authModes: ("apiKey" | "oauth" | "deviceCode" | "none")[];
    defaultModelRef: string;
    models: WizardModelEntry[];
    customBaseUrlAllowed: boolean;
    requiresBaseUrl: boolean;
    // 前端 UI 属性（mergeWithUI 后添加）
    icon: any;
    color: string;
    bg: string;
}

// ---------- 前端唯一硬编码：视觉属性映射 ----------

function publicAssetUrl(basePath: string, assetPath: string): string {
    const base = normalizeBasePath(basePath);
    return base ? `${base}${assetPath}` : assetPath;
}

type ProviderIconRenderer = (basePath: string) => any;

const PROVIDER_UI_META: Record<string, { icon: ProviderIconRenderer | any; color: string; bg: string }> = {
    "google": {
        icon: (basePath) => html`<img src="${publicAssetUrl(basePath, "/providers/google.svg")}" style="width:75%; height:75%; object-fit:contain;" />`,
        color: "#1890FF", bg: "#E6F7FF"
    },
    "qwen": {
        icon: html`<div style="font-weight:bold; font-size:24px; color:#722ED1; display:flex; align-items:center; justify-content:center; height:100%;">Q</div>`,
        color: "#722ED1", bg: "#F9F0FF"
    },
    "github-copilot": {
        icon: html`<div style="font-weight:bold; font-size:18px; color:#24292e; display:flex; align-items:center; justify-content:center; height:100%;">GH</div>`,
        color: "#24292e", bg: "#f6f8fa"
    },
    "minimax": {
        icon: html`<div style="font-size:24px; display:flex; align-items:center; justify-content:center; height:100%;">🐚</div>`,
        color: "#FAAD14", bg: "#FFFBE6"
    },
    "deepseek": {
        icon: html`<div style="font-size:24px; display:flex; align-items:center; justify-content:center; height:100%;">🐳</div>`,
        color: "#13C2C2", bg: "#E6FFFB"
    },
    "doubao": {
        icon: html`<div style="font-weight:bold; font-size:18px; color:#1890FF; display:flex; align-items:center; justify-content:center; height:100%;">豆</div>`,
        color: "#1890FF", bg: "#E6F7FF"
    },
    "zhipu": {
        icon: html`<div style="font-weight:bold; font-size:22px; color:#2F54EB; display:flex; align-items:center; justify-content:center; height:100%;">G</div>`,
        color: "#2F54EB", bg: "#F0F5FF"
    },
    "moonshot": {
        icon: (basePath) => html`<img src="${publicAssetUrl(basePath, "/providers/kimi.ico")}" style="width:100%; height:100%; object-fit:contain; border-radius: 50%;" />`,
        color: "#1890FF", bg: "#E6F7FF"
    },
    "openai": {
        icon: (basePath) => html`<img src="${publicAssetUrl(basePath, "/providers/openai.svg")}" style="width:70%; height:70%; object-fit:contain; filter: invert(0);" />`,
        color: "#52C41A", bg: "#F6FFED"
    },
    "anthropic": {
        icon: (basePath) => html`<img src="${publicAssetUrl(basePath, "/providers/anthropic.svg")}" style="width:60%; height:60%; object-fit:contain; filter: invert(0);" />`,
        color: "#FF4D4F", bg: "#FFF1F0"
    },
    "xai": {
        icon: (basePath) => html`<img src="${publicAssetUrl(basePath, "/providers/xai.svg")}" style="width:70%; height:70%; object-fit:contain;" />`,
        color: "#000000", bg: "#F5F5F5"
    },
    "qianfan": {
        icon: html`<div style="font-weight:bold; font-size:18px; color:#2932E1; display:flex; align-items:center; justify-content:center; height:100%;">百</div>`,
        color: "#2932E1", bg: "#EFF1FF"
    },
    "xiaomi": {
        icon: html`<div style="font-weight:bold; font-size:18px; color:#FF6900; display:flex; align-items:center; justify-content:center; height:100%;">Mi</div>`,
        color: "#FF6900", bg: "#FFF4EC"
    },
    "kimi-coding": {
        icon: html`<div style="font-weight:bold; font-size:14px; color:#1890FF; display:flex; align-items:center; justify-content:center; height:100%;">K·C</div>`,
        color: "#1890FF", bg: "#E6F7FF"
    },
    "mistral": {
        icon: html`<div style="font-weight:bold; font-size:18px; color:#F97316; display:flex; align-items:center; justify-content:center; height:100%;">M</div>`,
        color: "#F97316", bg: "#FFF7ED"
    },
    "openrouter": {
        icon: html`<div style="font-weight:bold; font-size:14px; color:#6366F1; display:flex; align-items:center; justify-content:center; height:100%;">OR</div>`,
        color: "#6366F1", bg: "#EEF2FF"
    },
    "together": {
        icon: html`<div style="font-weight:bold; font-size:14px; color:#10B981; display:flex; align-items:center; justify-content:center; height:100%;">T</div>`,
        color: "#10B981", bg: "#ECFDF5"
    },
    "huggingface": {
        icon: html`<div style="font-size:22px; display:flex; align-items:center; justify-content:center; height:100%;">🤗</div>`,
        color: "#FFD21E", bg: "#FFFBEB"
    },
    "litellm": {
        icon: html`<div style="font-weight:bold; font-size:14px; color:#8B5CF6; display:flex; align-items:center; justify-content:center; height:100%;">LL</div>`,
        color: "#8B5CF6", bg: "#F5F3FF"
    },
    "byteplus": {
        icon: html`<div style="font-weight:bold; font-size:14px; color:#3B82F6; display:flex; align-items:center; justify-content:center; height:100%;">B+</div>`,
        color: "#3B82F6", bg: "#EFF6FF"
    },
    "custom-openai": {
        icon: html`<div style="font-weight:bold; font-size:12px; color:#722ED1; display:flex; align-items:center; justify-content:center; height:100%;">API</div>`,
        color: "#722ED1", bg: "#F9F0FF"
    },
    "ollama": {
        icon: html`<div style="font-size:24px; display:flex; align-items:center; justify-content:center; height:100%;">🦙</div>`,
        color: "#FA8C16", bg: "#FFF2E8"
    },
};

const DEFAULT_UI = {
    icon: () => html`<div style="font-weight:bold; font-size:14px; color:#8C8C8C; display:flex; align-items:center; justify-content:center; height:100%;">?</div>`,
    color: "#8C8C8C",
    bg: "#F5F5F5"
};

// ---------- 合并函数 ----------

/** 合并后端数据 + 前端视觉属性 */
export function mergeWithUI(backendProviders: any[], basePath: string = ""): WizardProvider[] {
    return backendProviders.map((p) => {
        const ui = PROVIDER_UI_META[p.id] || DEFAULT_UI;
        const icon = typeof ui.icon === "function" ? ui.icon(basePath) : ui.icon;
        return {
            ...p,
            icon,
            color: ui.color,
            bg: ui.bg,
        };
    });
}

// ---------- 静态 fallback（后端不可用时兜底） ----------

const FALLBACK_PROVIDER_DATA = [
    // --- 优先推荐组 ---
    { id: "google", name: "Google (Gemini)", desc: "Gemini 系列模型，多模态能力出众。支持 OAuth 一键授权或 API Key。", category: "oauth_priority", sortOrder: 1, authModes: ["oauth", "apiKey"], defaultModelRef: "google/gemini-3.1-pro-preview", models: [{ id: "gemini-3.1-pro-preview", name: "Gemini 3.1 Pro", reasoning: true, input: ["text", "image"], contextWindow: 1048576, maxTokens: 65536 }, { id: "gemini-3-flash-preview", name: "Gemini 3 Flash", reasoning: false, input: ["text", "image"], contextWindow: 1048576, maxTokens: 65536 }, { id: "gemini-2.5-pro", name: "Gemini 2.5 Pro", reasoning: true, input: ["text", "image"], contextWindow: 1048576, maxTokens: 65536 }, { id: "gemini-2.5-flash", name: "Gemini 2.5 Flash", reasoning: true, input: ["text", "image"], contextWindow: 1048576, maxTokens: 65536 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "qwen", name: "Qwen 通义千问 (阿里)", desc: "通义千问大模型，中文开源天花板。支持 Portal 设备码授权或 API Key。", category: "oauth_priority", sortOrder: 2, authModes: ["deviceCode", "apiKey"], defaultModelRef: "qwen/qwen3.5-plus", models: [{ id: "qwen3.5-plus", name: "Qwen3.5 Plus (旗舰多模态)", reasoning: false, input: ["text", "image"], contextWindow: 1000000, maxTokens: 81920 }, { id: "qwen3-max", name: "Qwen3 Max (旗舰推理)", reasoning: true, input: ["text"], contextWindow: 256000, maxTokens: 32000 }, { id: "qwen3-coder-plus", name: "Qwen3 Coder Plus (代码)", reasoning: false, input: ["text"], contextWindow: 262000, maxTokens: 65536 }, { id: "qwen3-235b-a22b", name: "Qwen 3 235B (开源旗舰)", reasoning: true, input: ["text"], contextWindow: 131072, maxTokens: 16384 }, { id: "qwq-plus", name: "QwQ Plus (推理专家)", reasoning: true, input: ["text"], contextWindow: 131072, maxTokens: 16384 }, { id: "qwen-max", name: "Qwen Max (经典)", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 16384 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "github-copilot", name: "GitHub Copilot", desc: "基于 GitHub Copilot 订阅。通过 Device Flow 设备码授权登录。", category: "oauth_priority", sortOrder: 3, authModes: ["deviceCode"], defaultModelRef: "github-copilot/gpt-4o", models: [{ id: "gpt-4o", name: "GPT-4o (Copilot)", reasoning: false, input: ["text", "image"], contextWindow: 128000, maxTokens: 8192 }, { id: "claude-sonnet-4.6", name: "Claude Sonnet 4.6 (Copilot)", reasoning: false, input: ["text", "image"], contextWindow: 128000, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "minimax", name: "MiniMax (海螺)", desc: "海螺大模型，长文本处理专家。", category: "oauth_priority", sortOrder: 4, authModes: ["deviceCode", "apiKey"], defaultModelRef: "minimax/MiniMax-M2.5", models: [{ id: "MiniMax-M2.5", name: "MiniMax-M2.5", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    // --- 国内主力组 ---
    { id: "deepseek", name: "DeepSeek", desc: "深度求索系列，国产之光。", category: "china_major", sortOrder: 1, authModes: ["apiKey"], defaultModelRef: "deepseek/deepseek-chat", models: [{ id: "deepseek-chat", name: "DeepSeek V3", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }, { id: "deepseek-reasoner", name: "DeepSeek R1", reasoning: true, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "doubao", name: "Doubao (字节跳动 Ark)", desc: "字节跳动火山引擎主力大模型。", category: "china_major", sortOrder: 2, authModes: ["apiKey"], defaultModelRef: "volcengine/ark-code-latest", models: [{ id: "ark-code-latest", name: "ARK Code Latest", reasoning: true, input: ["text"], contextWindow: 131072, maxTokens: 8192 }, { id: "doubao-1-5-pro-32k", name: "Doubao 1.5 Pro 32K", reasoning: false, input: ["text"], contextWindow: 32768, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "zhipu", name: "智谱 (Zhipu / Z.AI)", desc: "GLM 系列模型。", category: "china_major", sortOrder: 3, authModes: ["apiKey"], defaultModelRef: "zai/glm-5", models: [{ id: "glm-5", name: "GLM-5", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "moonshot", name: "Kimi (月之暗面)", desc: "长文本处理专家。", category: "china_major", sortOrder: 4, authModes: ["apiKey"], defaultModelRef: "moonshot/kimi-k2.5", models: [{ id: "kimi-k2.5", name: "Kimi K2.5", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    // --- 国际主力组 ---
    { id: "openai", name: "OpenAI", desc: "行业标杆 GPT 与 o 系列推理模型。", category: "international", sortOrder: 1, authModes: ["apiKey"], defaultModelRef: "openai/gpt-4.1", models: [{ id: "gpt-4.1", name: "GPT-4.1", reasoning: false, input: ["text", "image"], contextWindow: 1047576, maxTokens: 32768 }, { id: "o4-mini", name: "o4-mini", reasoning: true, input: ["text", "image"], contextWindow: 200000, maxTokens: 100000 }, { id: "gpt-4.1-mini", name: "GPT-4.1 mini", reasoning: false, input: ["text", "image"], contextWindow: 1047576, maxTokens: 16384 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "anthropic", name: "Anthropic", desc: "Claude 系列模型，顶级代码与复杂推理能力。", category: "international", sortOrder: 2, authModes: ["apiKey"], defaultModelRef: "anthropic/claude-sonnet-4-6", models: [{ id: "claude-sonnet-4-6", name: "Claude Sonnet 4.6", reasoning: true, input: ["text", "image"], contextWindow: 200000, maxTokens: 16384 }, { id: "claude-opus-4-6", name: "Claude Opus 4.6", reasoning: true, input: ["text", "image"], contextWindow: 200000, maxTokens: 32000 }, { id: "claude-haiku-4-5-20251001", name: "Claude Haiku 4.5", reasoning: true, input: ["text", "image"], contextWindow: 200000, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "xai", name: "xAI Grok", desc: "Grok 系列模型。", category: "international", sortOrder: 3, authModes: ["apiKey"], defaultModelRef: "xai/grok-4", models: [{ id: "grok-4", name: "Grok 4", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    // --- 新兴平台组 ---
    { id: "qianfan", name: "百度千帆 (Qianfan)", desc: "ERNIE 系列大模型。", category: "emerging", sortOrder: 1, authModes: ["apiKey"], defaultModelRef: "qianfan/ernie-x1-turbo-32k", models: [{ id: "ernie-x1-turbo-32k", name: "ERNIE X1 Turbo", reasoning: false, input: ["text"], contextWindow: 32768, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "xiaomi", name: "小米 MiMo", desc: "小米自研大模型。", category: "emerging", sortOrder: 2, authModes: ["apiKey"], defaultModelRef: "xiaomi/mimo-v2-flash", models: [{ id: "mimo-v2-flash", name: "MiMo V2 Flash", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "kimi-coding", name: "Kimi Coding", desc: "代码补全与重构。", category: "emerging", sortOrder: 3, authModes: ["apiKey"], defaultModelRef: "kimi-coding/k2p5", models: [{ id: "k2p5", name: "K2P5", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "mistral", name: "Mistral AI", desc: "欧洲开源模型厂商。", category: "emerging", sortOrder: 4, authModes: ["apiKey"], defaultModelRef: "mistral/mistral-large-latest", models: [{ id: "mistral-large-latest", name: "Mistral Large", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    // --- 聚合平台组 ---
    { id: "openrouter", name: "OpenRouter", desc: "多模型统一网关路由。", category: "aggregator", sortOrder: 1, authModes: ["apiKey"], defaultModelRef: "openrouter/auto", models: [{ id: "auto", name: "Auto", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "together", name: "Together AI", desc: "高性价比推理平台。", category: "aggregator", sortOrder: 2, authModes: ["apiKey"], defaultModelRef: "together/moonshotai/Kimi-K2.5", models: [{ id: "kimi-k2.5", name: "Kimi K2.5", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "huggingface", name: "Hugging Face", desc: "全球最大开源模型社区。", category: "aggregator", sortOrder: 3, authModes: ["apiKey"], defaultModelRef: "huggingface/deepseek-ai/DeepSeek-R1", models: [{ id: "deepseek-r1", name: "DeepSeek R1", reasoning: true, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "litellm", name: "LiteLLM Proxy", desc: "轻量级多模型代理。", category: "aggregator", sortOrder: 4, authModes: ["apiKey"], defaultModelRef: "litellm/claude-opus-4-6", models: [{ id: "claude-opus-4-6", name: "Claude Opus 4.6", reasoning: true, input: ["text"], contextWindow: 200000, maxTokens: 32000 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    { id: "byteplus", name: "BytePlus (海外火山)", desc: "字节跳动海外版。", category: "aggregator", sortOrder: 5, authModes: ["apiKey"], defaultModelRef: "volcengine/ark-code-latest", models: [{ id: "doubao-1-5-pro-32k", name: "Doubao 1.5 Pro 32K", reasoning: false, input: ["text"], contextWindow: 32768, maxTokens: 8192 }], customBaseUrlAllowed: false, requiresBaseUrl: false },
    // --- 本地推理与自定义组 ---
    { id: "custom-openai", name: "自定义端点 (OpenAI 兼容)", desc: "支持接入 OpenRouter, vLLM 等符合 OpenAI 协议的自定义服务。", category: "local_custom", sortOrder: 1, authModes: ["apiKey"], defaultModelRef: "openai-compat/custom", models: [{ id: "custom", name: "自定义模型ID请在下方填入", reasoning: false, input: ["text"], contextWindow: 128000, maxTokens: 8192 }], customBaseUrlAllowed: true, requiresBaseUrl: true },
    { id: "ollama", name: "Ollama (本地私有化)", desc: "零配置接入本地私有化模型体系。", category: "local_custom", sortOrder: 2, authModes: ["none"], defaultModelRef: "ollama/llama3.3", models: [{ id: "llama3.3", name: "Llama 3.3", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 8192 }, { id: "qwen3:32b", name: "Qwen 3 32B", reasoning: true, input: ["text"], contextWindow: 131072, maxTokens: 8192 }], customBaseUrlAllowed: true, requiresBaseUrl: false },
];

/** @deprecated 仅作为后端不可用时的兜底。数据源应为后端 wizard.v2.providers.list。 */
export function getFallbackProviders(basePath: string = ""): WizardProvider[] {
    return mergeWithUI(FALLBACK_PROVIDER_DATA, basePath);
}

export const FALLBACK_PROVIDERS: WizardProvider[] = getFallbackProviders();

// ---------- 旧接口兼容 ----------

/** @deprecated 使用 WizardProvider 替代 */
export type AIProvider = WizardProvider;

/** @deprecated 使用动态加载替代。保留给不支持后端 RPC 的场景。 */
export const PROVIDERS = FALLBACK_PROVIDERS;
