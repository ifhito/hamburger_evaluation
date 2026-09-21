// 写真を送る前に、ブラウザの中で、backend が決めた上限に収まるよう縮小する(canvas で描き直して JPEG にする)。
//
// 縮小するのは、次のどちらかのときだけである:
//   - 長辺が maxEdge ピクセルを超える(iPhone の 48 メガ画素の写真など)
//   - ファイルが maxBytes バイトを超える
// どちらでもなければ、元のファイルをそのまま返す。上限の値は backend が決めていて(GET /meta の photo)、
// ここでは定数を持たない。受け付けるかどうかの判断も backend が行う(縮小は、送る量を減らすための前処理で、
// 縮小に失敗しても、元のファイルを送れば、backend が判断して、理由つきのメッセージを返す)。
//
// 縮小に失敗したとき(ブラウザが画像を読み込めない、canvas を使えない、など)は、例外にせず、元のファイルを返す。
// そのまま送れば、backend が判断して、受け付けるか、理由つきのメッセージを返す。

export interface PhotoLimits {
  /** 長辺の上限(ピクセル) */
  maxEdge: number;
  /** ファイルの大きさの上限(バイト) */
  maxBytes: number;
}

/** 読み込んだ画像。createImageBitmap の戻り値と、テストの偽物の両方が満たす最小の形。 */
export interface DecodedImage {
  width: number;
  height: number;
  close?: () => void;
}

/** ブラウザの機能への入り口。テストで差し替えられるように、引数で受け取る。 */
export interface PhotoResizeDeps {
  /** ファイルを画像として読み込む。EXIF の向きは、読み込んだ時点で画素に反映される(imageOrientation: from-image)。 */
  decode: (file: Blob) => Promise<DecodedImage>;
  /** 画像を width x height の canvas に描き、JPEG の Blob にする。作れなければ null。 */
  encodeJpeg: (image: DecodedImage, width: number, height: number, quality: number) => Promise<Blob | null>;
}

/** JPEG に再エンコードするときの画質(0〜1)。写真として十分に見える範囲で、大きさを抑える値。 */
const JPEG_QUALITY = 0.85;

const browserDeps: PhotoResizeDeps = {
  // imageOrientation: "from-image" は、写真に付いた「向き」の情報(EXIF)を画素に反映する指定である。
  // canvas に描き直すと、向きの情報は失われるので、ここで反映しておかないと、縦向きの写真が横向きになる。
  decode: (file) => createImageBitmap(file, { imageOrientation: "from-image" }),
  encodeJpeg: (image, width, height, quality) => {
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext("2d");
    if (!context) return Promise.resolve(null);
    context.drawImage(image as CanvasImageSource, 0, 0, width, height);
    return new Promise((resolve) => canvas.toBlob(resolve, "image/jpeg", quality));
  },
};

/** 縮小後の名前。拡張子を .jpg にする(元が PNG や WebP でも、出力は JPEG のため)。 */
function jpegName(original: string): string {
  const base = original.replace(/\.[^./\\]+$/, "");
  return `${base || "photo"}.jpg`;
}

/**
 * file が上限を超えているときだけ、上限に収まるよう縮小した JPEG の File を返す。
 * 超えていない・縮小できない・縮小しても小さくならないときは、元の file をそのまま返す(拡大はしない)。
 */
export async function shrinkPhoto(
  file: File,
  limits: PhotoLimits,
  deps: PhotoResizeDeps = browserDeps,
): Promise<File> {
  let image: DecodedImage;
  try {
    image = await deps.decode(file);
  } catch {
    return file;
  }
  try {
    const longEdge = Math.max(image.width, image.height);
    if (!(longEdge > 0)) return file;
    const tooManyPixels = longEdge > limits.maxEdge;
    if (!tooManyPixels && file.size <= limits.maxBytes) return file;

    // 長辺が上限に収まる倍率(1 を超えない。拡大はしない)。ファイルの大きさだけが超えているときは、
    // 縦横はそのままで、JPEG に再エンコードして小さくする。
    const scale = Math.min(1, limits.maxEdge / longEdge);
    const width = Math.max(1, Math.round(image.width * scale));
    const height = Math.max(1, Math.round(image.height * scale));
    let blob: Blob | null;
    try {
      blob = await deps.encodeJpeg(image, width, height, JPEG_QUALITY);
    } catch {
      return file;
    }
    // 作れなかった、または、かえって大きくなったときは、元のファイルを送る。
    if (!blob || blob.size >= file.size) return file;
    return new File([blob], jpegName(file.name), { type: "image/jpeg", lastModified: file.lastModified });
  } finally {
    image.close?.();
  }
}
