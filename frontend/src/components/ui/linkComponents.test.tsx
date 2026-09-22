import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { LinkButton } from "./LinkButton";
import { TextLink } from "./TextLink";

const html = (node: React.ReactElement) => renderToStaticMarkup(<MemoryRouter>{node}</MemoryRouter>);
const classNames = (markup: string) =>
  (markup.match(/class="([^"]*)"/)?.[1] ?? "").split(" ").map((c) => c.replace(/^_(.+)_[0-9a-f]+$/, "$1"));

describe("LinkButton", () => {
  it("移動先を持つリンク(a)1 つだけで、その中にボタンを入れない", () => {
    const markup = html(<LinkButton to="/users/7/edit">Edit</LinkButton>);

    expect(markup).toMatch(/^<a /);
    expect(markup).toContain('href="/users/7/edit"');
    expect(markup).not.toContain("<button");
    expect(classNames(markup)).toContain("linkButton");
  });

  it("呼び出し側の class も、いっしょに付く", () => {
    expect(classNames(html(<LinkButton to="/x" className="extra">Go</LinkButton>))).toEqual(["linkButton", "extra"]);
  });
});

describe("TextLink", () => {
  it("移動先を持つ、下線つきのリンク(a)になる", () => {
    const markup = html(<TextLink to="/users/7">Back</TextLink>);

    expect(markup).toContain('href="/users/7"');
    expect(classNames(markup)).toEqual(["textLink"]);
  });
});
