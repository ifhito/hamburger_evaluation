package domain_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// API の外部契約なので、メッセージは定数から組み立てず一字一句を固定する。
const (
	msgBlank = "Password can't be blank"
	msgShort = "Password is too short (minimum is 8 characters)"
	msgLong  = "Password is too long (maximum is 72 characters)"
	msgKinds = "Password must include letters, numbers and symbols"
)

// TestValidatePassword は、パスワード強度ルールの唯一の置き場として、長さ・文字種・
// 空・複数違反・非 ASCII・バイト長の境界を固定する。want は期待メッセージの完全一致
// （順序込み）で、有効なら空（nil）とする。
func TestValidatePassword(t *testing.T) {
	// マルチバイト文字は複数バイトになる。長さはバイト数で数える（bcrypt の入力上限に合わせる）。
	// 「あ」は UTF-8 で 3 バイトなので、23 個で 69 バイト、24 個で 72 バイトになる。
	a23 := strings.Repeat("あ", 23)
	a24 := strings.Repeat("あ", 24)
	// 英字・数字・記号を含む 8 バイトの先頭に英字を足して 72 バイト・73 バイトにする。
	pw72 := "Abcdef1!" + strings.Repeat("a", 64)
	pw73 := pw72 + "a"

	tests := []struct {
		name     string
		password string
		// bytes は入力のバイト数の期待値。0 のときは確認しない（テストデータの取り違え防止）。
		bytes int
		want  []string
	}{
		// 1. 有効
		{name: "有効: 英字・数字・記号を含む 9 バイト", password: "Passw0rd!", bytes: 9, want: nil},
		{name: "有効: ちょうど 8 バイト", password: "Abcdef1!", bytes: 8, want: nil},
		{name: "有効: ちょうど 72 バイト", password: pw72, bytes: 72, want: nil},

		// 2. 文字種が足りない 6 パターン（長さは満たすので文字種メッセージだけ）
		{name: "文字種: 英字だけ", password: "abcdefgh", bytes: 8, want: []string{msgKinds}},
		{name: "文字種: 数字だけ", password: "12345678", bytes: 8, want: []string{msgKinds}},
		{name: "文字種: 記号だけ", password: "!@#$%^&*", bytes: 8, want: []string{msgKinds}},
		{name: "文字種: 英字と数字のみ（記号なし）", password: "abcd1234", bytes: 8, want: []string{msgKinds}},
		{name: "文字種: 英字と記号のみ（数字なし）", password: "abcd!@#$", bytes: 8, want: []string{msgKinds}},
		{name: "文字種: 数字と記号のみ（英字なし）", password: "1234!@#$", bytes: 8, want: []string{msgKinds}},

		// 3. 長さの境界（文字種は満たす）
		{name: "長さ: 7 バイトは too short のみ", password: "Abcde1!", bytes: 7, want: []string{msgShort}},
		{name: "長さ: 8 バイトは有効", password: "Abcdef1!", bytes: 8, want: nil},
		{name: "長さ: 72 バイトは有効", password: pw72, bytes: 72, want: nil},
		{name: "長さ: 73 バイトは too long のみ", password: pw73, bytes: 73, want: []string{msgLong}},

		// 4. 空
		{name: "空文字列は blank だけを返す（too short や文字種を重ねない）", password: "", bytes: 0, want: []string{msgBlank}},

		// 5. 複数違反（順序は常に 短い → 長い → 文字種）
		{name: "複数違反: 短く記号なしは too short と文字種の順", password: "abc123", bytes: 6, want: []string{msgShort, msgKinds}},
		{name: "複数違反: 全部欠落（1 文字）は too short と文字種の順", password: "a", bytes: 1, want: []string{msgShort, msgKinds}},
		{name: "複数違反: 73 バイトで英字だけは too long と文字種の順", password: strings.Repeat("a", 73), bytes: 73, want: []string{msgLong, msgKinds}},

		// 6. 文字クラスの境界のうち、個別に例示された入力（範囲の端は TestValidatePasswordCharKindBoundaries）
		{name: "空白は記号に数えない: 末尾が空白なら文字種エラー", password: "Passw0rd ", bytes: 9, want: []string{msgKinds}},
		{name: "空白を含んでいても他が満たされていれば有効", password: "Pass w0rd!", bytes: 10, want: nil},
		{name: "改行は記号に数えない", password: "Passw0rd\n", bytes: 9, want: []string{msgKinds}},
		{name: "改行を含んでいても記号があれば有効", password: "Passw0rd!\n", bytes: 10, want: nil},
		{name: "タブは記号に数えない", password: "Passw0rd\t", bytes: 9, want: []string{msgKinds}},
		{name: "DEL(0x7F) は記号に数えない", password: "Passw0rd\x7f", bytes: 9, want: []string{msgKinds}},
		{name: "NUL は記号に数えない", password: "Passw0rd\x00", bytes: 9, want: []string{msgKinds}},

		// 7. 非 ASCII
		{name: "非 ASCII: 日本語と数字と記号だけ（半角英字なし）は文字種エラー", password: "あいう123!!", want: []string{msgKinds}},
		{name: "非 ASCII: 全角の英数記号は英字にも数字にも記号にも数えない", password: "Ａｂｃ１２３！！", want: []string{msgKinds}},
		{name: "非 ASCII: 半角の英字・数字・記号が 1 つずつあれば日本語が混ざっても有効", password: "あいう1a!", bytes: 12, want: nil},
		{name: "非 ASCII: 半角の英字・数字・記号が 1 つずつあれば全角が混ざっても有効", password: "Ａ1a!ｂｃ", want: nil},

		// 8. バイト長と文字数の混同を防ぐ
		{name: "バイト長: 4 文字でも 6 バイトなら too short", password: "あ1a!", bytes: 6, want: []string{msgShort}},
		{name: "バイト長: 72 バイト（23 文字 + 3 文字）は有効", password: a23 + "a1!", bytes: 72, want: nil},
		{name: "バイト長: 73 バイト（23 文字 + 4 文字）は too long のみ", password: a23 + "a1!!", bytes: 73, want: []string{msgLong}},
		{name: "バイト長: 72 バイトでも英数記号がなければ文字種エラーのみ", password: a24, bytes: 72, want: []string{msgKinds}},

		// 9. 空白だけ（空ではないので通常判定に進む）
		{name: "空白だけ: 8 個の半角スペースは blank ではなく文字種エラー", password: "        ", bytes: 8, want: []string{msgKinds}},
		{name: "空白だけ: 1 個の半角スペースは too short と文字種の順", password: " ", bytes: 1, want: []string{msgShort, msgKinds}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.bytes != 0 && len(tt.password) != tt.bytes {
				t.Fatalf("テストデータの誤り: len(%q) = %d, want %d バイト", tt.password, len(tt.password), tt.bytes)
			}
			// 決定性: 同じ入力は何度呼んでも同じ結果になる。
			for i := 0; i < 3; i++ {
				got := domain.ValidatePassword(tt.password)
				if !slices.Equal(got, tt.want) {
					t.Errorf("%d 回目: ValidatePassword(%q) = %q, want %q", i+1, tt.password, got, tt.want)
				}
			}
		})
	}
}

// TestValidatePasswordCharKindBoundaries は、英字・数字・記号の各範囲の端とその隣の文字を、
// 「その 1 文字だけがその種別を供給する」形で検証し、off-by-one を検出する。
// 種別ごとに、他の 2 種を満たす 8 バイトの土台へ 1 文字足す。足した文字がその種別なら有効、
// そうでなければ文字種エラーだけになる。
func TestValidatePasswordCharKindBoundaries(t *testing.T) {
	// 各土台は 8 バイトで、対象の種別だけを欠いている。
	const (
		withoutLetter = "1234567!"
		withoutDigit  = "abcdefg!"
		withoutSymbol = "Abcdefg1"
	)

	tests := []struct {
		name                        string
		char                        string
		isLetter, isDigit, isSymbol bool
	}{
		{name: "0x1F(制御文字)", char: "\x1f"},
		{name: "0x20(空白)", char: " "},
		{name: "0x21 '!'（記号の下端）", char: "!", isSymbol: true},
		{name: "0x2F '/'（記号。数字 '0' の 1 つ下）", char: "/", isSymbol: true},
		{name: "0x30 '0'（数字の下端）", char: "0", isDigit: true},
		{name: "0x39 '9'（数字の上端）", char: "9", isDigit: true},
		{name: "0x3A ':'（記号。数字 '9' の 1 つ上）", char: ":", isSymbol: true},
		{name: "0x40 '@'（記号。英字 'A' の 1 つ下）", char: "@", isSymbol: true},
		{name: "0x41 'A'（大文字英字の下端）", char: "A", isLetter: true},
		{name: "0x5A 'Z'（大文字英字の上端）", char: "Z", isLetter: true},
		{name: "0x5B '['（記号。英字 'Z' の 1 つ上）", char: "[", isSymbol: true},
		{name: "0x60 '`'（記号。英字 'a' の 1 つ下）", char: "`", isSymbol: true},
		{name: "0x61 'a'（小文字英字の下端）", char: "a", isLetter: true},
		{name: "0x7A 'z'（小文字英字の上端）", char: "z", isLetter: true},
		{name: "0x7B '{'（記号。英字 'z' の 1 つ上）", char: "{", isSymbol: true},
		{name: "0x7E '~'（記号の上端）", char: "~", isSymbol: true},
		{name: "0x7F DEL", char: "\x7f"},
		{name: "0x00 NUL", char: "\x00"},
		{name: "改行", char: "\n"},
		{name: "タブ", char: "\t"},
		{name: "0x80（非 ASCII の先頭バイト）", char: "\x80"},
		{name: "全角英字 'Ａ'", char: "Ａ"},
		{name: "全角数字 '１'", char: "１"},
		{name: "全角記号 '！'", char: "！"},
		{name: "日本語 'あ'", char: "あ"},
	}

	// 土台に 1 文字足した入力が、その種別を供給するなら有効、しなければ文字種エラーだけになる。
	wantFor := func(supplies bool) []string {
		if supplies {
			return nil
		}
		return []string{msgKinds}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := []struct {
				kind     string
				password string
				want     []string
			}{
				{kind: "英字", password: withoutLetter + tt.char, want: wantFor(tt.isLetter)},
				{kind: "数字", password: withoutDigit + tt.char, want: wantFor(tt.isDigit)},
				{kind: "記号", password: withoutSymbol + tt.char, want: wantFor(tt.isSymbol)},
			}
			for _, c := range checks {
				if got := domain.ValidatePassword(c.password); !slices.Equal(got, c.want) {
					t.Errorf("%s を供給する検証: ValidatePassword(%q) = %q, want %q", c.kind, c.password, got, c.want)
				}
			}
		})
	}
}
