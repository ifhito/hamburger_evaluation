// jsdom 環境のテスト用(ファイルの先頭に `// @vitest-environment jsdom` を書く)。コンポーネントを実際にマウントして、
// 押す・入力する・表示を待つ、ための最小の道具。ページの移動は jsdom では起きない(起きるはずの移動は、テスト側で表す)。
import { act, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// 外部へ移動するリンクを押しても、jsdom の「未実装」のエラーを出さない(react-router のリンクは、自分で処理したあとなので影響しない)。
document.addEventListener("click", (e) => {
  if (e.target instanceof Element && e.target.closest("a[href]")) e.preventDefault();
});

const mounted: { root: Root; container: HTMLElement }[] = [];

export async function mount(ui: ReactElement): Promise<HTMLElement> {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  mounted.push({ root, container });
  await act(async () => {
    root.render(ui);
  });
  return container;
}

export async function unmount(container: HTMLElement): Promise<void> {
  const i = mounted.findIndex((m) => m.container === container);
  if (i < 0) return; // すでに外した(または、mount していない)ものは、何もしない(別の root を外さない)
  const [entry] = mounted.splice(i, 1);
  await act(async () => entry.root.unmount());
  entry.container.remove();
}

export async function cleanup(): Promise<void> {
  while (mounted.length > 0) await unmount(mounted[0].container);
}

export async function click(el: Element): Promise<void> {
  await act(async () => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
  });
}

// React が入力の変化として受け取れるように、値を、ブラウザ本来の setter で入れてから、input イベントを流す。
export async function type(input: HTMLInputElement, value: string): Promise<void> {
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  if (!setValue) throw new Error("HTMLInputElement value setter not found");
  await act(async () => {
    setValue.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

// 待つ時間(ミリ秒)。テストの道具の値で、backend の規則の数字とは関係がない(noDuplicatedLimits の検査と重ならない値にしている)。
const WAIT_TIMEOUT_MS = 3_000;
const WAIT_INTERVAL_MS = 10;

// 条件が満たされるまで待つ。React 19 は、act の中で予約した更新を、act の終わりまで流さないので、待つ全体を 1 つの act に
// 包むと、待っている間にタイマーなどで起きる描画を観測できない。間隔ごとに act を閉じて、更新を流しながら、確かめ直す。
// 時間内に満たされなかったときは、最後の失敗を投げる。
export async function eventually(assertion: () => void): Promise<void> {
  const deadline = Date.now() + WAIT_TIMEOUT_MS;
  for (;;) {
    try {
      assertion();
      return;
    } catch (e) {
      if (Date.now() >= deadline) throw e;
    }
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, WAIT_INTERVAL_MS));
    });
  }
}

export function byText<T extends Element>(container: ParentNode, selector: string, text: string): T | undefined {
  return [...container.querySelectorAll<T>(selector)].find((el) => el.textContent?.trim() === text);
}

export function need<T>(value: T | undefined | null, what: string): T {
  if (value === undefined || value === null) throw new Error(`not found: ${what}`);
  return value;
}
