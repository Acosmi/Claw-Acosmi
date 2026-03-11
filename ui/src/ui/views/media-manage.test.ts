import { render } from "lit";
import { describe, expect, it } from "vitest";
import type { AppViewState } from "../app-view-state.ts";
import { initLocale } from "../i18n.ts";
import { renderMediaManage } from "./media-manage.ts";

initLocale("zh");

function createState(subTab: AppViewState["mediaManageSubTab"]): AppViewState {
  return {
    basePath: "/ui",
    mediaManageSubTab: subTab,
    mediaConfig: {
      agent_id: "oa-media",
      label: "媒体运营智能体",
      status: "configured",
      trending_sources: [
        { name: "weibo", enabled: true, configured: true, status: "configured" },
        { name: "bocha", enabled: true, configured: false, status: "configured", source_configured: true, requires_credential: true },
        { name: "custom_openai", enabled: true, configured: false, status: "configured", source_configured: true, requires_credential: true },
      ],
      trending_source_profiles: {
        bocha: {
          configured: true,
          status: "configured",
          authMode: "api_key",
          apiKey: "boch****1234",
          baseUrl: "https://api.bochaai.com",
          freshness: "oneDay",
        },
        custom_openai: {
          configured: true,
          status: "configured",
          authMode: "api_key",
          apiKey: "sk-o****1234",
          baseUrl: "https://example.com/v1",
          model: "sonar-pro",
          systemPrompt: "Return JSON only.",
          requestExtras: "{\"web_search\":true}",
        },
      },
      tools: [
        { name: "trending_topics", description: "热点发现", enabled: true, status: "builtin", scope: "media" },
        { name: "media_publish", description: "发布", enabled: true, status: "configured", scope: "media" },
      ],
      publishers: ["wechat", "xiaohongshu"],
      publisher_profiles: {
        wechat: {
          enabled: true,
          configured: true,
          status: "configured",
          accountName: "品牌公众号",
          appId: "wx123",
          appSecret: "secr****1234",
          tokenCachePath: "_media/wechat/token.json",
        },
        xiaohongshu: {
          enabled: true,
          configured: true,
          status: "configured",
          accountName: "品牌小红书",
          cookiePath: "/tmp/xhs-cookie.json",
          authStatus: "authenticated",
          authMessage: "检测到有效 Cookie，可直接发布和互动。",
          authUpdatedAt: "2026-03-09T08:00:00Z",
          browserReady: true,
          autoInteractInterval: 30,
          rateLimitSeconds: 5,
          errorScreenshotDir: "_media/xhs/errors",
        },
        website: {
          enabled: true,
          configured: true,
          status: "configured",
          siteName: "品牌官网",
          apiUrl: "https://example.com/api/posts",
          authType: "bearer",
          authToken: "webt****1234",
          timeoutSeconds: 30,
          maxRetries: 3,
        },
      },
      publish_enabled: true,
      publish_configured: true,
      llm: {
        provider: "deepseek",
        model: "deepseek-chat",
        apiKey: "sk-****",
        baseUrl: "https://api.deepseek.com",
        autoSpawnEnabled: true,
        maxAutoSpawnsPerDay: 5,
      },
      trending_strategy: {
        hotKeywords: ["AI"],
        monitorIntervalMin: 30,
        trendingThreshold: 10000,
        contentCategories: ["科技"],
        autoDraftEnabled: true,
      },
      enabled_sources: ["weibo"],
      enabled_sources_configured: true,
    },
    mediaDrafts: [],
    mediaHeartbeat: {
      lastPatrolAt: null,
      nextPatrolAt: null,
      activeJobId: null,
      lastError: null,
      autoSpawnCount: 2,
    },
    mediaDraftDetail: null,
    mediaDraftDetailLoading: false,
    mediaPublishDetail: null,
    mediaPublishDetailLoading: false,
    mediaDraftEdit: null,
    requestUpdate: () => undefined,
  } as unknown as AppViewState;
}

describe("media manage view", () => {
  it("renders the summary hero only in the overview tab", () => {
    window.history.replaceState({}, "", "/");
    const overview = document.createElement("div");
    render(renderMediaManage(createState("overview")), overview);
    expect(overview.querySelector(".media-manage__hero-panel")).not.toBeNull();

    window.history.replaceState({}, "", "/");
    const tools = document.createElement("div");
    render(renderMediaManage(createState("tools")), tools);
    expect(tools.querySelector(".media-manage__hero-panel")).toBeNull();
    expect(tools.querySelector(".media-manage__subtabs")).not.toBeNull();
  });

  it("renders model and account fields in the llm tab", () => {
    window.history.replaceState({}, "", "/?mediaSubTab=llm");
    const container = document.createElement("div");
    render(renderMediaManage(createState("llm")), container);

    expect(container.textContent).toContain("媒体账号与发布器");
    expect(container.textContent).toContain("AppID");
    expect(container.textContent).toContain("Cookie 文件路径");
    expect(container.textContent).toContain("认证 Token");
    expect(container.textContent).toContain("热点配置入口");
    expect(container.textContent).toContain("自动登录执行器");
    expect(container.textContent).toContain("检查并保存登录态");
  });

  it("renders Bocha API hotspot configuration in the sources tab", () => {
    window.history.replaceState({}, "", "/?mediaSubTab=sources");
    const container = document.createElement("div");
    render(renderMediaManage(createState("sources")), container);

    expect(container.textContent).toContain("Bocha API 热点发现");
    expect(container.textContent).toContain("自定义 OpenAI 兼容热点源");
    expect(container.textContent).toContain("API Key");
    expect(container.textContent).toContain("Freshness");
    expect(container.textContent).toContain("Base URL");
    expect(container.textContent).toContain("Request Extras");
    expect(container.textContent).toContain("Model");
  });
});
