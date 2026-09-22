// @vitest-environment jsdom
import { vi } from "vitest";
// API の根が、画面と別のオリジンの絶対 URL の環境。import(API_BASE_URL の評価)より前に、固定する。
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "https://api.example.com/api"));
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { SWRConfig } from "swr";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import { authApi } from "../api/authApiClient";
import { GoogleConnection } from "./GoogleConnection";
import { GoogleSignIn } from "./GoogleSignIn";

vi.mock("../api/authApiClient", () => ({ authApi: { listIdentities: vi.fn(), unlinkGoogle: vi.fn(), startGoogleLink: vi.fn() } }));
// GET /meta は、Google を有効と返している。
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: { loginProviders: ["google"] } }) }));

beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

describe("API が画面と別のオリジンにあるとき(交換と結び付けの cookie が届かず、必ず失敗する)", () => {
  it("GET /meta が Google を返していても、サインイン・新規登録のボタンも、プロフィールの連携の欄も出さず、一覧も取得しない", async () => {
    const page = await mount(
      <SWRConfig value={{ provider: () => new Map() }}>
        <GoogleSignIn mode="signin" />
        <GoogleSignIn mode="signup" />
        <GoogleConnection viewerId="7" />
      </SWRConfig>,
    );

    expect(page.textContent).toBe("");
    expect(authApi.listIdentities).not.toHaveBeenCalled();
  });
});
