import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../../auth/storage";
import { toPage, type Page } from "../../../api/page";
import type { AuthorizeRequestView, ConnectedApp, DecisionResponse } from "./types";

export const oauthApiClient = buildApiClient(getToken);

// 認可の URL の値(URL の ? のあと)。先頭の ? はあってもなくてもよい。
function withoutQuestionMark(search: string): string {
  return search.startsWith("?") ? search.slice(1) : search;
}

// 許可したアプリの一覧の URL。1 ページの件数は送らない(backend が決める)。
export function grantsUrl(page: number): string {
  return `/oauth/grants?page=${page}`;
}

export const oauthApi = {
  // 許可を尋ねる画面に出す内容を取得する。search は、この画面の URL の ? 以降を、そのまま渡す
  // (要求の検証は backend が行う。frontend は値を解釈しない)。
  // signal は、待たなくなった取得(画面が別の要求に切り替わった)を取り消すために使う。
  async describe(search: string, signal?: AbortSignal): Promise<AuthorizeRequestView> {
    const query = withoutQuestionMark(search);
    const res = await oauthApiClient.get<AuthorizeRequestView>(`/oauth/authorize/request${query ? `?${query}` : ""}`, { signal });
    return res.data;
  },
  // 利用者の許可・拒否を送り、アプリへの戻り先を受け取る。
  async decide(search: string, approve: boolean, signal?: AbortSignal): Promise<DecisionResponse> {
    const res = await oauthApiClient.post<DecisionResponse>(
      "/oauth/authorize/decision",
      { query: withoutQuestionMark(search), approve },
      { signal },
    );
    return res.data;
  },
  // 許可したアプリの一覧の 1 ページぶん。url は grantsUrl で作る。続きがあるかは backend の X-Has-More で決まる。
  async listApps(url: string): Promise<Page<ConnectedApp>> {
    return toPage(await oauthApiClient.get<ConnectedApp[]>(url));
  },
  async revokeApp(id: string): Promise<void> {
    await oauthApiClient.delete(`/oauth/grants/${id}`);
  },
};
