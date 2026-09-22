import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import "../../../lib/i18n";
import { GoogleSignIn, GoogleSignInLink } from "./GoogleSignIn";

describe("GoogleSignInLink(「Google で続ける」のリンク)", () => {
  it("ボタンではなくリンク(a)で、指定された URL と文言を持ち、ロゴ(インラインの svg)は読み上げない", () => {
    const html = renderToStaticMarkup(<GoogleSignInLink href="/api/auth/google/start?return_to=%2Fshops" label="Continue with Google" />);
    expect(html).toContain("<a ");
    expect(html).toContain('href="/api/auth/google/start?return_to=%2Fshops"');
    expect(html).toContain("Continue with Google");
    expect(html).toMatch(/<svg[^>]*aria-hidden="true"/);
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<button");
  });
});

// GET /meta の内容は、テストごとに差し替える。
const meta = vi.hoisted(() => ({ data: undefined as { loginProviders: string[] } | undefined }));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: meta.data }) }));

describe("GoogleSignIn(サインイン・新規登録の画面に置くボタン)", () => {
  it("GET /meta が Google を返しているときだけ、リンクを出す(文言は、サインインでも新規登録でも同じ「Continue with Google」)", () => {
    meta.data = { loginProviders: ["google"] };

    const signin = renderToStaticMarkup(<GoogleSignIn mode="signin" />);
    const signup = renderToStaticMarkup(<GoogleSignIn mode="signup" returnTo="/shops" />);

    expect(signin).toContain("Continue with Google");
    expect(signup).toContain("Continue with Google");
    expect(signin + signup).not.toMatch(/Sign (in|up) with Google/);
    expect(signup).toContain("return_to=%2Fshops");
  });

  it("サインインは、「または」の区切りのあとにボタン(フォームの下に置くため)、新規登録は、ボタンのあとに区切り(フォームの上に置くため)の順に並べる", () => {
    meta.data = { loginProviders: ["google"] };

    const signin = renderToStaticMarkup(<GoogleSignIn mode="signin" />);
    const signup = renderToStaticMarkup(<GoogleSignIn mode="signup" />);

    // 区切り(「または」)の位置は、外側の .group の <div> と紛れないよう、テキスト自体の位置で比べる。
    expect(signin.indexOf(">or<")).toBeLessThan(signin.indexOf("<a "));
    expect(signup.indexOf("<a ")).toBeLessThan(signup.indexOf(">or<"));
  });

  it("取得できていない間・Google がない・別の方法だけのときは、何も出さない", () => {
    for (const data of [undefined, { loginProviders: [] }, { loginProviders: ["other"] }]) {
      meta.data = data;
      expect(renderToStaticMarkup(<GoogleSignIn mode="signin" />), JSON.stringify(data)).toBe("");
    }
  });
});
