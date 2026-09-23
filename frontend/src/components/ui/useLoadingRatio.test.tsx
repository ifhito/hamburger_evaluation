// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { cleanup, eventually, mount } from "../../test/dom";
import { useLoadingRatio } from "./useLoadingRatio";

function Probe() {
  const ratio = useLoadingRatio();
  return <span data-testid="ratio">{ratio}</span>;
}

afterEach(cleanup);

describe("useLoadingRatio(読み込み中のバーガーの水位)", () => {
  it("時間が経つと、0 から始まって値が変わる(下から満ちていくアニメーション)", async () => {
    const page = await mount(<Probe />);
    const el = page.querySelector('[data-testid="ratio"]')!;
    expect(el.textContent).toBe("0");
    await eventually(() => expect(el.textContent).not.toBe("0"));
  });
});
