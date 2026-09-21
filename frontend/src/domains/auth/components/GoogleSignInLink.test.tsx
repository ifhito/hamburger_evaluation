// @vitest-environment jsdom
import { vi } from "vitest";
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "/api"));
import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";
import "../../../lib/i18n";
import { byText, cleanup, mount, need } from "../../../test/dom";
import { GoogleSignInLink } from "./GoogleSignIn";

afterEach(cleanup);

const HREF = "/api/auth/google/start";

// 押したときに、React のハンドラが移動を止めたか(defaultPrevented)を見る。この確認用の listener は、React の listener(container に付く)のあとで動く。
async function press(page: HTMLElement, link: Element, init: MouseEventInit = {}) {
  let prevented = false;
  const observe = (e: Event) => {
    prevented = e.defaultPrevented;
  };
  page.addEventListener("click", observe);
  await act(async () => {
    link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0, ...init }));
  });
  page.removeEventListener("click", observe);
  return prevented;
}

describe("GoogleSignInLink(押したあと、Google の画面へ移るまでの間)", () => {
  it("押すと、移動中の文言に変わり、押せない状態(aria-busy・aria-disabled・フォーカス外し)になる。最初の押下は、移動を止めない", async () => {
    const page = await mount(<GoogleSignInLink href={HREF} label="Continue with Google" />);
    const link = need(byText(page, "a", "Continue with Google"), "link");

    const prevented = await press(page, link);

    expect(prevented).toBe(false);
    expect(link.textContent).toBe("Going to Google…");
    expect(link.getAttribute("aria-busy")).toBe("true");
    expect(link.getAttribute("aria-disabled")).toBe("true");
    expect(link.getAttribute("tabindex")).toBe("-1");
    expect(link.getAttribute("href")).toBe(HREF);
  });

  it("移動中にもう一度押しても、移動を始めない(二重に開始しない)", async () => {
    const page = await mount(<GoogleSignInLink href={HREF} label="Continue with Google" />);
    const link = need(byText(page, "a", "Continue with Google"), "link");
    await press(page, link);

    expect(await press(page, link)).toBe(true);
  });

  it("新しいタブで開く押し方(修飾キー・中ボタン)のときは、この画面は移動しないので、移動中にしない", async () => {
    const page = await mount(<GoogleSignInLink href={HREF} label="Continue with Google" />);
    const link = need(byText(page, "a", "Continue with Google"), "link");

    await press(page, link, { ctrlKey: true });
    await press(page, link, { metaKey: true });
    await press(page, link, { button: 1 });

    expect(link.getAttribute("aria-busy")).toBeNull();
    expect(link.textContent).toBe("Continue with Google");
  });

  it("戻る操作で、移動中のまま復元されたとき(pageshow・persisted)は、押せる状態に戻す", async () => {
    const page = await mount(<GoogleSignInLink href={HREF} label="Continue with Google" />);
    const link = need(byText(page, "a", "Continue with Google"), "link");
    await press(page, link);

    await act(async () => {
      window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: true }));
    });

    expect(link.getAttribute("aria-busy")).toBeNull();
    expect(link.textContent).toBe("Continue with Google");
  });
});
