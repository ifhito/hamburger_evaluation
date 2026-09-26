// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import UserDetailPage from "./UserDetailPage";

// プロフィールの本体(名前・自己紹介・操作・レビューの並び)の出し分けだけを確かめる。データの取得は差し替える。
const state = vi.hoisted(() => ({
  profile: undefined as undefined | { id: string; username: string; bio: string; email?: string; canEdit: boolean },
  userLoading: false,
  userError: undefined as unknown,
  reviews: [] as { id: string; rating: number; comment: string; photoUrl: null; createdAt: string; visitedAt: string | null; user: null; burger: null }[],
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "9", username: "dave" }, isLoading: false }) }));
vi.mock("../hooks/useUser", () => ({
  useUser: () => ({ data: state.profile, isLoading: state.userLoading, error: state.userError }),
}));
vi.mock("../../reviews/hooks/useReviews", () => ({
  useReviews: () => ({ data: state.reviews, isLoading: false, error: undefined, hasNextPage: false, fetchNextPage: vi.fn(), isFetchingNextPage: false }),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));
vi.mock("../../auth/components/GoogleConnection", () => ({ GoogleConnection: () => null }));
vi.mock("../../oauth/components/ConnectedApps", () => ({ ConnectedApps: () => null }));

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/users/7"]}>
      <Routes>
        <Route path="/users/:id" element={<UserDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.profile = { id: "7", username: "carol", bio: "I like cheese", email: "c@example.com", canEdit: true };
  state.userLoading = false;
  state.userError = undefined;
  state.reviews = [];
});
afterEach(cleanup);

describe("UserDetailPage のプロフィール本体", () => {
  it("名前を見出し(h1)に、自己紹介とコピーの操作を出し、画面の見出しは重ねて出さない", async () => {
    const page = await show();

    expect(page.querySelectorAll("h1")).toHaveLength(1);
    expect(page.querySelector("h1")?.textContent).toBe("carol");
    expect(page.textContent).toContain("I like cheese");
    expect(page.textContent).toContain("Copy link");
  });

  it("本人の情報にメールが含まれていても、プロフィールには表示しない", async () => {
    let page = await show();
    expect(page.textContent).not.toContain("c@example.com");
    await cleanup();

    state.profile = { id: "7", username: "carol", bio: "", canEdit: false };
    page = await show();
    expect(page.textContent).not.toContain("c@example.com");
  });

  it("「Edit Profile」は、backend が canEdit を返したときだけ、編集の画面へのリンクとして出す", async () => {
    let page = await show();
    const link = page.querySelector<HTMLAnchorElement>('a[href="/users/7/edit"]');
    expect(link?.textContent).toBe("Edit Profile");
    expect(link?.querySelector("button")).toBeNull();
    await cleanup();

    state.profile = { id: "7", username: "carol", bio: "", canEdit: false };
    page = await show();
    expect(page.querySelector('a[href="/users/7/edit"]')).toBeNull();
  });

  it("レビューが 0 件のとき: 本人には空の画面(遊びの文言)、他人には「No reviews yet.」だけを出す", async () => {
    let page = await show();
    expect(page.textContent).toContain("No burgers tried yet");
    await cleanup();

    state.profile = { id: "7", username: "carol", bio: "", canEdit: false };
    page = await show();
    expect(page.textContent).not.toContain("No burgers tried yet");
    expect(page.textContent).toContain("No reviews yet.");
  });

  it("レビューがあるとき: 「Newest first」と、レビューごとのカードを出す", async () => {
    state.reviews = [{ id: "r1", rating: 4, comment: "Nice", photoUrl: null, createdAt: "2026-09-21T00:00:00Z", visitedAt: null, user: null, burger: null }];
    const page = await show();

    expect(page.textContent).toContain("Newest first");
    expect(page.querySelectorAll("time")).toHaveLength(1);
    expect(page.textContent).toContain("Nice");
  });

  it("取得中は読み込みの表示、取得に失敗したときは、画面の見出しとエラーを出す", async () => {
    state.profile = undefined;
    state.userLoading = true;
    let page = await show();
    expect(page.querySelector('[role="status"]')).not.toBeNull();
    await cleanup();

    state.userLoading = false;
    state.userError = new Error("boom");
    page = await show();
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Failed to load user.");
    expect(page.querySelector("h1")?.textContent).toBe("User Profile");
  });
});
