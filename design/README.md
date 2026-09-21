# design/ — デザインツール Penpot(ローカル)

[Penpot](https://penpot.app/) は、オープンソースのデザインツールです。ここには、Penpot を自分の PC で動かすための compose と、書き出したデザインのファイル(`.penpot`)を置きます。アプリ(`backend-go/`・`frontend/`)とは独立していて、アプリの起動・テストには影響しません。

- 開く URL: <http://localhost:9001>(ポートは `.env` で変えられます)
- ローカル専用です。**インターネットには公開しないでください**(HTTPS なし・メール確認なしの設定にしています)

## 前提

- Docker(Docker Desktop など)。空きメモリは約 1.5 GiB、ディスクは約 4 GiB(下の「動作の目安」)
- 使わないときは `down` して止めてください(数百 MiB〜1 GiB を使い続けるため)

## 初回セットアップ

```bash
# 1. 設定ファイルを作る(design/.env はコミットしない)
cp design/.env.example design/.env

# 2. Penpot の鍵を生成して design/.env に追記する(値を画面に出さない・読み返さない)
printf 'PENPOT_SECRET_KEY=%s\n' "$(openssl rand -base64 48 | tr -d '\n')" >> design/.env
```

`design/.env` は鍵を含む秘密のファイルです。`.gitignore` の対象で、エージェントも読めない設定(`.claude/settings.json` の deny)になっています。`design/.env.example` は、値の雛形(秘密なし)です。

## 起動・停止・初期化

以降のコマンドは、リポジトリのルートで実行します。`-p` はプロジェクト名です。

```bash
# 起動(初回はイメージの取得に数分かかります)
docker compose -p hamburger-penpot -f design/docker-compose.yml --env-file design/.env up -d

# 停止(データは残る)
docker compose -p hamburger-penpot -f design/docker-compose.yml --env-file design/.env down

# 初期化(データもすべて消える。ユーザー・ファイルが消えます)
docker compose -p hamburger-penpot -f design/docker-compose.yml --env-file design/.env down -v
```

## 最初のユーザーを作る

起動したら、ユーザーを 1 人作ります(対話式。メールアドレス・名前・パスワードを聞かれます)。

```bash
docker exec -ti hamburger-penpot-penpot-backend-1 python3 manage.py create-profile
```

作ったメールアドレスとパスワードで、<http://localhost:9001> にログインできます(メールの確認はありません)。
初回のログインでは、利用目的のアンケート(3 問。答えないと進めません)と、チームを作る画面が出ます。チームは「Continue without team」で飛ばせます。

## デザインの書き出しと保存(`.penpot`)

デザインの変更の履歴は、書き出した `.penpot` のファイルを、`design/files/` に置いて git で追います。

- **書き出し**: 開いているファイルの左上のメニュー(︙)→ **File** → **Download Penpot file (.penpot)**。ダウンロードしたファイルを `design/files/<画面や機能の名前>.penpot` に置きます
- **読み込み**: ダッシュボードで **Import a file**(Import a .penpot file)→ `design/files/` のファイルを選びます。初期化(`down -v`)したあとの、新しい Penpot に戻すときも、この手順です
- `.penpot` は zip のバイナリなので、**git の差分は読めません**(変更はファイル単位の更新として残ります)。変更のレビューが必要になったら、SVG の書き出しの併用などを検討します
- `design/files/sample-login.penpot` は、書き出しと読み込みの動作確認用のサンプル(ログイン画面の枠 1 つと、長方形 2 つ)です。デザインではありません

### (参考)ブラウザを使わずに書き出す・読み込む

スクリプトや自動確認では、Penpot の API を使えます(`http://localhost:9001` は自分のポートに読み替えます)。

```bash
# ログイン(cookie を保存)
curl -s -c cookies.txt -H 'Content-Type: application/json' \
  -X POST http://localhost:9001/api/rpc/command/login-with-password \
  -d '{"email":"<メール>","password":"<パスワード>"}' -o /dev/null

# 書き出し: 進捗のイベントの最後に、ダウンロード用の URL(assets/by-id/...)が返る
curl -s -b cookies.txt -H 'Content-Type: application/json' \
  -X POST http://localhost:9001/api/rpc/command/export-binfile \
  -d '{"fileId":"<ファイルの id>","version":3,"includeLibraries":false,"embedAssets":false}'
curl -s -b cookies.txt -L "<返ってきた URL>" -o design/files/<名前>.penpot

# 読み込み: project-id は、読み込み先のプロジェクト(Drafts など)の id
curl -s -b cookies.txt -X POST http://localhost:9001/api/rpc/command/import-binfile \
  -F name=<ファイル名> -F project-id=<プロジェクトの id> -F version=3 -F file=@design/files/<名前>.penpot
```

パスワードを、コマンドの引数(シェルの履歴に残る)に書く場合は、ローカルの検証用のアカウントだけにしてください。

## いまの画面のデザイン(`hamburger-evaluation.penpot`)

`design/files/hamburger-evaluation.penpot` は、**いまのアプリの画面を、そのまま Penpot に写したもの**です。これから画面を作り直す(リデザインする)ときの、「変える前の見本」と「デザインの部品・色の土台」になります。見た目は変えていません(変えるのは、この次の段階です)。

最初に出てくる言葉:

- **ボード(フレーム)**: 画面 1 枚分の枠です。Penpot の画面では「Board」と表示されます。この README では「画面の枠」と書きます
- **部品(コンポーネント)**: ボタンやカードなど、何度も使う見た目を 1 つにまとめたものです。元の部品(メインコンポーネント)を直すと、そこから作った複製に反映されます
- **スタイル**: 色や文字の大きさ・太さに、名前を付けて登録したものです。「primary(オレンジ)」のように名前で使えて、あとで色を変えると、そのスタイルを使っている所がまとめて変わります。この README の「トークン」は、これのことです

### 開いて見る

1. 「起動・停止・初期化」と「最初のユーザーを作る」で、Penpot を起動してログインする
2. ダッシュボードで **Import a file** → `design/files/hamburger-evaluation.penpot` を選ぶ(数秒で終わります)
3. 読み込んだ `hamburger-evaluation` を開く。左上の **PAGES** で、下の 4 つのページを切り替えて見る
4. 見終わったら、`down` で Penpot を止める(メモリを約 1.2〜1.3 GiB 使うため)

### ページの構成

| ページ | 中身 |
|---|---|
| `値` | 色の見本(13 色・CSS の変数名つき)と、文字のスタイルの見本、角丸(6px)。ここがトークンの一覧 |
| `部品` | 部品(コンポーネント)15 個。左の **ASSETS** タブからも使えます |
| `画面(PC)` | 幅 1280px の画面 14 枚 |
| `画面(モバイル)` | 幅 375px の画面 14 枚(同じ 14 画面) |

画面の高さは、実際の画面の高さ(内容の長さ)に合わせています。

### 画面の一覧と、Penpot の枠の名前

画面の枠の名前は、`<画面の名前> / PC` と `<画面の名前> / モバイル` です(下の表は PC。モバイルは末尾を `/ モバイル` にしたもの)。並べて比べた画像は `design/screenshots/<キー>-pc.jpg` と `<キー>-mobile.jpg` です(左が実際の画面、右が Penpot)。

| 画面の名前(枠の名前の前半) | 実際の URL | 見え方 | 比べた画像のキー |
|---|---|---|---|
| サインイン | `/signin` | 未ログイン | `signin` |
| サインイン(エラー表示) | `/signin` | 間違ったパスワードで送信した後 | `signin-error` |
| 新規登録 | `/signup` | 未ログイン | `signup` |
| メールを確認してください | `/signup` | 登録を送信した後(Check your email) | `signup-sent` |
| ショップの一覧 | `/shops` | 一般ユーザー | `shops` |
| ショップの詳細 | `/shops/:id` | 一般ユーザー | `shop-detail` |
| レビューの一覧 | `/reviews` | 一般ユーザー | `reviews` |
| レビューの詳細 | `/reviews/:id` | 一般ユーザー(自分のレビュー) | `review-detail` |
| レビューの投稿 | `/reviews/new?shop_id=:id` | 一般ユーザー | `review-new` |
| レビューの編集 | `/reviews/:id/edit` | 一般ユーザー(自分のレビュー) | `review-edit` |
| プロフィール | `/users/:id` | 一般ユーザー | `profile` |
| プロフィールの編集 | `/users/:id/edit` | 一般ユーザー | `profile-edit` |
| 管理(ショップの承認・却下) | `/admin/shops` | 管理者 | `admin-shops` |
| 管理(ショップの編集) | `/admin/shops/:id/edit` | 管理者 | `admin-shop-edit` |

画面に入っている文字(ショップ名・レビューの文など)は、開発用のサンプルデータ(`backend-go/cmd/seed`)の内容です。

### 部品の一覧

`部品` ページの 15 個です。元にした画面の見た目を、そのまま部品にしています(名前の `/` は、Penpot の左のリストでグループになります)。

| 部品 | 元にした画面 | 大きさ(px) |
|---|---|---|
| Header/PC | サインイン(PC) | 1280 × 56 |
| Header/モバイル | サインイン(モバイル) | 375 × 56 |
| Button/Primary(オレンジのボタン) | サインイン(PC) | 400 × 32 |
| Button/Secondary(灰色のボタン) | メールを確認してください(PC) | 400 × 32 |
| Button/Danger(赤のボタン) | 管理(ショップの承認・却下)(PC) | 72 × 32 |
| Input(1 行の入力欄) | サインイン(PC) | 400 × 34 |
| Textarea(複数行の入力欄) | レビューの投稿(PC) | 500 × 100 |
| Select(選ぶ欄) | レビューの投稿(PC) | 500 × 36 |
| ErrorMessage | サインイン(エラー表示)(PC) | 400 × 40 |
| Card/Review(レビューの一覧の中) | レビューの一覧(PC) | 752 × 153 |
| Card/Review(ショップの詳細の中) | ショップの詳細(PC) | 752 × 111 |
| Card/Review(プロフィールの中) | プロフィール(PC) | 752 × 124 |
| Card/Shop | ショップの一覧(PC) | 752 × 52 |
| Row/管理のショップ | 管理(ショップの承認・却下)(PC) | 752 × 122 |
| Badge(Active・Pending の印) | 管理(ショップの承認・却下)(PC) | 49 × 26 |

### 色・文字のトークンの対応表

元は `frontend/src/app/styles/globals.css` の CSS の変数です。Penpot に、次の名前で登録しています(`値` ページに見本があります)。

**色のスタイル**(Penpot の **ASSETS** → Colors、グループ名は「色」)

| CSS の変数 | Penpot の名前 | 値 | 使う所 |
|---|---|---|---|
| `--color-primary` | `primary` | `#e05c00` | ヘッダー・主なボタン・リンク |
| `--color-primary-hover` | `primary-hover` | `#c75400` | 主なボタンにマウスを乗せたとき |
| `--color-danger` | `danger` | `#dc2626` | 却下・削除のボタン |
| `--color-danger-hover` | `danger-hover` | `#b91c1c` | 赤いボタンにマウスを乗せたとき |
| `--color-secondary-bg` | `secondary-bg` | `#f3f4f6` | 灰色のボタン・絞り込みの背景 |
| `--color-secondary-hover` | `secondary-hover` | `#e5e7eb` | 灰色のボタンにマウスを乗せたとき |
| `--color-border` | `border` | `#d1d5db` | 枠線・区切り線 |
| `--color-text` | `text` | `#111827` | 本文の文字 |
| `--color-text-muted` | `text-muted` | `#6b7280` | 補足の文字(日付・作成者など) |
| `--color-error` | `error` | `#dc2626` | エラーの文字 |
| `--color-success` | `success` | `#16a34a` | Active の印 |
| (`body` の `background`) | `background` | `#fafafa` | ページの背景 |
| (変数なし) | `white` | `#ffffff` | ヘッダーの文字・入力欄やカードの背景 |

- `danger` と `error` は同じ値です。画面の図形は、同じ値なら先に登録した `danger` にひも付けています
- 変数にない色(バッジの文字 `#166534`・`#9a3412`、エラー表示の背景 `#fee2e2` と枠 `#fca5a5`、入力欄の案内の文字 `#757575` など)は、CSS の中に直接書かれている色です。スタイルにはせず、色コードのままにしています
- マウスを乗せたときの色(`*-hover`)は登録していますが、画面としては描いていません

**文字のスタイル**(Penpot の **ASSETS** → Typographies、グループ名は「文字」)

名前は `<大きさ>/<太さ>`(標準 = 400、太字 = 700)です。実際の画面で使われている組み合わせを、すべて登録しています。

| 名前 | 使う所 |
|---|---|
| `24px/太字` | 画面の見出し(h1)・プロフィールのユーザー名 |
| `24px/標準` | (星の評価の大きい表示) |
| `17.6px/太字` | ヘッダーのロゴ・レビュー一覧の小見出し |
| `17.6px/標準` | 星の評価 |
| `16px/太字` | 強調・「Danger Zone」の見出し |
| `16px/標準` | ヘッダーのリンク・レビューの本文 |
| `15.2px/太字`・`15.2px/標準` | ショップ名の表示 |
| `14px/標準` | ボタン・入力欄・ラベル |
| `13.6px/標準` | 補足の文字(Created by など) |
| `12px/標準` | 日付・バッジ・入力のヒント |

- 角丸(`--radius`): 6px(図形の角丸には、実際の画面の値をそのまま入れています)
- **余白と文字の大きさには、`globals.css` に変数がありません。** 文字の大きさは、実際の画面で使われている組み合わせを、上の表のスタイルにしました。余白は、変数がないので、スタイルとしては登録していません(各図形の位置と大きさに、実際の画面の値が入っています)。余白のルールを決めるのは、リデザインの段階です
- フォント(`--font-sans`): `system-ui, -apple-system, sans-serif`。Penpot にこの名前のフォントはないので、**Penpot に最初から入っている `sourcesanspro` で代用**しています(下の「できていないこと・注意」)

### 作り方と、その理由

**方法: Penpot の API(JSON)で作りました。** 実際の画面(frontend)をヘッドレスのブラウザ(Chromium)で開き、見えている要素の位置・大きさ・色・文字を一覧(JSON)にして、`update-file`(Penpot が図形を追加・変更する API)で、長方形・グループ・**文字(編集できるテキスト)**として登録しています。

| 方法 | 使ったか | 理由 |
|---|---|---|
| Penpot の API(`update-file`)で、DOM から取った位置・色・文字を登録する | **使った** | 位置・色・文字を正確に写せる。文字が文字のまま残る(編集できる)。何度でも同じ結果で作り直せる |
| SVG の読み込み(画面を SVG にして取り込む) | 使わなかった | Penpot の SVG の読み込みは、ブラウザの中の処理で、API から呼べない。取り込むと、文字が図形(線)になり、編集できなくなることが多い |
| ブラウザで Penpot の画面を操作して作る(キャンバスの自動操作) | 使わなかった | 座標の操作は壊れやすく、時間もかかる。API のほうが確実 |

画面が変わったら、次の「いまの画面から作り直す」で、同じ手順を繰り返せます。

### いまの画面から作り直す

アプリの画面を変えたあとに、`hamburger-evaluation.penpot` を作り直す手順です。すべて、リポジトリのルートで実行します。ブラウザは Docker の中の Chromium を使います(自分の PC に入れる必要はありません)。

```bash
# 0. 作業用のディレクトリに、スクリプトと、Playwright(ブラウザ自動操作)を用意する
WORK=/tmp/penpot-work && mkdir -p $WORK && cp design/scripts/*.js $WORK/
PW_IMAGE=mcr.microsoft.com/playwright:v1.50.1-jammy
docker run --rm -v $WORK:/work -w /work $PW_IMAGE npm i --no-save playwright@1.50.1

# 1. 実際の画面を取る。アプリ(API と frontend)を、開発用のサンプルデータ入りで起動しておく(README のルートの手順)。
#    frontend のコンテナ名は docker ps で確認する。コンテナの「localhost」を frontend にするので、Vite に断られません
docker run --rm --network container:<frontend のコンテナ名> -v $WORK:/work -w /work \
  -e BASE=http://localhost:5173 -e OUT=/work/out $PW_IMAGE node capture.js

# 2. Penpot を起動して、ユーザーを作る(「起動・停止・初期化」と「最初のユーザーを作る」)。
#    作ったパスワードを、$WORK/penpot-pw.txt に 1 行で書いておく(コマンドの履歴に残さないため。コミットしない)

# 3. Penpot に、デザインを作る(数秒。画面 28 枚・部品 15・色 13・文字 11)
python3 design/scripts/build_penpot.py --base http://localhost:9001 --email <メール> \
  --password-file $WORK/penpot-pw.txt --captures $WORK/out --map-out $WORK/map.json

# 4. 書き出す(「(参考)ブラウザを使わずに書き出す・読み込む」の export-binfile。ファイルの id は map.json の fileId)
#    → design/files/hamburger-evaluation.penpot に置く
```

- **書き出しは、作ったすぐ後に行ってください。** Penpot は、ファイルを画面(エディター・ビューアー)で開くと、縮小画像を自動で作り、書き出しに含めます(ファイルが約 1.3 MB → 約 2.1 MB になります)。中身は変わりませんが、差分が大きくなります
- 画面を足すときは、`capture.js` の `routes` と、`build_penpot.py` の `SCREEN_ORDER` に足します。部品を足すときは、`build_penpot.py` の `COMPONENTS` に足します
- 作るたびに、Penpot 上には新しいファイルができます(前のファイルは、ダッシュボードで消します)

### 見た目の確認(実際の画面と並べて比べる)

```bash
# Penpot の画面(閲覧用のビューアー)を、画像にする。map.json は、手順 3 の --map-out で作ったもの
docker run --rm --network hamburger-penpot_penpot -v $WORK:/work -w /work \
  -e PENPOT_URL=http://localhost:9001 -e PENPOT_EMAIL=<メール> -e PENPOT_PASSWORD_FILE=/work/penpot-pw.txt \
  -e MAP_JSON=/work/map.json -e OUT=/work/render $PW_IMAGE \
  sh -c 'node forward.js & sleep 1; node render_penpot.js'

# 実際の画面(左)と Penpot(右)を並べた画像にする
docker run --rm -v $WORK:/work -w /work -e MAP_JSON=/work/map.json -e OUT=/work/compare -e FORMAT=jpeg $PW_IMAGE node compare.js
```

Penpot の画面は、`PENPOT_PUBLIC_URI`(この compose では `http://localhost:9001`)と同じ URL で開かないと、「404 This page doesn't exist」になります。Docker の中から開く手順では、上のように `forward.js`(`localhost:9001` を Penpot の frontend につなぐ短い中継)を使っています。`--network` の名前は、`docker network ls` で確認できます(プロジェクト名 + `_penpot`)。

### できていないこと・注意

- **文字の見た目は、完全には同じではありません。** 実際の画面のフォントは、OS の標準のフォント(`system-ui`)ですが、Penpot に入れられるフォントは `sourcesanspro`(英数字)などです。文字の幅や字形が少し違い、Penpot では文字の位置が数 px ずれて見える所があります(ボタンの中の文字が、少し上に見える、など)。Penpot は、実際の画面で測った文字の幅に合わせて文字を描くため、実際より詰まって(または広がって)見える所もあります
- **並べた画像の「実際の画面」は、太字が細く写っています。** 画像を撮った Docker の Chromium は、太字のないフォント(WenQuanYi Zen Hei)を標準のフォントとして使うためです。デザインの値は CSS の指定(太字は 700)から取っているので、Penpot 側の太字は、CSS どおりです。また、文字の幅・折り返しの位置も、このフォントで測ったものです。自分の PC のブラウザで見ると、文字の幅が少し違う所があります(ボタンやバッジのように、文字の長さで幅が決まる所)。正確に合わせたいときは、`capture.js` を、自分の PC のブラウザ(Playwright)で実行してください
- **画面の中の図形は、部品の複製(インスタンス)ではありません。** 部品は `部品` ページにメインコンポーネントとして作ってありますが、`画面(PC)` などの中のボタンやカードは、同じ見た目の**普通の図形**です(ボタン・入力欄は、`Button/Primary` のような名前のグループにまとめてあります)。部品を直しても、画面には反映されません。リデザインで部品を使いながら画面を作り直すときに、部品から置き直してください
- **色・文字は「スタイル」として登録していて、Penpot の新しい機能「Tokens」(左の **TOKENS** タブ)には入れていません。** TOKENS タブは空です。スタイルは **ASSETS** タブにあります。Tokens への移行は、リデザインの段階で、必要かどうかを決めます
- **入力欄の右端の「▼」など、ブラウザが描く部品の見た目**(選ぶ欄の矢印など)は、写していません
- **状態は、上の一覧の 1 つずつだけです。** マウスを乗せた・入力中・読み込み中・エラー(サインイン以外)などの状態は、画面として作っていません
- **配置は、位置の固定です。** Penpot の自動配置(Auto layout)や、大きさを変えたときの追従(制約)は付けていないので、画面の幅を変えても中身は追従しません。文字を長くしても、周りは動きません
- 絵文字(ヘッダーのロゴ 🍔)は、文字のまま入れています(Penpot の環境によって、見え方が変わることがあります)
- 部品・色を後から直した場合、画面の中の図形には反映されません(上の 1 つ前と同じ理由です)

### 動作の目安(このファイル。実測)

測定: Penpot 2.17.2、Docker Desktop の VM(4 CPU・7.7 GiB)、Apple Silicon(arm64)。

| 項目 | 値 |
|---|---|
| `hamburger-evaluation.penpot` の大きさ | 約 1.3 MB(1,295,392 バイト。中身は 1,230 個のファイル) |
| 読み込み(`import-binfile`)にかかる時間 | 1 秒以内 |
| 作る時間(`build_penpot.py`) | 約 2 秒 |
| 実際の画面を取る時間(`capture.js`。28 画面) | 約 48 秒 |
| Penpot の画面を画像にする時間(28 画面) | 約 135 秒 |
| Penpot のメモリ(このファイルを開いた状態の合計) | 約 1.2〜1.3 GiB(backend 約 0.8〜0.9 GiB) |
| 並べた画像(`design/screenshots/`) | 28 枚・合計 約 1.7 MB(JPEG) |

## リデザインのラフ(`hamburger-evaluation-redesign.penpot`)

`design/files/hamburger-evaluation-redesign.penpot` は、**新しい見た目の案(ラフ)**です。いまの画面(`hamburger-evaluation.penpot`)とは別のファイルで、こちらは上書きしていません。話し合いで決めた方向(**白い背景に黒い文字**・ハンバーガーの写真が主役・マスタードの黄色は塗りだけ・現代的なゴシック・日本語と英語の切り替え・PC とモバイルを同じくらい)と、「遊び」として選んだ 2 つ(**バーガーの段数で見せる評価**、**空・読み込み中・見つからないときの一言**)を、最初の 3 画面と状態の画面で確かめるためのものです。色や大きさの値は「案」で、見て調整します。(最初のラフは黒い背景でしたが、白い背景に変えました。)

### 開いて見る

「いまの画面のデザイン」の「開いて見る」と同じ手順です(**Import a file** → `design/files/hamburger-evaluation-redesign.penpot`)。**PAGES** は、次の 3 つです。

| ページ | 中身 |
|---|---|
| `値` | 色 11 色、文字と背景のコントラストの実測、評価のバーガー(0〜5)、文字のスタイル、キーボード操作の表示の見本 |
| `画面(PC)` | 幅 1280px の画面 6 枚 |
| `画面(モバイル)` | 幅 375px の画面 6 枚 |

画面の並べた画像は `design/screenshots/redesign/` にあります(Penpot の閲覧画面を画像にしたもの。`values.jpg` は `値` ページ)。

| 画面 | 枠の名前(PC / モバイル) | 画像 |
|---|---|---|
| ショップの一覧(日本語) | `ショップ一覧 / PC`、`ショップ一覧 / モバイル` | `shops-pc.jpg`、`shops-mobile.jpg` |
| レビューの一覧 | `レビュー一覧 / PC`、`レビュー一覧 / モバイル` | `reviews-pc.jpg`、`reviews-mobile.jpg` |
| レビューの詳細 | `レビュー詳細 / PC`、`レビュー詳細 / モバイル` | `review-detail-pc.jpg`、`review-detail-mobile.jpg` |
| ショップの一覧(英語) | `ショップ一覧(English) / PC`、`ショップ一覧(English) / モバイル` | `shops-en-pc.jpg`、`shops-en-mobile.jpg` |
| 空・読み込み中・見つからない(日本語) | `空・読み込み・404 / PC`、`空・読み込み・404 / モバイル` | `states-pc.jpg`、`states-mobile.jpg` |
| 空・読み込み中・見つからない(英語) | `空・読み込み・404(English) / PC`、`空・読み込み・404(English) / モバイル` | `states-en-pc.jpg`、`states-en-mobile.jpg` |

### 案の中身

- **ショップの一覧**: 写真をポスターのように大きく見せ、店名と評価(バーガー・数字・件数)は、写真の下に黒い文字で置きます。PC は 3 列、モバイルは 1 列です。審査待ちの店には、写真の上に「審査待ち」の札(白い面・黒い枠と文字)、レビューがまだない店には、空のバーガーと「まだレビューがありません」を出します
- **レビューの一覧**: 1 件ずつのカードで、上に写真、下にバーガーと数字・日付・コメント・バーガーとお店の名前・投稿した人を並べます。PC は 2 列、モバイルは 1 列です。右下の「詳しく見る →」(黒い文字に下線)で詳細に進みます。写真のない投稿は、写真の場所に薄い灰色の面と「写真なし」を出しています(見せ方は、あとで決めます)
- **レビューの詳細**: 上から順に、大きな写真 → バーガー名とお店の名前 → 平均評価(大きな数字と、大きなバーガーと件数。「レビューを書く」ボタンつき)→ レビューを新しい順に縦に並べます。PC もモバイルも同じ流れです。開いているレビューは、黒い太い枠と薄い灰色の面、黄色の札「表示中のレビュー」で分かるようにし、自分の投稿には「編集」「削除」を出します
- **評価のバーガー(遊びの 1 つ目)**: 星の代わりに、バーガーの段が積み上がります。下から、バンズ・パティ(黒)・チーズ(黄)・野菜(灰色)・上のバンズの 5 段で、**塗ってある段の数が評価**です。4.5 のように半分があるときは、いちばん上の段の左半分だけを塗ります。**数字は必ずそばに書きます**(形だけで伝えません)。塗った段は黒い輪郭、空の段は灰色の輪郭と白い面です。バーガーの形は、Penpot の長方形(角の丸み)だけで作っています(絵は使っていません)。`値` ページに、0・1・2・3・4・4.5・5 の見本があります
- **一言の文言(遊びの 2 つ目)**: 空・読み込み中・見つからないとき(404)だけです。日本語は「まだ誰も食べていません」「焼いています…」「売り切れです」(下に「ページが見つかりません」)、英語は "Nobody's eaten here yet"・"Grilling…"・"Sold out"(下に "Page not found")です。空の画面は空のバーガーと「レビューを書く」ボタン、読み込み中の画面は 3 段のバーガーと灰色の棒、見つからない画面は 404 の大きな数字と「ショップ一覧へ戻る」ボタンです。**API が返す検証・権限のエラーの文言は、置き換えません**(それは backend が決めます)
- **英語**: ヘッダーの `JA` / `EN` で切り替える想定です。英語のショップの一覧と、状態の画面を、PC とモバイルで 1 枚ずつ作り、文字が長くなっても収まることを確かめています(店名は投稿された文字のままで、翻訳しません)
- **ロゴ**: 文字だけです。`Hamburger`(太字・黒)と、黄色の面の上に置いた `Evaluation`(標準・黒)を、同じ Noto Sans JP で組んでいます。**黄色は面だけで、文字の色にしません**。イラストは使いません
- **押せるものの見分け**: ボタンは黄色の面に黒い文字(主な操作)か、白い面に灰色の枠と黒い文字(ほかの操作。削除もこの形)、リンクは黒い文字に下線、選んでいるタブは黒い下線、選んでいる言語は黒い面に白い文字です

### 色と文字の値(案)

**色のスタイル**(Penpot の **ASSETS** → Colors、グループ名は「色」。`値` ページに見本があります)

| 名前 | 値 | 使う所 |
|---|---|---|
| `background` | `#ffffff` | ページ・カード・入力欄の背景 |
| `surface-2` | `#f3f2ee` | 選択中のカード・写真なしの場所・読み込み中の灰色の棒 |
| `border` | `#dedcd6` | 区切り線・カードの枠(飾り) |
| `control-border` | `#6b6b66` | 入力欄・ボタンの枠(部品の境界) |
| `text` | `#111111` | 本文・見出し・リンク・選択中の下線と枠・バーガーの輪郭とパティ |
| `text-muted` | `#5a5a55` | 補足の文字(日付・件数) |
| `accent` | `#e9b824` | 差し色(マスタード黄)。**塗りだけ**: ボタンの面・札の面・ロゴの面・バーガーのチーズ |
| `on-accent` | `#111111` | 黄色い面の上の文字(黒) |
| `burger-bun` | `#ebe8de` | バーガーのバンズの面 |
| `burger-veggie` | `#a8a8a1` | バーガーの野菜の面 |
| `burger-empty` | `#767671` | バーガーの空の段の輪郭 |

- **黄色は、白い地では文字にも線にも使いません**(白との比が 1.85:1 で、文字の基準 4.5:1 にも、図形の基準 3:1 にも届かないため)。黄色は「塗り」だけに使い、その上の文字は黒にします(10.20:1)
- ボタンの黄色い面と白い背景の境目の比も 1.85:1 で、境目の色には頼っていません。ボタンは、黄色の面の上の**黒い文字**で見分けます
- 削除やエラーの色(赤など)は、まだ決めていません(いまの案は、削除ボタンも、灰色の枠のボタンです)
- ダーク(暗い背景)は作りません

**文字**: Noto Sans JP(Google Fonts)を、標準(400)・中(500)・太字(700)で使います。大きさは、`値` ページの見本のとおりです(見出し 32px・モバイルの見出し 28px・カードの店名 22px・小見出し 20px・本文 16px・ボタンと補足 14px・札 12px)。**Penpot の画面で文字を描くには、Google Fonts の読み込みが必要**で、インターネットにつながっていないと、代わりのフォントで表示されます

### 文字と背景のコントラスト(WCAG AA。実測)

`値` ページの表と同じです。ページの中で、実際の色の値から計算しています(`design/redesign/tokens.html`)。文字は 4.5:1、部品の境目や図形は 3:1 が基準です。

| 組み合わせ(前景 / 背景) | 比率 | 基準 | 結果 |
|---|---|---|---|
| 本文・リンクの文字(黒) / ページの背景 | 18.88:1 | 4.5:1 以上 | ○ |
| 本文の文字(黒) / 選択中のカードの背景 | 16.86:1 | 4.5:1 以上 | ○ |
| 補足の文字 / ページの背景 | 6.93:1 | 4.5:1 以上 | ○ |
| 補足の文字 / 選択中のカード・写真なしの場所の背景 | 6.19:1 | 4.5:1 以上 | ○ |
| ボタン・札・ロゴの文字(黒) / 黄色の面 | 10.20:1 | 4.5:1 以上 | ○ |
| 言語切替の選んだ側の文字(白) / 黒の面 | 18.88:1 | 4.5:1 以上 | ○ |
| 写真の仮の場所の文字(黒) / 暗い側の色 `#a5714a` | 4.55:1 | 4.5:1 以上 | ○ |
| 入力欄・ボタンの枠 / ページの背景 | 5.36:1 | 3:1 以上 | ○ |
| バーガー・キーボードで選んだ枠・選択中の枠(黒) / ページの背景 | 18.88:1 | 3:1 以上 | ○ |
| 選択中のカードの枠(黒) / カードの背景 | 16.86:1 | 3:1 以上 | ○ |
| バーガーの空の段の輪郭 / ページの背景 | 4.57:1 | 3:1 以上 | ○ |
| (使わない)黄色の文字 / ページの背景 | 1.85:1 | 4.5:1 以上 | 使わない(届かない) |
| (使わない)黄色の線・枠 / ページの背景 | 1.85:1 | 3:1 以上 | 使わない(届かない) |

- 実際の写真の上には、文字を置いていません(店名・評価は写真の下の白い地。仮の場所の「写真」の文字は、印です)。写真の上にあるのは、「審査待ち」の札(白い面・黒い文字)だけで、札の面が不透明なので、写真の明るさに左右されません
- バーガーの「塗った段」と「空の段」は、輪郭の色と太さ(黒 / 灰色)と面の色で違いを出していますが、塗った段の面の色(灰色がかった白や黄色)は、白い背景との比が低いので、面の色だけには頼っていません。数字が、必ずそばにあります
- 押せる部分は 44px 以上にしています(ボタン・入力欄・タブ・言語の切り替え・「詳しく見る →」などの行)。キーボードで選んだときは、黒の 2px の枠を、要素の 2px 外側に出す想定です(`値` ページに見本)

### 写真・文言・数字は、仮のものです

- **写真は、まだありません。** 写真の場所には、明るい茶色・緑・赤みのグラデーションと「写真」の文字を置いています。実際の写真が入ると、見た目の印象が変わるので、写真を入れた確認が、あとで必要です。グラデーションの向きは、Penpot と HTML で少し違います(仮の場所だけの差です)
- 文言は、日本語を案として書いています。**いまのアプリの画面は英語の表示**なので、日本語の文言は、アプリにまだありません(日本語と英語の切り替えは、アプリ側の別の作業です)
- 店名・レビューの文・ユーザー名は、開発用のサンプルデータ(`backend-go/cmd/seed`)に合わせています。ショップの一覧の「バーガーラボ 中目黒」「グリルバーガー 吉祥寺」と、評価・件数の数字は、見た目を確かめるための架空のものです

### いまの API にない情報(実装のときに必要になるもの)

見た目を作るために、いまのデータにないものを、次のように置いています。実装のとき、データを足すかどうかは、別に決めます。

- ショップの一覧の写真と、平均評価・件数(いまの一覧は、店名と状態だけです)
- レビューの詳細で、同じバーガーのほかのレビューを並べること(いまの詳細は、開いた 1 件だけです)
- 写真のない投稿の見せ方

### 実装のときの決め事(メモ)

- **バーガーの段の数は、`GET /meta` の評価の最大値(`rating.max`)から決めます**(ラフは 5 段)。フロントに 5 を書き写しません。最大値が 5 でなくなるときの段の並び(上下のバンズは固定で、間を、パティ・チーズ・野菜の順に繰り返す、など)は、そのとき決めます
- 評価のバーガーは、平均を、いちばん近い 0.5 に丸めて描きます(3.8 は 4、4.7 は 4.5)。**数字は丸めずに、そのまま出します**
- バーガーの絵は、読み上げの文字(「5 段階中 4.5」)を持たせます。空・読み込み中の画面の絵は、飾りなので読み上げません
- 投稿画面の評価の入力は、今回のラフの対象外です。作るときは、キーボードで操作できる部品(矢印キーで増減する選択肢)にして、数字を必ず表示します
- 一言の文言に置き換えるのは、**空・読み込み中・見つからない(404)だけ**です。API が返す検証・権限・通信のエラーの文言は、backend が決めるので、フロントは、そのまま出します

### 作り方(作り直す手順)

「いまの画面のデザイン」と同じく、ヘッドレスのブラウザで HTML を開いて、位置・色・文字を一覧(JSON)にし、Penpot の API で登録しています(文字は編集できる文字のままです)。見本の HTML は `design/redesign/` にあります(`mock.css` が見た目、`shops.html`・`reviews.html`・`review-detail.html`・`states.html`・`tokens.html` が画面。言語は `?lang=en` で切り替え。バーガーは `mock.js` が `data-score` から組みます)。値や配置を直したいときは、この HTML を直して、次の手順で作り直します。リポジトリのルートで実行します。

```bash
# 0. 作業用のディレクトリ(「いまの画面から作り直す」と同じ。Playwright を入れておく)
WORK=/tmp/penpot-work && mkdir -p $WORK && cp design/scripts/*.js design/scripts/*.py $WORK/
PW_IMAGE=mcr.microsoft.com/playwright:v1.50.1-jammy
docker run --rm -v $WORK:/work -w /work $PW_IMAGE npm i --no-save playwright@1.50.1

# 1. 見本の HTML を、位置・色・文字の一覧(JSON)にする(12 画面と、値のページ。色の値とコントラストの計算結果も出す)
docker run --rm -v $WORK:/work -v "$PWD/design/redesign":/mock:ro -w /work -e OUT=/work/out-redesign $PW_IMAGE node capture_mock.js

# 2. Penpot を起動してユーザーを作り、パスワードを $WORK/penpot-pw.txt に 1 行で書く(「起動・停止・初期化」と「最初のユーザーを作る」)

# 3. Penpot に作る(数秒)
python3 $WORK/build_redesign.py --email <メール> --password-file $WORK/penpot-pw.txt --captures $WORK/out-redesign --map-out $WORK/map-redesign.json

# 4. 書き出す(「(参考)ブラウザを使わずに書き出す・読み込む」の export-binfile。ファイルの id は map-redesign.json の fileId)
#    → design/files/hamburger-evaluation-redesign.penpot に置く。ビューアーで開く前に書き出す

# 5. 画像にする。map-redesign.json の screens に、値のページの 1 行(key は tokens、size は pc、page は 値、frame は デザインの値)を足しておく
docker run --rm --network hamburger-penpot_penpot -v $WORK:/work -w /work \
  -e PENPOT_URL=http://localhost:9001 -e PENPOT_EMAIL=<メール> -e PENPOT_PASSWORD_FILE=/work/penpot-pw.txt \
  -e MAP_JSON=/work/map-redesign.json -e OUT=/work/render-redesign -e CLIP=1 -e QUALITY=72 $PW_IMAGE \
  sh -c 'node forward.js & sleep 1; node render_penpot.js'
```

- `CLIP=1` にすると、`render_penpot.js` が、画面の枠だけを切り出した JPEG を作ります(既定の PNG は、枠のまわりの余白つきです)。`tokens-pc.jpg` は、`design/screenshots/redesign/values.jpg` の名前で置いています
- 見本の HTML(ブラウザ)と Penpot の画面を画像にして、主な文字 24 か所の上下と左端の位置を、画素(暗い画素の外接)で比べました。**23 か所のずれが 1px 以内**で、残りの 1 か所は、リンク(「詳しく見る →」)の下線の位置が 2px 違いました(HTML は下線を文字から 3px 離していますが、Penpot の下線は文字に近い)。Noto Sans JP では、Penpot が文字を、登録した行の位置(y)よりも大きさの 0.28 倍ほど上に描いたので、行の位置を「行の上端 + 行の高さ」(`build_redesign.py` の `BASELINE` = 1.0)で登録しています。「いまの画面から作り直す」の `build_penpot.py` の既定は、以前のとおり 0.8 のままです(Penpot の画像で文字が少し上に見えるのは、同じ理由と考えられますが、そのフォントでは確かめていません)
- 長方形の位置と大きさは、整数の px に丸めています(小数のままだと、暗い帯や輪郭の端に、細い線が出るため)
- `.penpot` の大きさは、約 1.07 MB(1,120,018 バイト)です。画像は 13 枚で、1 枚あたり 33〜200 KB です

### できていないこと・注意(このラフ)

- **部品(コンポーネント)にしていません。** ボタン・カード・札・バーガーなどは、同じ見た目の普通の図形です(バーガー 1 つは、長方形が 5〜6 つ)。決まった方向で画面を増やすときに、部品にします
- **状態は、画面 1 種類ずつです。** マウスを乗せた・押した・入力中・エラーなどは、作っていません。空・読み込み中・見つからないときは、文言の見本として、1 枚に 3 つを並べただけです(実際の画面の中での見え方は、別に決めます)。キーボード操作の表示だけ、`値` ページに見本があります
- **配置は、位置の固定です。** Penpot の自動配置(Auto layout)や、大きさの追従は付けていません
- 残りの画面(サインイン・新規登録・ショップの詳細・投稿・プロフィール・管理)は、方向が決まってから作ります

## ポートを変える

9001 が他のアプリと重なるときは、`design/.env` の `PENPOT_PORT` を変えて、`up -d` し直します(ブラウザで開く URL も、その値に追従します)。

## バージョンの更新

1. `design/.env.example` と `design/.env` の `PENPOT_VERSION` を、新しい安定版(<https://github.com/penpot/penpot/releases>)に変える
2. 更新する前に、開いている大事なファイルを `.penpot` に書き出しておく(バージョンをまたぐ互換のため)
3. `up -d` し直す(新しいイメージを取得して、データは残ったまま起動する)
4. 公式の compose の変更(`docker/images/docker-compose.yaml`)が、`design/docker-compose.yml` に必要か、差分を見る

## 公式の構成から外したもの

| もの | 理由 |
|---|---|
| `penpot-mcp` | エージェントが Penpot を読む仕組み。この story の対象外(必要になったら別の story)。外しても frontend は起動する(確認済み) |
| `penpot-mailcatch` と `enable-smtp` | メールは送らない。SMTP を有効にしない Penpot は、メールの内容をログに出すだけ。メールの確認もしない(`disable-email-verification`) |
| テレメトリ | 匿名の利用データを送らない(`PENPOT_TELEMETRY_ENABLED=false`) |
| `restart: always` | Docker の起動で勝手に立ち上がらないようにした |

## 動作の目安(実測)

測定: Penpot 2.17.2、Docker Desktop の VM(4 CPU・7.7 GiB)、Apple Silicon(arm64)。

| 項目 | 値 |
|---|---|
| イメージの合計(ディスク) | 約 4.0 GiB(exporter 2.17 GB・backend 739 MB・frontend 540 MB・postgres 468 MB・valkey 142 MB) |
| 起動時間(イメージ取得済み、空の volume から) | frontend が応答するまで 約 10 秒、backend の API が応答するまで 約 20〜25 秒(実測 19 秒・25 秒) |
| 初回のイメージの取得 | 数分(回線による) |
| メモリ(使用中) | 合計 約 1.2 GiB(backend 約 780 MiB・exporter 約 170 MiB・frontend 約 115 MiB・postgres 約 135 MiB・valkey 約 10 MiB) |
| メモリの上限(`mem_limit` の合計) | 3.2 GiB(frontend 192 MiB・backend 1.5 GiB・exporter 768 MiB・postgres 512 MiB・valkey 256 MiB) |

## トラブルシュート

- `PENPOT_SECRET_KEY` が未設定のエラー → 「初回セットアップ」の 2 を実行する
- `create-profile` で `No such container` → `docker ps` で、名前が `<プロジェクト名>-penpot-backend-1` のコンテナを確認する
- メモリ不足で落ちる → 他のコンテナを止める。それでも足りなければ `design/docker-compose.yml` の `mem_limit` を上げる
