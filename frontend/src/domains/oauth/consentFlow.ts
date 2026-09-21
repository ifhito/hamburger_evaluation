import type { AuthorizeRequestView, DecisionResponse } from "./api/types";
import { isNavigable } from "./navigation";

// 許可を尋ねる画面の状態。状態は、それを作った認可の要求(画面の URL の ? 以降。search)に結び付ける。
// 画面の URL が別の要求に変わったとき(同じ画面のまま query だけが変わる、履歴を戻る・進む)に、前の要求の
// 内容(アプリ A の名前と範囲)が残ったまま、新しい要求(アプリ B)への許可を送ってしまうのを防ぐため。

export type ConsentPhase =
  | { status: "loading" }
  | { status: "asking"; view: AuthorizeRequestView }
  | { status: "redirecting" }
  | { status: "failed"; error: unknown };

export interface ConsentState {
  search: string;
  phase: ConsentPhase;
}

const LOADING: ConsentPhase = { status: "loading" };

// 画面に見せる状態。いまの URL(search)のために作られた状態だけを見せ、それ以外(まだ何もない・前の要求の
// もの)は、確認中として扱う。前の要求の内容や操作は、画面に出ない。
export function phaseFor(state: ConsentState | null, search: string): ConsentPhase {
  return state !== null && state.search === search ? state.phase : LOADING;
}

// 利用者が許可・拒否を選んだとき、backend に送る認可の要求(search)。いまの URL のために、内容を見せている
// (asking の)状態があるときだけ、その要求を返す。送る内容は、画面に見えているアプリと、いつも一致する。
// それ以外(確認中・前の要求の画面・すでに進行中)は null で、何も送らない。
export function searchToDecide(state: ConsentState | null, search: string): string | null {
  return state !== null && state.search === search && state.phase.status === "asking" ? state.search : null;
}

// アプリへの戻り先を、ブラウザで開けない(http(s) ではない)ことを表す。
export class NotNavigableError extends Error {
  constructor() {
    super("The redirect target can't be opened");
    this.name = "NotNavigableError";
  }
}

// アプリへの戻り先を、開いてよいかで、次の状態を決める。
export function redirectPhase(target: string): ConsentPhase {
  return isNavigable(target) ? { status: "redirecting" } : { status: "failed", error: new NotNavigableError() };
}

export interface ConsentApi {
  describe(search: string, signal?: AbortSignal): Promise<AuthorizeRequestView>;
  decide(search: string, approve: boolean, signal?: AbortSignal): Promise<DecisionResponse>;
}

export interface ConsentHandlers {
  asking(view: AuthorizeRequestView): void;
  redirect(target: string): void;
  failed(error: unknown): void;
}

// 認可の要求(search)の内容を取得して、許可を尋ねる(すでに許可済みの範囲に収まるなら、尋ねずに許可を送る)。
// 戻り値は取り消しの関数で、画面が別の要求に切り替わったときに呼ぶ。呼ぶと、進行中の取得を中止し、
// あとから返ってきた応答は、handlers に一切届かない(古い要求の結果が、新しい画面を上書きしない)。
export function startConsent(search: string, api: ConsentApi, on: ConsentHandlers): () => void {
  const controller = new AbortController();
  const live = () => !controller.signal.aborted;
  api
    .describe(search, controller.signal)
    .then(async (view) => {
      if (!live()) return;
      if (view.consentRequired) {
        on.asking(view);
        return;
      }
      const { redirectTo } = await api.decide(search, true, controller.signal);
      if (live()) on.redirect(redirectTo);
    })
    .catch((error: unknown) => {
      if (live()) on.failed(error);
    });
  return () => controller.abort();
}
