// @vitest-environment jsdom
import { act } from "react";
import { Provider as JotaiProvider } from "jotai";
import useSWR, { SWRConfig, useSWRConfig } from "swr";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client/buildApiClient";
import { cleanup, mount } from "../../test/dom";
import { AuthProvider, useAuth } from "./AuthProvider";
import { authApi, authApiClient } from "./api/authApiClient";

vi.mock("./api/authApiClient", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api/authApiClient")>();
  return {
    ...actual,
    authApi: {
      ...actual.authApi,
      me: vi.fn(),
      logout: vi.fn(),
      login: vi.fn(),
    },
  };
});

const mockedAuthApi = vi.mocked(authApi);
const defaultAdapter = authApiClient.defaults.adapter;

function Probe({ onReady }: { onReady: (ctx: ReturnType<typeof useAuth>, cache: Map<string, unknown>) => void }) {
  const auth = useAuth();
  const { cache } = useSWRConfig();
  onReady(auth, cache as unknown as Map<string, unknown>);
  return (
    <div>
      <span data-testid="user">{auth.user ? auth.user.username : "guest"}</span>
      <span data-testid="loading">{String(auth.isLoading)}</span>
    </div>
  );
}

beforeEach(() => {
  localStorage.clear();
  authApiClient.defaults.adapter = defaultAdapter;
  mockedAuthApi.me.mockReset();
  mockedAuthApi.logout.mockReset();
  mockedAuthApi.login.mockReset();
});
afterEach(async () => {
  authApiClient.defaults.adapter = defaultAdapter;
  await cleanup();
});

describe("AuthProvider(認証状態の同期とキャッシュ消去)", () => {
  it("サインアウトすると、未マウントのキャッシュを消しつつ表示中の SWR キーは再取得し、未認証での再サインアウトでは API を呼ばない", async () => {
    localStorage.setItem("token", "tok-1");
    mockedAuthApi.me.mockResolvedValue({ id: "u1", username: "alice", email: "alice@example.com", canModerate: false });
    mockedAuthApi.logout.mockResolvedValue({ message: "ok" });

    let rankingFetchCount = 0;
    function RankingConsumer() {
      const { data } = useSWR("/burgers?page=1", async () => {
        rankingFetchCount += 1;
        return `ranking-${rankingFetchCount}`;
      });
      return <span data-testid="ranking">{data ?? "loading"}</span>;
    }

    let latestAuth!: ReturnType<typeof useAuth>;
    let swrCache!: Map<string, unknown>;
    const map = new Map<string, unknown>([
      ["/meta", { data: { rating: { min: 1, max: 5 } } }],
      ["/me/identities|u1", { data: { google: null } }],
    ]);

    const page = await mount(
      <SWRConfig value={{ provider: () => map as never, dedupingInterval: 0 }}>
        <JotaiProvider>
          <AuthProvider>
            <Probe
              onReady={(ctx, cache) => {
                latestAuth = ctx;
                swrCache = cache;
              }}
            />
            <RankingConsumer />
          </AuthProvider>
        </JotaiProvider>
      </SWRConfig>,
    );

    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("alice");
    expect(page.querySelector('[data-testid="ranking"]')?.textContent).toBe("ranking-1");

    await act(async () => {
      await latestAuth.logout();
    });

    expect(mockedAuthApi.logout).toHaveBeenCalledTimes(1);
    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("guest");
    expect(swrCache.has("/meta")).toBe(true);
    expect(swrCache.has("/me/identities|u1")).toBe(false);
    expect(page.querySelector('[data-testid="ranking"]')?.textContent).toBe("ranking-2");

    await act(async () => {
      await latestAuth.logout();
    });
    expect(mockedAuthApi.logout).toHaveBeenCalledTimes(1);
  });

  it("セッション途中の 401 は、現在のトークンと一致するときだけ認証状態を消し、古いトークンの 401 は無視する", async () => {
    localStorage.setItem("token", "tok-live");
    mockedAuthApi.me.mockResolvedValue({ id: "u1", username: "alice", email: "alice@example.com", canModerate: false });

    const page = await mount(
      <SWRConfig value={{ provider: () => new Map() }}>
        <JotaiProvider>
          <AuthProvider>
            <Probe onReady={() => undefined} />
          </AuthProvider>
        </JotaiProvider>
      </SWRConfig>,
    );
    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("alice");

    // すでに新しいトークンに切り替わったあとに遅れて返ってきた古いトークンの 401 は無視する
    authApiClient.defaults.adapter = async (config) => {
      config.headers.Authorization = "Bearer tok-stale";
      const response = { data: { error: "Unauthorized" }, status: 401, statusText: "", headers: {}, config };
      throw Object.assign(new Error("401"), { isAxiosError: true, response, config });
    };
    await act(async () => {
      await authApiClient.get("/oauth/grants").catch(() => undefined);
    });
    expect(localStorage.getItem("token")).toBe("tok-live");
    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("alice");

    // 現在保持しているトークンの 401 では認証状態を消去する
    authApiClient.defaults.adapter = async (config) => {
      const response = { data: { error: "Unauthorized" }, status: 401, statusText: "", headers: {}, config };
      throw Object.assign(new Error("401"), { isAxiosError: true, response, config });
    };
    await act(async () => {
      await authApiClient.get("/oauth/grants").catch((e: unknown) => {
        expect(e).toBeInstanceOf(ApiError);
      });
    });

    expect(localStorage.getItem("token")).toBeNull();
    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("guest");
  });

  it("別のタブでトークンが切り替わったとき、取得成功で新ユーザーへ同期し、通信失敗時は前のユーザーを残さない", async () => {
    localStorage.setItem("token", "tok-a");
    mockedAuthApi.me.mockResolvedValueOnce({ id: "u1", username: "alice", email: "alice@example.com", canModerate: false });

    const page = await mount(
      <SWRConfig value={{ provider: () => new Map() }}>
        <JotaiProvider>
          <AuthProvider>
            <Probe onReady={() => undefined} />
          </AuthProvider>
        </JotaiProvider>
      </SWRConfig>,
    );
    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("alice");

    mockedAuthApi.me.mockResolvedValueOnce({ id: "u2", username: "bob", email: "bob@example.com", canModerate: true });
    localStorage.setItem("token", "tok-b");
    await act(async () => {
      window.dispatchEvent(new StorageEvent("storage", { key: "token", newValue: "tok-b" }));
    });
    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("bob");

    // さらに別トークンへ切り替わり GET /me が一時障害(503)で失敗したとき、前のユーザー(bob)を残さない
    mockedAuthApi.me.mockRejectedValueOnce(new ApiError(["Service unavailable"], 503));
    localStorage.setItem("token", "tok-c");
    await act(async () => {
      window.dispatchEvent(new StorageEvent("storage", { key: "token", newValue: "tok-c" }));
    });
    expect(page.querySelector('[data-testid="user"]')?.textContent).toBe("guest");
    expect(page.querySelector('[data-testid="loading"]')?.textContent).toBe("false");
    expect(localStorage.getItem("token")).toBe("tok-c");
  });
});
