import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import "../../../lib/i18n";
import { GoogleSignInLink } from "./GoogleSignIn";

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
