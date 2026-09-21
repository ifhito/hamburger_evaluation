import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import "../../../lib/i18n";
import { GoogleSignInLink } from "./GoogleSignIn";
import { GoogleConnectionView } from "./GoogleConnection";
import type { Identity } from "../types";

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

describe("GoogleConnectionView(プロフィールの Google の連携)", () => {
  const noop = () => undefined;
  const view = (props: Partial<Parameters<typeof GoogleConnectionView>[0]>) =>
    renderToStaticMarkup(
      <GoogleConnectionView identity={null} loadFailed={false} actionError={null} busy={null} onConnect={noop} onDisconnect={noop} {...props} />,
    );
  const identity = (canUnlink: boolean): Identity => ({ provider: "google", email: "carol@gmail.example", connectedAt: "2026-09-21T00:00:00Z", canUnlink });

  it("結び付いていないときは、「結び付ける」だけを出す", () => {
    const html = view({});
    expect(html).toContain("Not connected.");
    expect(html).toContain("Connect Google");
    expect(html).not.toContain("Disconnect");
  });

  it("結び付いていて、backend が解除してよいと返したときは、メールと「解除」を出す", () => {
    const html = view({ identity: identity(true) });
    expect(html).toContain("Connected as carol@gmail.example");
    expect(html).toContain("Disconnect");
    expect(html).not.toContain("Connect Google");
  });

  it("backend が解除できないと返したとき(サインインする方法がなくなる)は、「解除」を出さず、理由を出す", () => {
    const html = view({ identity: identity(false) });
    expect(html).toContain("Connected as carol@gmail.example");
    expect(html).not.toContain("Disconnect");
    expect(html).toContain("Add a password");
  });

  it("メールに HTML があっても、文字として描画する(解釈しない)", () => {
    const html = view({ identity: { ...identity(true), email: "<img src=x onerror=alert(1)>@evil.example" } });
    expect(html).not.toContain("<img src=x");
    expect(html).toContain("&lt;img src=x");
  });

  it("操作の失敗(サーバーの文言)と、一覧の取得の失敗を、それぞれ表示する", () => {
    const html = view({ actionError: ["Google is your only way to sign in. Add a password before disconnecting it."], loadFailed: true });
    expect(html).toContain("Google is your only way to sign in. Add a password before disconnecting it.");
    expect(html).toContain("Failed to load your Google connection.");
  });

  it("結び付けの処理中は、その操作のボタンだけが処理中の表示になる", () => {
    expect(view({ busy: "connect" })).toContain("Loading");
    expect(view({ identity: identity(true), busy: "disconnect" })).toContain("Loading");
  });
});
