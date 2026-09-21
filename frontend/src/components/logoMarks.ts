import markSvg from "../assets/logo/burgerstack-mark.svg?raw";
import markSmallSvg from "../assets/logo/burgerstack-mark-small.svg?raw";

// ロゴの記号(1 本の連続した線)。元は design/assets/logo/ の SVG で、src/assets/logo/ の 2 ファイルは、
// その写し(logoAssets.test.ts が、内容の一致を確かめる)。path の値を、この .ts に書き写さないために、SVG から読む。
export interface LogoMark {
  path: string;
  viewBox: string;
  strokeWidth: number;
}

function readMark(svg: string): LogoMark {
  const path = /<path d="([^"]+)"/.exec(svg)?.[1];
  const viewBox = /viewBox="([^"]+)"/.exec(svg)?.[1];
  const strokeWidth = /stroke-width="([^"]+)"/.exec(svg)?.[1];
  if (!path || !viewBox || !strokeWidth) throw new Error("ロゴの SVG から、path・viewBox・線の太さを読めない");
  return { path, viewBox, strokeWidth: Number(strokeWidth) };
}

// 通常版(レタスの波つき)。
export const MARK = readMark(markSvg);

// 小さい版(波を省いた形。小さい大きさで、波が点の並びになるのを避ける)。
export const MARK_SMALL = readMark(markSmallSvg);
