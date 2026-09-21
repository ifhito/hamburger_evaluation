// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import UserDetailPage from "./UserDetailPage";

// プロフィールの、連携の欄(Google・許可したアプリ)を出す条件だけを確かめる。データの取得と、欄の中身は、差し替える。
const state = vi.hoisted(() => ({ canEdit: true, authUser: { id: "7", username: "carol" } as { id: string; username: string } | null }));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: state.authUser, isLoading: false }) }));
vi.mock("../hooks/useUser", () => ({
  useUser: () => ({ data: { id: "7", username: "carol", bio: "", email: "c@example.com", canEdit: state.canEdit }, isLoading: false, error: undefined }),
}));
vi.mock("../../reviews/hooks/useReviews", () => ({
  useReviews: () => ({ data: [], isLoading: false, error: undefined, hasNextPage: false, fetchNextPage: vi.fn(), isFetchingNextPage: false }),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));
vi.mock("../../auth/components/GoogleConnection", () => ({ GoogleConnection: ({ viewerId }: { viewerId: string }) => <p>google-connection:{viewerId}</p> }));
vi.mock("../../oauth/components/ConnectedApps", () => ({ ConnectedApps: ({ viewerId }: { viewerId: string }) => <p>connected-apps:{viewerId}</p> }));

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/users/7"]}>
      <Routes>
        <Route path="/users/:id" element={<UserDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.canEdit = true;
  state.authUser = { id: "7", username: "carol" };
});
afterEach(cleanup);

describe("UserDetailPage の連携の欄(Google・許可したアプリ)", () => {
  it("本人のプロフィール(backend が canEdit を返し、ログイン中)にだけ、出す", async () => {
    const page = await show();

    expect(page.textContent).toContain("google-connection:7");
    expect(page.textContent).toContain("connected-apps:7");
  });

  it("他人のプロフィール(canEdit が false)では、どちらも出さない", async () => {
    state.canEdit = false;
    const page = await show();

    expect(page.textContent).not.toContain("google-connection");
    expect(page.textContent).not.toContain("connected-apps");
  });

  it("ログインしていないときは、どちらも出さない", async () => {
    state.authUser = null;
    const page = await show();

    expect(page.textContent).not.toContain("google-connection");
    expect(page.textContent).not.toContain("connected-apps");
  });
});
