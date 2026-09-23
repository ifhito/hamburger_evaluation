// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import "../lib/i18n";
import { byText, cleanup, mount, need } from "../test/dom";
import { Layout } from "./Layout";

vi.mock("../domains/auth/AuthProvider", () => ({ useAuth: () => ({ user: null, isLoading: false }) }));

const show = () => mount(<MemoryRouter><Layout>content</Layout></MemoryRouter>);

afterEach(cleanup);

describe("Layout のヘッダーのリンク", () => {
  it("ブランドのロゴは、ホームページ(/)へのリンクである", async () => {
    const page = await show();
    const brand = need(page.querySelector("a"), "ブランドのリンク");
    expect(brand.getAttribute("href")).toBe("/");
  });

  it("ナビゲーションの「Shops」は、/shops へのリンクのままである", async () => {
    const page = await show();
    const shopsLink = need(byText<HTMLAnchorElement>(page, "a", "Shops"), "Shops のリンク");
    expect(shopsLink.getAttribute("href")).toBe("/shops");
  });
});
