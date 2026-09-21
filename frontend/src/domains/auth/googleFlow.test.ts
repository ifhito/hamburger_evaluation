import { describe, expect, it } from "vitest";
import type { Meta } from "../../api/meta";
import { GOOGLE_PROVIDER, googleEnabled, googleStartUrl, isGoogleSignIn } from "./googleFlow";
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
});

describe("googleEnabled(GET /meta が Google を使えると返しているか)", () => {
  it("取得できていない間・空の配列・別の方法だけのときは false(値を推測しない)", () => {
    expect(googleEnabled(undefined)).toBe(false);
    expect(googleEnabled({ loginProviders: [] })).toBe(false);
    expect(googleEnabled({ loginProviders: ["other"] })).toBe(false);
  });

  it("meta があっても loginProviders がない(この項目を足す前の応答が、HTTP キャッシュに残っているとき)は、落ちずに false", () => {
    expect(googleEnabled({} as unknown as Pick<Meta, "loginProviders">)).toBe(false);
    expect(googleEnabled({ loginProviders: null } as unknown as Pick<Meta, "loginProviders">)).toBe(false);
  });

  it("google が含まれているときだけ true", () => {
    expect(googleEnabled({ loginProviders: [GOOGLE_PROVIDER] })).toBe(true);
    expect(googleEnabled({ loginProviders: ["other", GOOGLE_PROVIDER] })).toBe(true);
  });
});

describe("googleEnabled(API が、画面と別のオリジンにあるとき)", () => {
  const google = { loginProviders: ["google"] };

  it("API の根が、相対の path(既定の /api)か、画面と同じオリジンの絶対 URL なら、true", () => {
    expect(googleEnabled(google, "/api", "http://localhost:5173")).toBe(true);
    expect(googleEnabled(google, "http://localhost:5173/api", "http://localhost:5173")).toBe(true);
    expect(googleEnabled(google, "/", "https://app.example.com")).toBe(true);
  });

  it("API の根が、画面と別のオリジンの絶対 URL なら、false(交換と結び付けの cookie が届かず、必ず失敗するので、ボタンを出さない)", () => {
    expect(googleEnabled(google, "https://api.example.com/api", "https://app.example.com")).toBe(false);
    expect(googleEnabled(google, "//api.example.com/api", "https://app.example.com")).toBe(false);
    expect(googleEnabled(google, "http://localhost:8080", "http://localhost:5173")).toBe(false);
  });

  it("画面のオリジンが分からない環境(サーバー側の描画など)では、判断せず、meta の内容だけで決める", () => {
    expect(googleEnabled(google, "https://api.example.com/api", null)).toBe(true);
    expect(googleEnabled({ loginProviders: [] }, "https://api.example.com/api", null)).toBe(false);
  });
});
