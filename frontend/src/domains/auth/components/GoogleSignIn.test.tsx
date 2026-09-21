import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import "../../../lib/i18n";
import { GoogleSignIn, GoogleSignInLink } from "./GoogleSignIn";

describe("GoogleSignInLink(「Google でサインイン」のリンク)", () => {
  it("ボタンではなくリンク(a)で、指定された URL と文言を持ち、ロゴは読み上げない", () => {
    const html = renderToStaticMarkup(<GoogleSignInLink href="/api/auth/google/start?return_to=%2Fshops" label="Sign in with Google" />);
    expect(html).toContain("<a ");
    expect(html).toContain('href="/api/auth/google/start?return_to=%2Fshops"');
    expect(html).toContain("Sign in with Google");
    expect(html).toContain('alt=""');
    expect(html).not.toContain("<button");
  });
});

// GET /meta の内容は、テストごとに差し替える。
const meta = vi.hoisted(() => ({ data: undefined as { loginProviders: string[] } | undefined }));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: meta.data }) }));

describe("GoogleSignIn(サインイン・新規登録の画面に置くボタン)", () => {
  it("GET /meta が Google を返しているときだけ、リンクを出す(文言は、サインインと新規登録で違う)", () => {
    meta.data = { loginProviders: ["google"] };

    const signin = renderToStaticMarkup(<GoogleSignIn mode="signin" />);
    const signup = renderToStaticMarkup(<GoogleSignIn mode="signup" returnTo="/shops" />);

    expect(signin).toContain("Sign in with Google");
    expect(signup).toContain("Sign up with Google");
    expect(signup).toContain("return_to=%2Fshops");
  });

  it("取得できていない間・Google がない・別の方法だけのときは、何も出さない", () => {
    for (const data of [undefined, { loginProviders: [] }, { loginProviders: ["other"] }]) {
      meta.data = data;
      expect(renderToStaticMarkup(<GoogleSignIn mode="signin" />), JSON.stringify(data)).toBe("");
    }
  });
});
