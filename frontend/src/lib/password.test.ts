import { describe, it, expect } from "vitest";
import { validatePassword } from "./password";

const SHORT = "Password is too short (minimum is 8 characters)";
const LONG = "Password is too long (maximum is 72 characters)";
const KINDS = "Password must include letters, numbers and symbols";
const BLANK = "Password can't be blank";

// backend-go/internal/domain/password_test.go の主な事例。サーバーと結果が食い違わないことを固定する
const cases: [string, string, string[]][] = [
  ["有効: 英字・数字・記号を含む 9 バイト", "Passw0rd!", []],
  ["文字種: 英字だけ", "abcdefgh", [KINDS]],
  ["文字種: 数字だけ", "12345678", [KINDS]],
  ["文字種: 記号だけ", "!@#$%^&*", [KINDS]],
  ["文字種: 記号なし", "abcd1234", [KINDS]],
  ["文字種: 数字なし", "abcd!@#$", [KINDS]],
  ["文字種: 英字なし", "1234!@#$", [KINDS]],
  ["長さ: 7 バイトは too short のみ", "Abcde1!", [SHORT]],
  ["長さ: 8 バイトは有効", "Abcdef1!", []],
  ["長さ: 72 バイトは有効", "Aa1!" + "a".repeat(68), []],
  ["長さ: 73 バイトは too long のみ", "Aa1!" + "a".repeat(69), [LONG]],
  ["空文字列は blank だけを返す", "", [BLANK]],
  ["複数違反: 短く記号なしは too short と文字種の順", "abc123", [SHORT, KINDS]],
  ["複数違反: 73 バイトで英字だけは too long と文字種の順", "a".repeat(73), [LONG, KINDS]],
  ["空白は記号に数えない", "Passw0rd ", [KINDS]],
  ["空白を含んでいても他が満たされていれば有効", "Pass w0rd!", []],
  ["改行は記号に数えない", "Passw0rd\n", [KINDS]],
  ["空白だけ 8 個は blank ではなく文字種エラー", "        ", [KINDS]],
  ["非 ASCII: 日本語と数字と記号だけは文字種エラー", "あいう123!!", [KINDS]],
  ["非 ASCII: 全角の英数記号は種別に数えない", "Ａｂｃ１２３！！", [KINDS]],
  ["非 ASCII: 半角の英字・数字・記号が 1 つずつあれば日本語が混ざっても有効", "あいう1a!", []],
  ["バイト長: 3 文字でも 9 バイトあれば too short にならない", "ああa1!", []],
  ["バイト長: 72 バイト(23 文字 + 3 文字)は有効", "あ".repeat(23) + "a1!", []],
  ["バイト長: 73 バイトは too long のみ", "あ".repeat(23) + "a1!!", [LONG]],
];

describe("validatePassword", () => {
  it.each(cases)("%s", (_name, password, want) => {
    expect(validatePassword(password)).toEqual(want);
  });

  it("絵文字は UTF-16 の長さではなく UTF-8 のバイト数(4 バイト)で数える", () => {
    // "😀😀a1!" は UTF-16 では 7 単位(too short になる数え方)だが、UTF-8 では 11 バイトなので有効
    expect(validatePassword("😀a1!")).toEqual([SHORT]);
    expect(validatePassword("😀😀a1!")).toEqual([]);
  });
});

// ASCII の境界の文字が、英字・数字・記号のどれを供給するか(記号の範囲の端を固定する)
describe("validatePassword の文字種の境界", () => {
  const boundary: [string, string, "letter" | "digit" | "symbol" | "none"][] = [
    ["0x1F 制御文字", "\x1f", "none"],
    ["0x20 空白", " ", "none"],
    ["0x21 '!'", "!", "symbol"],
    ["0x2F '/'", "/", "symbol"],
    ["0x30 '0'", "0", "digit"],
    ["0x39 '9'", "9", "digit"],
    ["0x3A ':'", ":", "symbol"],
    ["0x40 '@'", "@", "symbol"],
    ["0x41 'A'", "A", "letter"],
    ["0x5A 'Z'", "Z", "letter"],
    ["0x5B '['", "[", "symbol"],
    ["0x60 '`'", "`", "symbol"],
    ["0x61 'a'", "a", "letter"],
    ["0x7A 'z'", "z", "letter"],
    ["0x7B '{'", "{", "symbol"],
    ["0x7E '~'", "~", "symbol"],
    ["0x7F DEL", "\x7f", "none"],
    ["全角英字", "Ａ", "none"],
    ["全角数字", "１", "none"],
    ["全角記号", "！", "none"],
    ["日本語", "あ", "none"],
  ];
  // 3 種のうち 1 種だけを欠いた 8 バイト以上の文字列に、その文字を足す。欠けた種別を供給するときだけ有効になる
  const lacking = { letter: "1234!!!!", digit: "abcd!!!!", symbol: "abcd1234" } as const;

  it.each(boundary)("%s", (_name, char, supplies) => {
    for (const kind of ["letter", "digit", "symbol"] as const) {
      const want = supplies === kind ? [] : [KINDS];
      expect(validatePassword(lacking[kind] + char)).toEqual(want);
    }
  });
});
