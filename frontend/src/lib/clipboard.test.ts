import { describe, it, expect, vi } from "vitest";
import { copyToClipboard } from "./clipboard";

describe("copyToClipboard", () => {
  it("書き込めたら true を返し、渡したテキストをそのまま書き込む", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    await expect(copyToClipboard("http://localhost:5173/users/abc", { writeText })).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("http://localhost:5173/users/abc");
  });

  it("クリップボード API が無いときは false を返す(例外にしない)", async () => {
    await expect(copyToClipboard("x", undefined)).resolves.toBe(false);
    await expect(copyToClipboard("x", {} as Pick<Clipboard, "writeText">)).resolves.toBe(false);
  });

  it("書き込みを拒否されたときは false を返す(例外にしない)", async () => {
    const writeText = vi.fn().mockRejectedValue(new DOMException("denied", "NotAllowedError"));
    await expect(copyToClipboard("x", { writeText })).resolves.toBe(false);
  });
});
