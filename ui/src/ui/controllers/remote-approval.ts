import type { GatewayBrowserClient } from "../gateway.ts";

export type RemoteApprovalProvider = "feishu" | "dingtalk" | "wecom";

export interface RemoteApprovalState {
  client: GatewayBrowserClient | null;
  connected: boolean;
  remoteApprovalLoading: boolean;
  remoteApprovalError: string | null;
  remoteApprovalEnabled: boolean;
  remoteApprovalCallbackUrl: string;
  remoteApprovalEnabledProviders: string[];
  remoteApprovalFeishuEnabled: boolean;
  remoteApprovalFeishuAppId: string;
  remoteApprovalFeishuAppSecret: string;
  remoteApprovalFeishuChatId: string;
  remoteApprovalDingtalkEnabled: boolean;
  remoteApprovalDingtalkWebhookUrl: string;
  remoteApprovalDingtalkWebhookSecret: string;
  remoteApprovalWecomEnabled: boolean;
  remoteApprovalWecomCorpId: string;
  remoteApprovalWecomAgentId: string;
  remoteApprovalWecomSecret: string;
  remoteApprovalWecomToUser: string;
  remoteApprovalWecomToParty: string;
  remoteApprovalTestLoading: boolean;
  remoteApprovalTestResult: string | null;
  remoteApprovalTestError: string | null;
  remoteApprovalSaving: boolean;
  remoteApprovalSaved: boolean;
}

type RemoteApprovalConfigResult = {
  enabled?: boolean;
  callbackUrl?: string;
  enabledProviders?: string[];
  feishu?: {
    enabled?: boolean;
    appId?: string;
    appSecret?: string;
    chatId?: string;
  };
  dingtalk?: {
    enabled?: boolean;
    webhookUrl?: string;
    webhookSecret?: string;
  };
  wecom?: {
    enabled?: boolean;
    corpId?: string;
    agentId?: number;
    secret?: string;
    toUser?: string;
    toParty?: string;
  };
};

function asString(v: unknown): string {
  return typeof v === "string" ? v : "";
}

export async function loadRemoteApproval(state: RemoteApprovalState): Promise<void> {
  if (!state.client || !state.connected) {
    return;
  }
  state.remoteApprovalLoading = true;
  state.remoteApprovalError = null;
  try {
    const result = await state.client.request<RemoteApprovalConfigResult>(
      "security.remoteApproval.config.get",
      {},
    );

    state.remoteApprovalEnabled = result.enabled === true;
    state.remoteApprovalCallbackUrl = asString(result.callbackUrl);
    state.remoteApprovalEnabledProviders = Array.isArray(result.enabledProviders)
      ? result.enabledProviders.filter((p): p is string => typeof p === "string")
      : [];

    const feishu = result.feishu ?? {};
    state.remoteApprovalFeishuEnabled = feishu.enabled === true;
    state.remoteApprovalFeishuAppId = asString(feishu.appId);
    state.remoteApprovalFeishuAppSecret = asString(feishu.appSecret);
    state.remoteApprovalFeishuChatId = asString(feishu.chatId);

    const dingtalk = result.dingtalk ?? {};
    state.remoteApprovalDingtalkEnabled = dingtalk.enabled === true;
    state.remoteApprovalDingtalkWebhookUrl = asString(dingtalk.webhookUrl);
    state.remoteApprovalDingtalkWebhookSecret = asString(dingtalk.webhookSecret);

    const wecom = result.wecom ?? {};
    state.remoteApprovalWecomEnabled = wecom.enabled === true;
    state.remoteApprovalWecomCorpId = asString(wecom.corpId);
    state.remoteApprovalWecomAgentId =
      typeof wecom.agentId === "number" && Number.isFinite(wecom.agentId)
        ? String(wecom.agentId)
        : "";
    state.remoteApprovalWecomSecret = asString(wecom.secret);
    state.remoteApprovalWecomToUser = asString(wecom.toUser);
    state.remoteApprovalWecomToParty = asString(wecom.toParty);
  } catch (err) {
    state.remoteApprovalError = err instanceof Error ? err.message : String(err);
  } finally {
    state.remoteApprovalLoading = false;
  }
}

export async function testRemoteApproval(
  state: RemoteApprovalState,
  provider: string,
): Promise<void> {
  if (!state.client || !state.connected) {
    return;
  }
  state.remoteApprovalTestLoading = true;
  state.remoteApprovalTestError = null;
  state.remoteApprovalTestResult = null;
  try {
    await state.client.request("security.remoteApproval.test", { provider });
    state.remoteApprovalTestResult = `Provider ${provider} test succeeded`;
  } catch (err) {
    state.remoteApprovalTestError = err instanceof Error ? err.message : String(err);
  } finally {
    state.remoteApprovalTestLoading = false;
  }
}

export async function saveRemoteApproval(state: RemoteApprovalState): Promise<void> {
  if (!state.client || !state.connected) {
    return;
  }
  state.remoteApprovalSaving = true;
  state.remoteApprovalError = null;
  state.remoteApprovalSaved = false;
  try {
    const wecomAgentId = Number.parseInt(state.remoteApprovalWecomAgentId.trim(), 10);

    await state.client.request("security.remoteApproval.config.set", {
      enabled: state.remoteApprovalEnabled,
      callbackUrl: state.remoteApprovalCallbackUrl.trim(),
      feishu: {
        enabled: state.remoteApprovalFeishuEnabled,
        appId: state.remoteApprovalFeishuAppId.trim(),
        appSecret: state.remoteApprovalFeishuAppSecret,
        chatId: state.remoteApprovalFeishuChatId.trim(),
      },
      dingtalk: {
        enabled: state.remoteApprovalDingtalkEnabled,
        webhookUrl: state.remoteApprovalDingtalkWebhookUrl.trim(),
        webhookSecret: state.remoteApprovalDingtalkWebhookSecret,
      },
      wecom: {
        enabled: state.remoteApprovalWecomEnabled,
        corpId: state.remoteApprovalWecomCorpId.trim(),
        agentId: Number.isFinite(wecomAgentId) ? wecomAgentId : 0,
        secret: state.remoteApprovalWecomSecret,
        toUser: state.remoteApprovalWecomToUser.trim(),
        toParty: state.remoteApprovalWecomToParty.trim(),
      },
    });

    state.remoteApprovalSaved = true;
    await loadRemoteApproval(state);
  } catch (err) {
    state.remoteApprovalError = err instanceof Error ? err.message : String(err);
  } finally {
    state.remoteApprovalSaving = false;
  }
}
