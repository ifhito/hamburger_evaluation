import { describe, it, expect } from "vitest";
import type { AuthorizeRequestView } from "./api/types";
import {
  NotNavigableError,
  phaseFor,
  redirectPhase,
  searchToDecide,
  startConsent,
  type ConsentApi,
  type ConsentHandlers,
  type ConsentState,
} from "./consentFlow";

const searchA = "?client_id=app-a&redirect_uri=https%3A%2F%2Fa.example.com%2Fcb&scope=hamburger%3Aread&state=aaaaaaaa";
const searchB = "?client_id=app-b&redirect_uri=https%3A%2F%2Fb.example.com%2Fcb&scope=hamburger%3Aread+hamburger%3Awrite&state=bbbbbbbb";

// search の client_id から、そのアプリの内容を作る(アプリごとに、名前と範囲が違う)。
function viewFor(search: string, consentRequired = true): AuthorizeRequestView {
  const id = new URLSearchParams(search).get("client_id") ?? "";
  return { client: { id, name: `Name of ${id}` }, scopes: [], consentRequired };
}

function asking(search: string): ConsentState {
  return { search, phase: { status: "asking", view: viewFor(search) } };
}

// 遅らせて返せる、テスト用の取得役。resolve するまで、describe は返らない。
function fakeApi() {
  const pending: { search: string; resolve: (v: AuthorizeRequestView) => void; reject: (e: unknown) => void; signal?: AbortSignal }[] = [];
  const decided: { search: string; approve: boolean }[] = [];
  const pendingDecides: (() => void)[] = [];
  // hold が true のとき、decide は、pendingDecides の関数を呼ぶまで返らない。
  const control = { hold: false };
  const api: ConsentApi = {
    describe: (search, signal) =>
      new Promise((resolve, reject) => {
        pending.push({ search, resolve, reject, signal });
      }),
    decide: async (search, approve) => {
      decided.push({ search, approve });
      if (control.hold) await new Promise<void>((resolve) => pendingDecides.push(resolve));
      return { redirectTo: `https://${new URLSearchParams(search).get("client_id")}.example.com/cb?code=abc` };
    },
  };
  return { api, pending, decided, pendingDecides, control };
}

function recorder() {
  const calls: string[] = [];
  const on: ConsentHandlers = {
    asking: (view) => calls.push(`asking:${view.client.id}`),
    redirect: (target) => calls.push(`redirect:${target}`),
    failed: (error) => calls.push(`failed:${String(error)}`),
  };
  return { calls, on };
}

// 少し待って、Promise の連鎖を進める。
const tick = () => new Promise((r) => setTimeout(r, 0));

describe("phaseFor: 画面の URL(search)が変わったとき、前の要求の内容を見せない", () => {
  it("いまの URL のために作られた状態は、そのまま見せる", () => {
    expect(phaseFor(asking(searchA), searchA)).toEqual(asking(searchA).phase);
  });

  it("URL がアプリ B の要求に変わったら、アプリ A の許可の画面(asking)は見せず、確認中(loading)になる", () => {
    expect(phaseFor(asking(searchA), searchB)).toEqual({ status: "loading" });
  });

  it("前の要求の失敗や、進行中(redirecting)の状態も、別の要求では見せない", () => {
    for (const phase of [{ status: "failed", error: new Error("x") }, { status: "redirecting" }] as const) {
      expect(phaseFor({ search: searchA, phase }, searchB)).toEqual({ status: "loading" });
    }
  });

  it("まだ状態がないとき(最初の取得中)は、確認中になる", () => {
    expect(phaseFor(null, searchA)).toEqual({ status: "loading" });
  });

  it("A に戻したとき(履歴を戻る)も、状態は要求ごとに結び付いているので、いまの URL の状態だけを見せる", () => {
    expect(phaseFor(asking(searchB), searchB)).toEqual(asking(searchB).phase);
    expect(phaseFor(asking(searchB), searchA)).toEqual({ status: "loading" });
  });
});

describe("searchToDecide: 許可・拒否として backend に送る内容は、画面に見えているアプリの要求と一致する", () => {
  it("いまの URL のために内容を見せているとき(asking)は、その要求を返す", () => {
    expect(searchToDecide(asking(searchB), searchB)).toBe(searchB);
  });

  it("URL がアプリ B に変わって、まだアプリ A の内容の状態しかないとき(取得中)は、何も送らない(A の画面のボタンで B を許可できない)", () => {
    expect(searchToDecide(asking(searchA), searchB)).toBeNull();
  });

  it("確認中・失敗・進行中のときは、何も送らない", () => {
    expect(searchToDecide(null, searchA)).toBeNull();
    for (const phase of [{ status: "loading" }, { status: "redirecting" }, { status: "failed", error: 1 }] as const) {
      expect(searchToDecide({ search: searchA, phase }, searchA)).toBeNull();
    }
  });

  it("送る内容は、いつも状態が作られた要求で、別の要求の値に置き換わらない", () => {
    // 状態の要求と URL が一致するときだけ返るので、返る値は、見えている内容(client.id)の要求そのもの
    const state = asking(searchB);
    const sent = searchToDecide(state, searchB);
    expect(sent).not.toBeNull();
    expect(new URLSearchParams(sent ?? "").get("client_id")).toBe(state.phase.status === "asking" ? state.phase.view.client.id : "");
  });
});

describe("startConsent: 古い要求の取得が、新しい画面に影響しない", () => {
  it("要求の内容が返り、許可を尋ねる必要があれば、asking を伝える", async () => {
    const { api, pending } = fakeApi();
    const { calls, on } = recorder();
    startConsent(searchA, api, on);
    pending[0].resolve(viewFor(searchA));
    await tick();
    expect(calls).toEqual(["asking:app-a"]);
  });

  it("取り消したあとに、遅れて返ってきた古い要求(A)の応答は、何も伝えず、新しい要求(B)の結果だけが伝わる", async () => {
    const { api, pending } = fakeApi();
    const { calls, on } = recorder();
    const cancelA = startConsent(searchA, api, on);
    cancelA(); // 画面が B に切り替わった
    startConsent(searchB, api, on);
    pending[1].resolve(viewFor(searchB)); // B が先に返る
    await tick();
    pending[0].resolve(viewFor(searchA)); // A が、遅れて返る
    await tick();
    expect(calls).toEqual(["asking:app-b"]);
  });

  it("取り消したあとに、古い要求(A)の取得が失敗しても、失敗は伝わらない", async () => {
    const { api, pending } = fakeApi();
    const { calls, on } = recorder();
    const cancelA = startConsent(searchA, api, on);
    cancelA();
    pending[0].reject(new Error("aborted"));
    await tick();
    expect(calls).toEqual([]);
  });

  it("取り消すと、進行中の取得を中止する(AbortSignal が中止される)", () => {
    const { api, pending } = fakeApi();
    const { on } = recorder();
    const cancel = startConsent(searchA, api, on);
    expect(pending[0].signal?.aborted).toBe(false);
    cancel();
    expect(pending[0].signal?.aborted).toBe(true);
  });

  it("取得が失敗したときは、その失敗を伝える", async () => {
    const { api, pending } = fakeApi();
    const { calls, on } = recorder();
    startConsent(searchA, api, on);
    pending[0].reject(new Error("boom"));
    await tick();
    expect(calls).toEqual(["failed:Error: boom"]);
  });

  it("すでに許可済みの範囲に収まる要求は、尋ねずに、取得したのと同じ要求で許可を送り、その戻り先を伝える", async () => {
    const { api, pending, decided } = fakeApi();
    const { calls, on } = recorder();
    startConsent(searchA, api, on);
    pending[0].resolve(viewFor(searchA, false));
    await tick();
    expect(decided).toEqual([{ search: searchA, approve: true }]);
    expect(calls).toEqual(["redirect:https://app-a.example.com/cb?code=abc"]);
  });

  it("許可を送っている途中で取り消されたら、あとから返った戻り先は伝わらない(古い要求の結果で、画面を移動しない)", async () => {
    const { api, pending, decided, pendingDecides, control } = fakeApi();
    control.hold = true;
    const { calls, on } = recorder();
    const cancel = startConsent(searchA, api, on);
    pending[0].resolve(viewFor(searchA, false));
    await tick(); // describe の応答を処理し、許可を送った(まだ返っていない)
    expect(decided).toEqual([{ search: searchA, approve: true }]);
    cancel(); // 画面が別の要求に切り替わった
    pendingDecides[0](); // 許可の応答が、あとから返る
    await tick();
    expect(calls).toEqual([]);
  });
});

describe("シナリオ: アプリ A の画面が出ているときに、URL がアプリ B に変わる", () => {
  it("B の内容が返るまで、A の内容は見えず、ボタンは何も送らない。B の内容が返ると、B の内容と B の要求が対応する", async () => {
    const { api, pending } = fakeApi();
    let state: ConsentState | null = null;
    const attach = (search: string) => {
      const on: ConsentHandlers = {
        asking: (view) => (state = { search, phase: { status: "asking", view } }),
        redirect: () => undefined,
        failed: () => undefined,
      };
      return startConsent(search, api, on);
    };

    let cancel = attach(searchA);
    pending[0].resolve(viewFor(searchA));
    await tick();
    expect(phaseFor(state, searchA).status).toBe("asking");
    expect(searchToDecide(state, searchA)).toBe(searchA);

    // URL が B に変わる(画面の effect は、前の取得を取り消して、新しい取得を始める)
    cancel();
    cancel = attach(searchB);
    expect(phaseFor(state, searchB)).toEqual({ status: "loading" }); // A の内容は、もう見えない
    expect(searchToDecide(state, searchB)).toBeNull(); // A の画面のボタンで、B を許可できない

    pending[1].resolve(viewFor(searchB));
    await tick();
    const shown = phaseFor(state, searchB);
    expect(shown.status).toBe("asking");
    const sent = searchToDecide(state, searchB);
    expect(sent).toBe(searchB);
    // 見えている内容(アプリ B)と、送る要求(アプリ B の client_id)が一致する
    expect(shown.status === "asking" && shown.view.client.id).toBe(new URLSearchParams(sent ?? "").get("client_id"));
    cancel();
  });
});

describe("redirectPhase", () => {
  it("http(s) の戻り先は、進行中(redirecting)になる", () => {
    expect(redirectPhase("https://app.example.com/cb?code=abc")).toEqual({ status: "redirecting" });
  });

  it("javascript: などの戻り先は、開かず、失敗になる", () => {
    const phase = redirectPhase("javascript:alert(1)");
    expect(phase.status).toBe("failed");
    expect(phase.status === "failed" && phase.error).toBeInstanceOf(NotNavigableError);
  });
});
