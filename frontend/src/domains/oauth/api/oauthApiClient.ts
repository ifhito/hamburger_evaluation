import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../../auth/storage";
import type { AuthorizeRequestView, ConnectedApp, DecisionResponse } from "./types";

export const oauthApiClient = buildApiClient(getToken);

// 認可の URL の値(URL の ? のあと)。先頭の ? はあってもなくてもよい。
function withoutQuestionMark(search: string): string {
  return search.startsWith("?") ? search.slice(1) : search;
}

export const oauthApi = {
  // 許可を尋ねる画面に出す内容を取得する。search は、この画面の URL の ? 以降を、そのまま渡す
  // (要求の検証は backend が行う。frontend は値を解釈しない)。
  async describe(search: string): Promise<AuthorizeRequestView> {
    const query = withoutQuestionMark(search);
    const res = await oauthApiClient.get<AuthorizeRequestView>(`/oauth/authorize/request${query ? `?${query}` : ""}`);
    return res.data;
  },
  // 利用者の許可・拒否を送り、アプリへの戻り先を受け取る。
  async decide(search: string, approve: boolean): Promise<DecisionResponse> {
    const res = await oauthApiClient.post<DecisionResponse>("/oauth/authorize/decision", {
      query: withoutQuestionMark(search),
      approve,
    });
    return res.data;
  },
  async listApps(): Promise<ConnectedApp[]> {
    const res = await oauthApiClient.get<ConnectedApp[]>("/oauth/grants");
    return res.data;
  },
  async revokeApp(id: string): Promise<void> {
    await oauthApiClient.delete(`/oauth/grants/${id}`);
  },
};
