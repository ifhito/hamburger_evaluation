import { describe, it, expect } from "vitest";
import { isValidElement, type ReactElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import "../../lib/i18n";
import { Alert } from "./Alert";
import { Badge } from "./Badge";
import { Button } from "./Button";
import { Card } from "./Card";
import { RatingBurger, RatingBurgerIcon } from "./RatingBurger";
import { RatingInput } from "./RatingInput";
import { EmptyState, Loading, NotFound } from "./states";
import { TextArea, TextField } from "./TextField";

const html = (node: React.ReactElement) => renderToStaticMarkup(node);

// CSS モジュールの class 名は「_名前_ハッシュ」。名前だけを取り出して、完全一致で比べる(danger が dangerSolid・offDanger に一致しないように)。
const classNames = (markup: string) =>
  (markup.match(/class="([^"]*)"/)?.[1] ?? "").split(" ").map((c) => c.replace(/^_(.+)_[0-9a-f]+$/, "$1"));

describe("Button", () => {
  it("種類ごとに、別々の class を 1 つだけ持つ", () => {
    expect(classNames(html(<Button>Save</Button>))).toEqual(["btn", "primary"]);
    expect(classNames(html(<Button variant="secondary">Cancel</Button>))).toEqual(["btn", "secondary"]);
    expect(classNames(html(<Button variant="danger">Delete</Button>))).toEqual(["btn", "danger"]);
    expect(classNames(html(<Button variant="dangerSolid">Delete</Button>))).toEqual(["btn", "dangerSolid"]);
    expect(classNames(html(<Button variant="dark">Confirm reject</Button>))).toEqual(["btn", "dark"]);
    expect(html(<Button>Save</Button>)).not.toContain("disabled");
  });

  it("送信中は、押せず、aria-busy になり、指定した文言(なければ Loading…)を出す", () => {
    const withLabel = html(<Button isLoading loadingLabel="Posting…">Post</Button>);
    expect(withLabel).toContain("disabled");
    expect(withLabel).toContain('aria-busy="true"');
    expect(withLabel).toContain("Posting…");
    expect(withLabel).not.toContain(">Post<");
    expect(html(<Button isLoading>Post</Button>)).toContain("Loading\u2026");
  });

  it("送信中は、フォームが有効(disabled={false})でも、押せない(二重送信を防ぐ)", () => {
    const out = html(<Button isLoading disabled={false}>Post</Button>);
    expect(out).toContain("disabled");
    expect(classNames(out)).toEqual(["btn", "off"]);
  });

  it("無効のときは灰色の面(off)。削除の枠だけは、文字を赤のままにする(offDanger)。削除の確認は、灰色の面", () => {
    const off = html(<Button disabled>Post</Button>);
    expect(off).toContain("disabled");
    expect(classNames(off)).toEqual(["btn", "off"]);
    expect(classNames(html(<Button variant="danger" disabled>Delete</Button>))).toEqual(["btn", "offDanger"]);
    expect(classNames(html(<Button variant="dangerSolid" disabled>Delete</Button>))).toEqual(["btn", "off"]);
    expect(classNames(html(<Button variant="dark" disabled>Confirm reject</Button>))).toEqual(["btn", "off"]);
  });

  it("幅いっぱい(block)と、左右の余白を広げる(wide)は、種類に足される", () => {
    expect(classNames(html(<Button block wide>Save</Button>))).toEqual(["btn", "primary", "block", "wide"]);
  });

  it("form の中でも、指定しない限り送信ボタンにならない(type=button)", () => {
    expect(html(<Button>Save</Button>)).toContain('type="button"');
    expect(html(<Button type="submit">Save</Button>)).toContain('type="submit"');
  });
});

describe("Badge と Card", () => {
  it("札は、状態の文字を出し、見た目は outline と accent の 2 種類", () => {
    expect(classNames(html(<Badge>Pending</Badge>))).toEqual(["badge"]);
    expect(classNames(html(<Badge tone="accent">Public</Badge>))).toEqual(["badge", "accent"]);
    expect(html(<Badge>Pending</Badge>)).toContain("Pending");
  });

  it("カードは、強調と余白なしを選べる", () => {
    expect(classNames(html(<Card>x</Card>))).toEqual(["card", "padded"]);
    expect(classNames(html(<Card padded={false}>x</Card>))).toEqual(["card"]);
    expect(classNames(html(<Card current>x</Card>))).toEqual(["card", "current", "padded"]);
  });

  it("強調のカードは、色や枠だけでなく、aria-current でも「いまのもの」と伝える", () => {
    expect(html(<Card current>x</Card>)).toContain('aria-current="true"');
    expect(html(<Card>x</Card>)).not.toContain("aria-current");
  });
});

describe("Alert", () => {
  it("API の文言をそのまま出し、role=alert で読み上げ、アイコンと帯は装飾として読み上げない", () => {
    const out = html(<Alert title="Could not save" message="Comment is too long (maximum is 2000 characters)" />);
    expect(out).toContain('role="alert"');
    expect(out).toContain("Comment is too long (maximum is 2000 characters)");
    expect(out).toContain("Could not save");
    expect(out.match(/aria-hidden="true"/g)?.length).toBe(2);
  });

  it("文言が複数あるときは、書き換えず、箇条書きで並べる", () => {
    const out = html(<Alert message={["A is required", "B is invalid"]} />);
    expect(out).toContain("<li>A is required</li>");
    expect(out).toContain("<li>B is invalid</li>");
  });
});

describe("TextField と TextArea", () => {
  it("ラベルを入力欄に結び付け、説明と文字数のカウンターを aria-describedby で結び付け、指す id は実際にある", () => {
    const out = html(<TextField id="name" label="Name" hint="Shown to others" counter={{ value: "ab", max: 5 }} />);
    expect(out).toContain('for="name"');
    expect(out).toContain('aria-describedby="name-hint name-counter"');
    expect(out).toContain('id="name-hint"');
    expect(out).toContain('id="name-counter"');
    expect(out).toContain("2 / 5");
  });

  it("TextField は input、TextArea は textarea を出す", () => {
    expect(html(<TextField id="a" label="A" />)).toContain("<input");
    expect(html(<TextArea id="a" label="A" />)).toContain("<textarea");
  });

  it("上限が未取得(undefined)のときは、カウンターを出さず、結び付けもしない(値を推測しない)", () => {
    const out = html(<TextArea id="c" label="Comment" counter={{ value: "abc", max: undefined }} />);
    expect(out).not.toContain("/ ");
    expect(out).not.toContain("aria-describedby");
  });

  it("上限を超えたら、色だけでなく「Too long」の文字と、読み上げの文言でも知らせ、戻れば「収まっている」に戻る", () => {
    const over = html(<TextField id="n" label="Name" counter={{ value: "abcdef", max: 5 }} />);
    expect(over).toContain("6 / 5 Too long");
    expect(over).toContain("Over the limit (maximum 5 characters)");
    expect(classNames(over.match(/<span id="n-counter" class="[^"]*"/)![0].replace("<span id=\"n-counter\" ", "<x "))).toContain("over");
    const within = html(<TextField id="n" label="Name" counter={{ value: "abc", max: 5 }} />);
    expect(within).toContain("Within the limit");
    expect(within).not.toContain("Too long");
  });

  it("文字数は、絵文字も日本語も 1 文字で数える(backend と同じ数え方)", () => {
    expect(html(<TextField id="n" label="Name" counter={{ value: "\u{1F354}\u{1F354}\u3042\u3044", max: 5 }} />)).toContain("4 / 5");
  });

  it("任意の項目には、ラベルの隣に「任意」の説明を薄く出す", () => {
    expect(html(<TextField id="p" label="Photo" optional="(optional)" />)).toContain("(optional)");
  });
});

describe("空・読み込み中・見つからないときの画面", () => {
  it("空は、空のバーガーと、locale の文言と、操作(あれば)を出す", () => {
    const out = html(<EmptyState action={<Button>Write a review</Button>} />);
    expect(out).toContain("Nobody&#x27;s eaten here yet");
    expect(out).toContain("Why not write the first review?");
    expect(out).toContain("Write a review");
    expect(out).toContain("<svg");
  });

  it("読み込み中は、role=status で文言を伝え、灰色の帯は装飾として読み上げない。aria-busy は付けない(状態の読み上げが止まる)", () => {
    const out = html(<Loading />);
    expect(out).toContain('role="status"');
    expect(out).toContain("Grilling\u2026");
    expect(out).toContain("Just a moment");
    expect(out).not.toContain("aria-busy");
    expect(out).toMatch(/<div class="[^"]*skels[^"]*" aria-hidden="true">/);
  });

  it("見つからないときは、404 と文言と、操作(あれば)を出す", () => {
    const out = html(<NotFound action={<Button variant="secondary">Back to shops</Button>} />);
    expect(out).toContain("404");
    expect(out).toContain("Sold out");
    expect(out).toContain("Page not found");
    expect(out).toContain("Back to shops");
  });
});

const svgOf = (markup: string) => markup.match(/<svg[\s\S]*<\/svg>/)![0];

describe("RatingBurger(表示)", () => {
  it("数字を必ず併記し、読み上げは「Rating 4.5 out of 5」の 1 つの画像として伝え、絵と数字を二重に読み上げない", () => {
    const out = html(<RatingBurger value={4.5} max={5} />);
    expect(out).toContain('role="img"');
    expect(out).toContain('aria-label="Rating 4.5 out of 5"');
    expect(out).toContain('<b aria-hidden="true">4.5</b>');
    expect(out).toMatch(/<svg[^>]*aria-hidden="true"/);
  });

  it("数字は小数 1 桁に丸めて出す。整数は「4」、桁数を指定すると「4.0」", () => {
    expect(html(<RatingBurger value={4.46} max={5} />)).toContain(">4.5</b>");
    expect(html(<RatingBurger value={4} max={5} />)).toContain(">4</b>");
    expect(html(<RatingBurger value={4} max={5} fractionDigits={1} />)).toContain(">4.0</b>");
  });

  it("水位は、数字と同じ丸めた値 ÷ 最大値で決まる(数字と水位がずれない)。最大値は呼び出し側が渡す(GET /meta)", () => {
    expect(svgOf(html(<RatingBurger value={4.46} max={5} size="lg" />))).toBe(svgOf(html(<RatingBurgerIcon ratio={0.9} size="lg" />)));
    expect(svgOf(html(<RatingBurger value={2} max={4} size="lg" />))).toBe(svgOf(html(<RatingBurgerIcon ratio={0.5} size="lg" />)));
    expect(html(<RatingBurger value={2} max={4} />)).toContain("out of 4");
  });

  it("最大値が 0 以下でも、例外にならず、空のバーガーと同じ絵になる", () => {
    expect(svgOf(html(<RatingBurger value={3} max={0} size="lg" />))).toBe(svgOf(html(<RatingBurgerIcon ratio={0} size="lg" />)));
  });

  it("評価が 0 のときは、すべての食材が消灯の灰色の線で、塗り(fill)は使わない", () => {
    const out = html(<RatingBurger value={0} max={5} size="lg" />);
    expect(out).not.toContain('fill="#');
    expect(out.match(/stroke="#8e8e89"/g)?.length).toBe(6);
  });

  it("評価が最大のときは、すべての食材が自分の色の線で点灯し、消灯の灰色がない", () => {
    const out = html(<RatingBurger value={5} max={5} size="lg" />);
    expect(out).not.toContain('stroke="#8e8e89"');
  });

  it("水位の途中では、水位より下の食材が自分の色、上が消灯の灰色の線になる(塗りは使わない)", () => {
    const out = html(<RatingBurgerIcon ratio={0.55} size="lg" />);
    expect(out).not.toContain('fill="#');
    expect((out.match(/stroke="#8e8e89"/g)?.length ?? 0)).toBeGreaterThan(0);
    expect(out).toMatch(/stroke="#(?!8e8e89)[0-9a-f]{6}"/);
  });

  it("絵だけの部品は、装飾として読み上げず、数字を持たない", () => {
    const out = html(<RatingBurgerIcon ratio={0.5} size="sm" />);
    expect(out).toContain('aria-hidden="true"');
    expect(out).not.toContain("<b");
  });
});

type ElProps = {
  children?: ReactNode;
  onClick?: () => void;
  onKeyDown?: (e: { key: string; preventDefault: () => void }) => void;
  role?: string;
  "aria-checked"?: boolean;
};
const walk = (node: ReactNode, out: ReactElement<ElProps>[] = []) => {
  if (Array.isArray(node)) node.forEach((n) => walk(n, out));
  else if (isValidElement<ElProps>(node)) {
    out.push(node);
    walk(node.props.children, out);
  }
  return out;
};

describe("RatingInput(入力)", () => {
  const input = (props: Partial<React.ComponentProps<typeof RatingInput>> = {}) =>
    html(<RatingInput label="Rating" value={3} onChange={() => {}} min={1} max={5} {...props} />);
  const values = (markup: string) => [...markup.matchAll(/<button[^>]*>(\d+)<\/button>/g)].map((m) => m[1]);
  const buttonFor = (markup: string, n: number) => new RegExp(`<button[^>]*>${n}</button>`).exec(markup)?.[0] ?? "";

  it("最小〜最大の数字のボタン(role=radio)を並べ、選んでいる値だけが aria-checked になり、その札だけが選択中の見た目(on)になる", () => {
    const out = input();
    expect(out.match(/role="radio"/g)?.length).toBe(5);
    expect(out.match(/aria-checked="true"/g)?.length).toBe(1);
    for (let n = 1; n <= 5; n++) {
      const btn = buttonFor(out, n);
      expect(btn, btn).toContain(`aria-checked="${n === 3}"`);
      expect(classNames(btn).includes("on"), btn).toBe(n === 3);
    }
  });

  it("ボタンの数と数字は、渡された最小・最大(GET /meta)で決まる(frontend に評価の範囲を持たない)", () => {
    expect(values(input({ min: 1, max: 7 }))).toEqual(["1", "2", "3", "4", "5", "6", "7"]);
    expect(values(input({ min: 2, max: 5 }))).toEqual(["2", "3", "4", "5"]);
    expect(values(input({ min: 1, max: 3 }))).toEqual(["1", "2", "3"]);
  });

  it("ARIA のカスタム radiogroup として、ラベルを aria-labelledby で結び付けて読み上げる(fieldset/legend もネイティブ radio も使わない)", () => {
    const out = input();
    expect(out).toContain('role="radiogroup"');
    expect(out).toContain('aria-orientation="horizontal"');
    const labelId = /aria-labelledby="([^"]+)"/.exec(out)?.[1];
    expect(labelId).toBeTruthy();
    expect(out).toMatch(new RegExp(`<span id="${labelId}"[^>]*>Rating</span>`));
    expect(out).not.toContain("<fieldset");
    expect(out).not.toContain("<legend");
    expect(out).not.toContain('type="radio"');
  });

  it("同じ画面に複数置いても、ラベルの id が重ならない(name ではなく useId で作る)", () => {
    const out = html(
      <>
        <RatingInput label="Rating" value={3} onChange={() => {}} min={1} max={5} />
        <RatingInput label="Rating" value={3} onChange={() => {}} min={1} max={5} />
      </>,
    );
    const ids = [...out.matchAll(/aria-labelledby="([^"]+)"/g)].map((m) => m[1]);
    expect(ids.length).toBe(2);
    expect(new Set(ids).size).toBe(2);
  });

  it("roving tabindex: 選んでいる値の札だけ tabIndex=0、残りは -1", () => {
    const out = input({ value: 4 });
    expect(buttonFor(out, 4)).toContain('tabindex="0"');
    for (const n of [1, 2, 3, 5]) expect(buttonFor(out, n)).toContain('tabindex="-1"');
  });

  it("渡された値が、この入力の最小・最大の範囲の外にあるときも、先頭(min)の札が roving tabindex の対象になる(範囲外のままだと、どの札も tabIndex=0 を持てず、キーボードで群に入れなくなる)", () => {
    const out = input({ value: 7, min: 1, max: 5 });
    expect(buttonFor(out, 1)).toContain('tabindex="0"');
    for (const n of [2, 3, 4, 5]) expect(buttonFor(out, n)).toContain('tabindex="-1"');
  });

  it("選んだ値を大きな数字と「out of 5」でも見せる", () => {
    const out = input({ value: 4 });
    expect(out).toContain("<b>4</b>");
    expect(out).toContain("out of 5");
  });

  it("バーガーの水位は、選んだ値 ÷ 最大値。まだ選んでいない(null)ときは空", () => {
    const iconOf = (ratio: number) => svgOf(html(<RatingBurgerIcon ratio={ratio} size="lg" />));
    expect(svgOf(input({ value: 3 }))).toBe(iconOf(0.6));
    expect(svgOf(input({ value: 3, min: 1, max: 7 }))).toBe(iconOf(3 / 7));
    expect(svgOf(input({ value: null }))).toBe(iconOf(0));
  });

  it("まだ選んでいない(null)ときは、どのボタンも aria-checked=true でなく、数字の場所は「–」。roving tabindex は先頭(min)に置かれる", () => {
    const out = input({ value: null });
    expect(out).not.toContain('aria-checked="true"');
    expect(out).toContain("<b>\u2013</b>");
    expect(buttonFor(out, 1)).toContain('tabindex="0"');
    for (let n = 2; n <= 5; n++) expect(buttonFor(out, n)).toContain('tabindex="-1"');
  });

  const rendered = (value: number | null, onChange: (v: number) => void) => {
    let tree: ReactNode = null;
    const Probe = () => {
      tree = RatingInput({ label: "Rating", value, onChange, min: 1, max: 5 });
      return null;
    };
    html(<Probe />);
    return walk(tree).filter((e) => e.props.role === "radio");
  };

  it("ボタンを押す(クリック)と、その数字で onChange が呼ばれる", () => {
    const calls: number[] = [];
    const radios = rendered(3, (v) => calls.push(v));
    expect(radios.map((r) => r.props.children)).toEqual([1, 2, 3, 4, 5]);
    radios[3].props.onClick?.();
    radios[0].props.onClick?.();
    expect(calls).toEqual([4, 1]);
  });

  it("すでに選んでいる値のボタンを押しても、onChange は呼ばれない(ネイティブの radio が、選択済みのものを押しても change を発火しないのと同じ)", () => {
    const calls: number[] = [];
    const radios = rendered(3, (v) => calls.push(v));
    radios[2].props.onClick?.(); // 3(選んでいる値)を押す
    expect(calls).toEqual([]);
  });

  it("矢印キー(→/↓ で次、←/↑ で前)で選ぶ値が変わり、端では反対側へ回る。Home/End で先頭・末尾へ", () => {
    const press = (key: string, current: number) => {
      const calls: number[] = [];
      const radios = rendered(current, (v) => calls.push(v));
      const active = radios.find((r) => r.props["aria-checked"] === true) ?? radios[0];
      active.props.onKeyDown?.({ key, preventDefault: () => {} });
      return calls;
    };

    expect(press("ArrowRight", 3)).toEqual([4]);
    expect(press("ArrowDown", 3)).toEqual([4]);
    expect(press("ArrowLeft", 3)).toEqual([2]);
    expect(press("ArrowUp", 3)).toEqual([2]);
    expect(press("ArrowRight", 5)).toEqual([1]); // 末尾から先頭へ回る
    expect(press("ArrowLeft", 1)).toEqual([5]); // 先頭から末尾へ回る
    expect(press("Home", 3)).toEqual([1]);
    expect(press("End", 3)).toEqual([5]);
    expect(press("Tab", 3)).toEqual([]); // 対応しないキーは onChange を呼ばない
  });

  it("範囲(GET /meta)が取得できていない間は、何も出さない(値を推測しない)", () => {
    expect(input({ min: undefined })).toBe("");
    expect(input({ max: undefined })).toBe("");
  });
});
