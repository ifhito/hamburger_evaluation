// ログインが必要な画面へ未ログインで来たとき、ログインのあとに、元の画面へ戻すための、戻り先の取り扱い。
// 戻り先は、ルーターの遷移の state(このアプリの中の操作でしか付かない)に入れ、URL のパラメーターには
// 入れない。URL に入れると、外部のリンクから任意の戻り先を指定できてしまう(オープンリダイレクト)ため。
export interface ReturnToState {
  from?: string;
}

// state から、このアプリの中の戻り先(/ で始まり、// や \ で始まらない)だけを取り出す。
// それ以外は null(呼び出し側の既定の遷移先を使う)。
export function returnPathFrom(state: unknown): string | null {
  if (typeof state !== "object" || state === null) return null;
  const from = (state as ReturnToState).from;
  if (typeof from !== "string") return null;
  if (!from.startsWith("/") || from.startsWith("//") || from.startsWith("/\\")) return null;
  return from;
}
