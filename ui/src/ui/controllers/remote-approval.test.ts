import { describe, expect, it, vi } from "vitest";
import {
  loadRemoteApproval,
  saveRemoteApproval,
  testRemoteApproval,
  type RemoteApprovalState,
} from "./remote-approval.ts";

function makeState(overrides: Partial<RemoteApprovalState> = {}): RemoteApprovalState {
  return {
    client: null,
    connected: true,
    remoteApprovalLoading: false,
    remoteApprovalError: null,
    remoteApprovalEnabled: false,
    remoteApprovalCallbackUrl: "",
    remoteApprovalEnabledProviders: [],
    remoteApprovalFeishuEnabled: false,
    remoteApprovalFeishuAppId: "",
    remoteApprovalFeishuAppSecret: "",
    remoteApprovalFeishuChatId: "",
    remoteApprovalDingtalkEnabled: false,
    remoteApprovalDingtalkWebhookUrl: "",
    remoteApprovalDingtalkWebhookSecret: "",
    remoteApprovalWecomEnabled: false,
    remoteApprovalWecomCorpId: "",
    remoteApprovalWecomAgentId: "",
    remoteApprovalWecomSecret: "",
    remoteApprovalWecomToUser: "",
    remoteApprovalWecomToParty: "",
    remoteApprovalTestLoading: false,
    remoteApprovalTestResult: null,
    remoteApprovalTestError: null,
    remoteApprovalSaving: false,
    remoteApprovalSaved: false,
    ...overrides,
  };
}

describe("remote-approval controller", () => {
  it("loads and maps remote approval config", async () => {
    const request = vi.fn(async () => ({
      enabled: true,
      callbackUrl: "https://callback.example",
      enabledProviders: ["feishu", "wecom"],
      feishu: { enabled: true, appId: "app-id", appSecret: "***", chatId: "oc_chat" },
      dingtalk: { enabled: false, webhookUrl: "", webhookSecret: "" },
      wecom: {
        enabled: true,
        corpId: "corp-id",
        agentId: 10001,
        secret: "***",
        toUser: "@all",
        toParty: "",
      },
    }));

    const state = makeState({
      client: { request } as any,
    });

    await loadRemoteApproval(state);

    expect(state.remoteApprovalEnabled).toBe(true);
    expect(state.remoteApprovalCallbackUrl).toBe("https://callback.example");
    expect(state.remoteApprovalEnabledProviders).toEqual(["feishu", "wecom"]);
    expect(state.remoteApprovalFeishuEnabled).toBe(true);
    expect(state.remoteApprovalFeishuAppId).toBe("app-id");
    expect(state.remoteApprovalFeishuAppSecret).toBe("***");
    expect(state.remoteApprovalFeishuChatId).toBe("oc_chat");
    expect(state.remoteApprovalWecomEnabled).toBe(true);
    expect(state.remoteApprovalWecomAgentId).toBe("10001");
  });

  it("saves config and refreshes snapshot", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce({ status: "saved" })
      .mockResolvedValueOnce({
        enabled: true,
        callbackUrl: "https://saved.example",
        enabledProviders: ["feishu"],
        feishu: { enabled: true, appId: "saved-app", appSecret: "***", chatId: "oc_saved" },
      });

    const state = makeState({
      client: { request } as any,
      remoteApprovalEnabled: true,
      remoteApprovalCallbackUrl: "https://saved.example",
      remoteApprovalFeishuEnabled: true,
      remoteApprovalFeishuAppId: "saved-app",
      remoteApprovalFeishuAppSecret: "***",
      remoteApprovalFeishuChatId: "oc_saved",
    });

    await saveRemoteApproval(state);

    expect(request).toHaveBeenNthCalledWith(1, "security.remoteApproval.config.set", expect.any(Object));
    expect(request).toHaveBeenNthCalledWith(2, "security.remoteApproval.config.get", {});
    expect(state.remoteApprovalSaved).toBe(true);
    expect(state.remoteApprovalError).toBeNull();
  });

  it("tests provider and sets result", async () => {
    const request = vi.fn(async () => ({ status: "success" }));
    const state = makeState({ client: { request } as any });

    await testRemoteApproval(state, "feishu");

    expect(request).toHaveBeenCalledWith("security.remoteApproval.test", { provider: "feishu" });
    expect(state.remoteApprovalTestError).toBeNull();
    expect(state.remoteApprovalTestResult).toContain("feishu");
  });
});
