import { describe, it, expect, vi } from "vitest";
import { PHOTO_ACCEPT, shrinkPhoto, type DecodedImage, type PhotoLimits, type PhotoResizeDeps } from "./photoResize";

// 上限の値は backend が決める(GET /meta)ので、ここでの数値は、縮小の計算を確かめるための仮の値である。
const limits: PhotoLimits = { maxEdge: 1600, maxBytes: 5_000_000 };

function photoFile(name: string, bytes: number, type = "image/jpeg"): File {
  return new File([new Uint8Array(bytes)], name, { type });
}

// 画像を実際には読み込まず、指定した縦横の画像として振る舞う偽物。encodeJpeg には、指定した大きさのデータを返させる。
function fakeDeps(image: Partial<DecodedImage> & { width: number; height: number }, encodedBytes = 100_000) {
  const close = vi.fn();
  const encodeJpeg = vi.fn<PhotoResizeDeps["encodeJpeg"]>(async () => {
    return new Blob([new Uint8Array(encodedBytes)], { type: "image/jpeg" });
  });
  const deps: PhotoResizeDeps = {
    decode: vi.fn(async () => ({ ...image, close })),
    encodeJpeg,
  };
  return { deps, encodeJpeg, close };
}

describe("shrinkPhoto", () => {
  it("長辺もファイルの大きさも上限に収まる写真は、縮小せず、元のファイルをそのまま返す", async () => {
    const original = photoFile("small.jpg", 300_000);
    const { deps, encodeJpeg } = fakeDeps({ width: 1200, height: 900 });

    const result = await shrinkPhoto(original, limits, deps);

    expect(result).toBe(original);
    expect(encodeJpeg).not.toHaveBeenCalled();
  });

  it("長辺が上限ちょうどの写真は、縮小しない(上限を超えたときだけ縮小する)", async () => {
    const original = photoFile("edge.jpg", 300_000);
    const { deps, encodeJpeg } = fakeDeps({ width: 1600, height: 1200 });

    expect(await shrinkPhoto(original, limits, deps)).toBe(original);
    expect(encodeJpeg).not.toHaveBeenCalled();
  });

  it("横長で長辺が上限を超える写真は、縦横の比を保ったまま、長辺が上限になるよう縮小した JPEG にする", async () => {
    const original = photoFile("landscape.jpg", 4_000_000);
    const { deps, encodeJpeg } = fakeDeps({ width: 4000, height: 3000 });

    const result = await shrinkPhoto(original, limits, deps);

    expect(encodeJpeg).toHaveBeenCalledTimes(1);
    expect(encodeJpeg.mock.calls[0].slice(1, 3)).toEqual([1600, 1200]);
    expect(result).not.toBe(original);
    expect(result.type).toBe("image/jpeg");
    expect(result.size).toBe(100_000);
  });

  it("縦長で長辺が上限を超える写真は、縦を上限に合わせて縮小する", async () => {
    const original = photoFile("portrait.jpg", 4_000_000);
    const { deps, encodeJpeg } = fakeDeps({ width: 3000, height: 4000 });

    await shrinkPhoto(original, limits, deps);

    expect(encodeJpeg.mock.calls[0].slice(1, 3)).toEqual([1200, 1600]);
  });

  it("縦横は上限内で、ファイルの大きさだけが上限を超える写真は、縦横はそのままで JPEG にし直して小さくする", async () => {
    const original = photoFile("heavy.png", 8_000_000, "image/png");
    const { deps, encodeJpeg } = fakeDeps({ width: 1500, height: 1000 });

    const result = await shrinkPhoto(original, limits, deps);

    expect(encodeJpeg.mock.calls[0].slice(1, 3)).toEqual([1500, 1000]);
    expect(result.type).toBe("image/jpeg");
    expect(result.size).toBeLessThan(original.size);
  });

  it("縮小したファイルの名前は、拡張子を .jpg にする(HEIC や PNG でも、出力は JPEG のため)", async () => {
    const names = [
      ["IMG_0001.HEIC", "IMG_0001.jpg"],
      ["my.burger.png", "my.burger.jpg"],
      ["noextension", "noextension.jpg"],
    ];
    for (const [before, after] of names) {
      const { deps } = fakeDeps({ width: 4000, height: 3000 });
      const result = await shrinkPhoto(photoFile(before, 4_000_000), limits, deps);
      expect(result.name).toBe(after);
    }
  });

  it("縮小の途中でどの結果になっても、読み込んだ画像は解放する", async () => {
    const shrunk = fakeDeps({ width: 4000, height: 3000 });
    await shrinkPhoto(photoFile("a.jpg", 4_000_000), limits, shrunk.deps);
    expect(shrunk.close).toHaveBeenCalledTimes(1);

    const untouched = fakeDeps({ width: 800, height: 600 });
    await shrinkPhoto(photoFile("b.jpg", 100_000), limits, untouched.deps);
    expect(untouched.close).toHaveBeenCalledTimes(1);

    const failing = fakeDeps({ width: 4000, height: 3000 });
    failing.encodeJpeg.mockRejectedValue(new Error("canvas failed"));
    await shrinkPhoto(photoFile("c.jpg", 4_000_000), limits, failing.deps);
    expect(failing.close).toHaveBeenCalledTimes(1);
  });

  describe("縮小できないときは、失敗にせず、元のファイルをそのまま返す(受け付けるかどうかは backend が判断する)", () => {
    it("ブラウザが画像を読み込めない(HEIC を扱えないブラウザなど)", async () => {
      const original = photoFile("IMG_0001.HEIC", 3_000_000, "image/heic");
      const deps: PhotoResizeDeps = {
        decode: vi.fn(async () => {
          throw new Error("The source image could not be decoded.");
        }),
        encodeJpeg: vi.fn(),
      };

      expect(await shrinkPhoto(original, limits, deps)).toBe(original);
      expect(deps.encodeJpeg).not.toHaveBeenCalled();
    });

    it("canvas から JPEG を作れなかった(null が返った)", async () => {
      const original = photoFile("big.jpg", 4_000_000);
      const { deps, encodeJpeg } = fakeDeps({ width: 4000, height: 3000 });
      encodeJpeg.mockResolvedValue(null);

      expect(await shrinkPhoto(original, limits, deps)).toBe(original);
    });

    it("canvas の処理が例外になった", async () => {
      const original = photoFile("big.jpg", 4_000_000);
      const { deps, encodeJpeg } = fakeDeps({ width: 4000, height: 3000 });
      encodeJpeg.mockRejectedValue(new Error("out of memory"));

      expect(await shrinkPhoto(original, limits, deps)).toBe(original);
    });

    it("縮小した結果が、元のファイル以上の大きさになった(かえって大きくならないようにする)", async () => {
      const original = photoFile("already-small-but-large-edge.jpg", 200_000);
      const { deps } = fakeDeps({ width: 4000, height: 3000 }, 250_000);

      expect(await shrinkPhoto(original, limits, deps)).toBe(original);
    });

    it("画像の縦横が 0 と読めた", async () => {
      const original = photoFile("weird.jpg", 4_000_000);
      const { deps, encodeJpeg } = fakeDeps({ width: 0, height: 0 });

      expect(await shrinkPhoto(original, limits, deps)).toBe(original);
      expect(encodeJpeg).not.toHaveBeenCalled();
    });
  });

  it("上限が小さくなっても、その値で縮小する(上限の値は引数で決まり、この関数は値を持たない)", async () => {
    const { deps, encodeJpeg } = fakeDeps({ width: 1200, height: 900 });

    await shrinkPhoto(photoFile("a.jpg", 900_000), { maxEdge: 600, maxBytes: 5_000_000 }, deps);

    expect(encodeJpeg.mock.calls[0].slice(1, 3)).toEqual([600, 450]);
  });
});

describe("PHOTO_ACCEPT(ファイル選択の絞り込み)", () => {
  it("JPEG・PNG・WebP に加えて、iPhone の写真(HEIC / HEIF)を、種類名と拡張子の両方で選べる", () => {
    const accepted = PHOTO_ACCEPT.split(",");

    expect(accepted).toEqual(
      expect.arrayContaining(["image/jpeg", "image/png", "image/webp", "image/heic", "image/heif", ".heic", ".heif"]),
    );
  });
});
