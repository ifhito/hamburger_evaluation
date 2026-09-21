import { describe, expect, it } from "vitest";
import { GOOGLE_PROVIDER, googleEnabled, googleStartUrl, isGoogleSignIn, isNavigableUrl, landingPath } from "./googleFlow";
import type { GoogleExchangeResponse } from "./types";

describe("googleStartUrl(Google でのサインインを始める URL)", () => {
  it("何も指定しなければ、API の開始の URL だけになる", () => {
    expect(googleStartUrl({}, "/api")).toBe("/api/auth/google/start");
  });

  it("戻り先(アプリの中のパス)は、query に入れて渡す(記号は符号化される)", () => {
    expect(googleStartUrl({ returnTo: "/shops?keyword=a b" }, "/api")).toBe(
      "/api/auth/google/start?return_to=%2Fshops%3Fkeyword%3Da+b",
    );
  });

  it("開始の URL に、結び付けの開始のコード(link_code)は付けない(結び付けは、認証つきの POST で始める)", () => {
    expect(googleStartUrl({ returnTo: "/users/u1" }, "/api")).not.toContain("link_code");
    expect(googleStartUrl({ returnTo: "/users/u1" }, "/api")).toBe("/api/auth/google/start?return_to=%2Fusers%2Fu1");
  });

  it("アプリの外を指す戻り先(外部の URL・//host・バックスラッシュ)は、付けない", () => {
    for (const bad of ["https://evil.example/", "//evil.example", "/\\evil.example", "javascript:alert(1)", "shops", ""]) {
      expect(googleStartUrl({ returnTo: bad }, "/api")).toBe("/api/auth/google/start");
    }
    expect(googleStartUrl({ returnTo: null }, "/api")).toBe("/api/auth/google/start");
  });

  it("API の根の URL の末尾の / は、重ねずに取り除く(絶対 URL でもよい)", () => {
    expect(googleStartUrl({}, "http://localhost:8080/")).toBe("http://localhost:8080/auth/google/start");
    expect(googleStartUrl({}, "/api//")).toBe("/api/auth/google/start");
  });
});

describe("交換の結果の扱い", () => {
  const signedIn: GoogleExchangeResponse = {
    id: "u1", username: "carol", email: "c@example.com", canModerate: false, token: "jwt", returnTo: "/shops",
  };
  const linked: GoogleExchangeResponse = { linked: true, returnTo: "/users/u1" };

  it("token があればサインインの成功、なければ結び付けの成功として区別する", () => {
    expect(isGoogleSignIn(signedIn)).toBe(true);
    expect(isGoogleSignIn(linked)).toBe(false);
  });

  it("戻り先は、アプリの中のパスならそこへ、空や外部を指すときは既定の画面へ", () => {
    expect(landingPath("/shops", "/reviews")).toBe("/shops");
    expect(landingPath("", "/reviews")).toBe("/reviews");
    expect(landingPath("https://evil.example", "/reviews")).toBe("/reviews");
    expect(landingPath("//evil.example", "/reviews")).toBe("/reviews");
  });
});

describe("googleEnabled(GET /meta が Google を使えると返しているか)", () => {
  it("取得できていない間・空の配列・別の方法だけのときは false(値を推測しない)", () => {
    expect(googleEnabled(undefined)).toBe(false);
    expect(googleEnabled({ loginProviders: [] })).toBe(false);
    expect(googleEnabled({ loginProviders: ["other"] })).toBe(false);
  });

  it("google が含まれているときだけ true", () => {
    expect(googleEnabled({ loginProviders: [GOOGLE_PROVIDER] })).toBe(true);
    expect(googleEnabled({ loginProviders: ["other", GOOGLE_PROVIDER] })).toBe(true);
  });
});

describe("isNavigableUrl(Google の URL へ移動してよいか)", () => {
  it("http・https の URL だけ移動してよい(javascript: などのスキーム・URL でない文字列は、移動しない)", () => {
    expect(isNavigableUrl("https://accounts.google.com/o/oauth2/v2/auth?client_id=x")).toBe(true);
    expect(isNavigableUrl("http://127.0.0.1:9000/authorize?x=1")).toBe(true);
    for (const bad of ["javascript:alert(1)", "data:text/html,<script>alert(1)</script>", "file:///etc/passwd", "not a url", ""]) {
      expect(isNavigableUrl(bad)).toBe(false);
    }
  });
});
