import { describe, it, expect, vi } from "vitest";
import { copyToClipboard } from "./clipboard";

describe("copyToClipboard", () => {
  it("書き込めたときは true を返し、渡した文字列をそのままクリップボードへ書き込む", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    await expect(copyToClipboard("http://localhost:5173/users/abc", { writeText })).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("http://localhost:5173/users/abc");
  });

  it("クリップボードの機能が使えない環境では、例外にせず false を返す", async () => {
    await expect(copyToClipboard("x", undefined)).resolves.toBe(false);
    await expect(copyToClipboard("x", {} as Pick<Clipboard, "writeText">)).resolves.toBe(false);
  });

  it("書き込みをブラウザに拒否されたときは、例外にせず false を返す", async () => {
    const writeText = vi.fn().mockRejectedValue(new DOMException("denied", "NotAllowedError"));
    await expect(copyToClipboard("x", { writeText })).resolves.toBe(false);
  });
});
