# Burger Stack 現行画面＋追加機能

編集用ファイル: `../files/burgerstack-refresh-2026-09.penpot`

現行のReact画面を実測し、PC（1280px）・モバイル（375px）の2ページ、計20ボードに取り込んでいます。評価の波線・点線、サービスロゴ、カード、余白、ヘッダーは実装を基準にしています。

対象はショップ・バーガー・レビューの一覧と詳細、ユーザー詳細、レビューなしのユーザー詳細、本人プロフィール（接続済みアプリを含む）、Burger Stackについてです。

## 参照した実装

`feat/issue229-search-sort` の `4906e259` に `feat/issue229-content` の `01b2b83c` を統合したローカル作業用ツリーを参照しました。PR #233・#234・#235 の追加機能（紹介ページとヘッダーリンク、食べた日・店舗名・画像、検索・並び替えなど）を含みます。本番公開済み画面と追加機能を含む画面は同一ではありません。

写真・店名・評価・件数・プロフィールはローカルAPIのサンプルデータです。写真の実際の店や商品との対応は示しません。サンプルAPIは検索・並び替えの動作検証用ではありません。

## 比較・編集

`index.html` で各画面の「Penpot」と「React実画面」を比較できます。

- `previews/<画面>-<pc|mobile>.png`: Penpot exporterの出力
- `previews/<画面>-source-<pc|mobile>.png`: 同じデータのReact実画面
- `captures/*.json`: DOMの位置・文字・色・SVGパス・写真寸法の読み取り結果
- `boards.json`: Penpotのファイル・ページ・ボードID

ローカルPenpotは http://localhost:9001 です。`.penpot` をインポートすると文字・カード・アイコンを編集できます。固定配置であり、クリックできるプロトタイプではありません。既存の `hamburger-evaluation-redesign.penpot` は変更していません。

フォントはPenpotのNoto Sans JPを使用しています。ブラウザのVariableフォントと描画方式が異なるため、文字の太さ・ベースラインには小さな差があります。ネイティブ入力欄の文字幅・選択矢印は近似です。モバイルで接続済みアプリのバッジが縦に折り返す状態など、参照実装の表示もそのまま保持しています。

## 再生成

1. `python3 design/scripts/serve_design_fixture.py` でサンプルAPIを起動します（127.0.0.1:18091のみ）。
2. 参照するfrontendを `VITE_API_PROXY_TARGET=http://127.0.0.1:18091 pnpm run dev --host 127.0.0.1 --port 5188` で起動します。
   node_modulesを別のworktreeからシンボリックリンクする場合は、Viteの `server.fs.allow` にその依存パッケージの実パスを追加し、フォントが403になっていないことを確認します。
3. ローカル画面で `fixture@example.test` / `fixture` を入力してサンプルとしてサインインします。実在のアカウントではありません。
4. ブラウザをPC 1280×900、モバイル375×812で開き、`extract_current_ui.js` を読み取り専用のevaluateで実行し、JSONとfull-page screenshotを保存します。画面: `/shops`, `/burgers`, `/reviews`, `/shops/s1`, `/burgers/b1`, `/reviews/r1`, `/users/u3`, `/users/u2`, `/users/u1`, `/about`。
5. 次のコマンドで計測JSONから新規Penpotファイルを作成します。既存ファイルは上書きしません。再実行ごとに新規ファイルが作られます。

```sh
python3 design/scripts/build_burgerstack_refresh.py \
  --email <ローカルPenpotのメール> \
  --password-file <ローカルの非公開パスワードファイル>
```

各ボードをPenpotからPNGで書き出し、`previews` に保存してください。`--only reviews-pc` は部分検証用であり、配布前は全20画面を再生成してください。秘密情報はこのディレクトリに保存しないでください。

## 検証

`python3 design/scripts/validate_burgerstack_refresh.py` で、Penpot ZIP、2ページ・20ボード、計測寸法、主要文言、評価パス、40枚のPNGのCRC・寸法、比較ギャラリーのリンクを確認します。

このPRはデザイン資料のみです。frontend/backendの実装変更は含みません。
