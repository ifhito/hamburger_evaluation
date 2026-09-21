import { describe, it, expect } from "vitest";
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

describe("Button", () => {
  it("種類ごとに違う見た目の class を持ち、押せる状態では disabled でない", () => {
    expect(html(<Button variant="primary">Save</Button>)).toMatch(/class="[^"]*primary/);
    expect(html(<Button variant="secondary">Cancel</Button>)).toMatch(/class="[^"]*secondary/);
    expect(html(<Button variant="danger">Delete</Button>)).toMatch(/class="[^"]*danger/);
    expect(html(<Button variant="dangerSolid">Delete</Button>)).toMatch(/class="[^"]*dangerSolid/);
    expect(html(<Button variant="dark">Confirm reject</Button>)).toMatch(/class="[^"]*dark/);
    expect(html(<Button>Save</Button>)).not.toContain("disabled");
  });

  it("送信中は、押せず、aria-busy になり、指定した文言(なければ Loading…)を出す", () => {
    const withLabel = html(<Button isLoading loadingLabel="Posting…">Post</Button>);
    expect(withLabel).toContain("disabled");
    expect(withLabel).toContain('aria-busy="true"');
    expect(withLabel).toContain("Posting…");
    expect(withLabel).not.toContain(">Post<");
    expect(html(<Button isLoading>Post</Button>)).toContain("Loading…");
  });

  it("無効のときは、灰色の面の class になり、押せない。削除の無効は、文字だけ赤の class になる", () => {
    const off = html(<Button disabled>Post</Button>);
    expect(off).toContain("disabled");
    expect(off).toMatch(/class="[^"]*off/);
    expect(off).not.toMatch(/class="[^"]*primary/);
    expect(html(<Button variant="danger" disabled>Delete</Button>)).toMatch(/class="[^"]*offDanger/);
    expect(html(<Button variant="dangerSolid" disabled>Delete</Button>)).not.toMatch(/dangerSolid/);
  });

  it("form の中でも、指定しない限り送信ボタンにならない(type=button)", () => {
    expect(html(<Button>Save</Button>)).toContain('type="button"');
    expect(html(<Button type="submit">Save</Button>)).toContain('type="submit"');
  });
});

describe("Badge と Card", () => {
  it("札は、状態の文字を出し、見た目は outline と accent の 2 種類", () => {
    expect(html(<Badge>Pending</Badge>)).not.toMatch(/accent/);
    expect(html(<Badge tone="accent">Public</Badge>)).toMatch(/class="[^"]*accent/);
    expect(html(<Badge>Pending</Badge>)).toContain("Pending");
  });

  it("カードは、強調と余白なしを選べる", () => {
    expect(html(<Card>x</Card>)).toMatch(/padded/);
    expect(html(<Card padded={false}>x</Card>)).not.toMatch(/padded/);
    expect(html(<Card current>x</Card>)).toMatch(/current/);
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
  it("ラベルを入力欄に結び付け、説明と文字数のカウンターを aria-describedby で結び付ける", () => {
    const out = html(<TextField id="name" label="Name" hint="Shown to others" counter={{ value: "ab", max: 5 }} />);
    expect(out).toContain('for="name"');
    expect(out).toContain('aria-describedby="name-hint name-counter"');
    expect(out).toContain("2 / 5");
  });

  it("上限が未取得(undefined)のときは、カウンターを出さず、結び付けもしない(値を推測しない)", () => {
    const out = html(<TextArea id="c" label="Comment" counter={{ value: "abc", max: undefined }} />);
    expect(out).not.toContain("/ ");
    expect(out).not.toContain("aria-describedby");
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

  it("読み込み中は、role=status で読み上げ、灰色の帯は装飾として読み上げない", () => {
    const out = html(<Loading />);
    expect(out).toContain('role="status"');
    expect(out).toContain("Grilling…");
    expect(out).toContain("Just a moment");
  });

  it("見つからないときは、404 と文言と、操作(あれば)を出す", () => {
    const out = html(<NotFound action={<Button variant="secondary">Back to shops</Button>} />);
    expect(out).toContain("404");
    expect(out).toContain("Sold out");
    expect(out).toContain("Page not found");
    expect(out).toContain("Back to shops");
  });
});

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

  it("最大値は呼び出し側が渡した値(GET /meta)で決まり、最大値が違えば水位の割合も変わる", () => {
    const gradients = (max: number) => html(<RatingBurger value={2} max={max} size="lg" />).match(/<linearGradient/g)?.length ?? 0;
    expect(html(<RatingBurger value={3} max={3} size="lg" />)).not.toContain("linearGradient");
    expect(gradients(4)).toBeGreaterThan(0);
    expect(html(<RatingBurger value={2} max={4} />)).toContain("out of 4");
  });

  it("最大値が 0 以下でも例外にならず、空のバーガーになる", () => {
    expect(() => html(<RatingBurger value={3} max={0} />)).not.toThrow();
  });

  it("評価が 0 のときは水位の途中で塗る部品がなく、最大のときも塗り分けがない", () => {
    expect(html(<RatingBurger value={0} max={5} size="lg" />)).not.toContain("linearGradient");
    expect(html(<RatingBurger value={5} max={5} size="lg" />)).not.toContain("linearGradient");
  });

  it("絵だけの部品は、装飾として読み上げず、数字を持たない", () => {
    const out = html(<RatingBurgerIcon ratio={0.5} size="sm" />);
    expect(out).toContain('aria-hidden="true"');
    expect(out).not.toContain("<b");
  });

  it("同じ画面に複数置いても、水位の塗り分け(gradient)の id が重ならない", () => {
    const out = html(<><RatingBurgerIcon ratio={0.55} /><RatingBurgerIcon ratio={0.55} /></>);
    const ids = [...out.matchAll(/<linearGradient id="([^"]+)"/g)].map((m) => m[1]);
    expect(ids.length).toBeGreaterThan(1);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe("RatingInput(入力)", () => {
  const input = (props: Partial<React.ComponentProps<typeof RatingInput>> = {}) =>
    html(<RatingInput name="rating" label="Rating" value={3} onChange={() => {}} min={1} max={5} {...props} />);

  it("最小〜最大の数字のラジオボタンを並べ、選んでいる値だけが checked になる", () => {
    const out = input();
    expect(out.match(/type="radio"/g)?.length).toBe(5);
    expect(out.match(/checked=""/g)?.length).toBe(1);
    expect(out).toMatch(/value="3"[^>]*checked=""|checked=""[^>]*value="3"/);
  });

  it("ボタンの数は、渡された最小・最大(GET /meta)で決まる(frontend に評価の範囲を持たない)", () => {
    expect(input({ min: 1, max: 7 }).match(/type="radio"/g)?.length).toBe(7);
    expect(input({ min: 1, max: 3 }).match(/type="radio"/g)?.length).toBe(3);
  });

  it("同じ name のラジオボタンの集まりで、ラベルを凡例(legend)として読み上げる", () => {
    const out = input();
    expect(out).toContain("<fieldset");
    expect(out).toContain("<legend");
    expect(out.match(/name="rating"/g)?.length).toBe(5);
  });

  it("選んだ値を大きな数字と「out of 5」でも見せる", () => {
    const out = input({ value: 4 });
    expect(out).toContain("<b>4</b>");
    expect(out).toContain("out of 5");
  });

  it("まだ選んでいない(null)ときは、空のバーガーで、どのボタンも checked でなく、数字の場所は「–」", () => {
    const out = input({ value: null });
    expect(out).not.toContain("checked");
    expect(out).toContain("<b>–</b>");
  });

  it("範囲(GET /meta)が取得できていない間は、何も出さない(値を推測しない)", () => {
    expect(input({ min: undefined })).toBe("");
    expect(input({ max: undefined })).toBe("");
  });
});
