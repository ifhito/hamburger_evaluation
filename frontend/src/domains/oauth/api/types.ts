// 許可の範囲。名前と、利用者に見せる説明は backend が決める(frontend は写さず、そのまま表示する)。
export interface OAuthScope {
  name: string;
  description: string;
  // 書き込みを伴う範囲か(backend が domain の定義から返す。frontend は範囲の名前を比べない)。
  writes: boolean;
}

// 許可を尋ねる画面に出す内容。consentRequired が false なら、求められた範囲は許可済みの範囲に収まるので、
// 尋ねずにそのまま許可を送ってよい。
export interface AuthorizeRequestView {
  client: { id: string; name: string };
  scopes: OAuthScope[];
  consentRequired: boolean;
}

// 許可・拒否の結果。redirectTo は、利用者のブラウザで開く、アプリへの戻り先(認可コードまたはエラーが付く)。
export interface DecisionResponse {
  redirectTo: string;
}

// 利用者が許可したアプリ。
export interface ConnectedApp {
  id: string;
  clientId: string;
  clientName: string;
  scopes: OAuthScope[];
  createdAt: string;
  updatedAt: string;
}
