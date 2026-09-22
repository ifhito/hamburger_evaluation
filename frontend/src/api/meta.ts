import useSWRImmutable from "swr/immutable";
import { buildApiClient } from "./client/buildApiClient";

// backend が返す、描画に使う domain のルールの値(GET /meta)。ルールを持つのは backend だけで、
// frontend は値を複製せず、これを表示・選択肢の生成に使う。
export interface Meta {
  rating: { min: number; max: number };
  // 写真の上限。maxEdge は長辺(ピクセル)、maxBytes はファイルの大きさ(バイト)。
  photo: { maxEdge: number; maxBytes: number };
  // 入力欄ごとの文字数の上限(コードポイント数)。文字数のカウンターの表示にだけ使い、超えたかどうかの判定は
  // backend の 422 に任せる。
  text: {
    reviewCommentMaxChars: number;
    burgerNameMaxChars: number;
    shopNameMaxChars: number;
    usernameMaxChars: number;
    bioMaxChars: number;
    moderationNoteMaxChars: number;
  };
  // パスワードの長さの範囲。文字数ではなくバイト数(日本語の 1 文字は 3 バイト)。説明文の表示にだけ使う。
  password: { minBytes: number; maxBytes: number };
  // パスワードのほかに使えるサインイン方法の名前(例: ["google"])。規則ではなく、backend の設定で決まる。
  // サインインの画面に、これに含まれる方法のボタンだけを出す(なければ空の配列)。
  loginProviders: string[];
}

const metaApiClient = buildApiClient();

// 値はデプロイでしか変わらないので、取得は 1 回だけで、画面をまたいで共有する。
export function useMeta() {
  return useSWRImmutable<Meta>("/meta", async () => {
    const res = await metaApiClient.get<Meta>("/meta");
    return res.data;
  });
}
