// backend-go/internal/domain/password.go の ValidatePassword と同じ規則の定義。
// 変更するときは両方を直す(drift 注意)。
// メッセージはサーバーの 422 と同じ英語文言にして、クライアントとサーバーで別の文言を見せない。

/** パスワードの最小バイト数(UTF-8)。 */
export const PASSWORD_MIN_BYTES = 8;
/** パスワードの最大バイト数(UTF-8)。bcrypt の入力上限に合わせる。 */
export const PASSWORD_MAX_BYTES = 72;

// 記号は ASCII の英数字以外の印字可能文字。空白(0x20)・制御文字・非 ASCII は数えない。
const LETTER = /[A-Za-z]/;
const DIGIT = /[0-9]/;
const SYMBOL = /[\x21-\x2F\x3A-\x40\x5B-\x60\x7B-\x7E]/;

/**
 * validatePassword はパスワード規則の違反メッセージを返す。有効なら空配列。
 * 長さは UTF-16 コード単位ではなく UTF-8 のバイト数で数える。
 * 空文字列は blank のみを返し、それ以外は該当する違反を「短い → 長い → 文字種」の順にすべて返す。
 */
export function validatePassword(password: string): string[] {
  if (password === "") return ["Password can't be blank"];
  const messages: string[] = [];
  const bytes = new TextEncoder().encode(password).length;
  if (bytes < PASSWORD_MIN_BYTES) {
    messages.push(`Password is too short (minimum is ${PASSWORD_MIN_BYTES} characters)`);
  }
  if (bytes > PASSWORD_MAX_BYTES) {
    messages.push(`Password is too long (maximum is ${PASSWORD_MAX_BYTES} characters)`);
  }
  if (!(LETTER.test(password) && DIGIT.test(password) && SYMBOL.test(password))) {
    messages.push("Password must include letters, numbers and symbols");
  }
  return messages;
}
