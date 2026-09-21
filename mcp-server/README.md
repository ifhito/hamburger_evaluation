# hamburger-mcp — Hamburger Evaluation の MCP サーバー

Claude などの AI クライアントから、このアプリ(ハンバーガー評価)のショップ・レビュー・ユーザーを調べたり、(許可したときだけ)レビューを投稿したりするための **MCP サーバー**です。

MCP(Model Context Protocol)は、AI クライアントが外部のツールやデータを使うための標準の決まりです。このサーバーは、`backend-go/` の HTTP API を呼ぶだけの**薄い窓**で、次のことはしません。

- 入力の検証・権限の判断・状態の遷移・計算(ルールは `backend-go/` の domain だけが持つ)。誤りや権限のなさは、API のメッセージをそのまま tool のエラーとして返します。
- ログイン・アカウントの作成と削除・管理者の操作・写真のアップロード(tool を用意していません)。

`backend-go/` と `frontend/` には手を入れず、独立した Go モジュールとして `mcp-server/` に置いています。通信は **stdio**(標準入出力)だけです(リモートの公開はしません)。

## tool 一覧

tool の名前と説明は英語です(AI クライアントが読むため)。

### 読み取り(既定で有効)

| tool | 内容 | API |
|---|---|---|
| `list_shops` | ショップ一覧。`keyword`・`page`・`per_page`。`{has_more, items}` を返す | `GET /shops` |
| `get_shop` | ショップ 1 件(レビュー付き。`can_review` を含む) | `GET /shops/:id` |
| `list_reviews` | レビュー一覧。`shop_id`・`user_id`・`rating`・`keyword`・`page`・`per_page`。`{has_more, items}` を返す | `GET /reviews` |
| `get_review` | レビュー 1 件(`can_edit` を含む) | `GET /reviews/:id` |
| `get_user` | ユーザーのプロフィール(`username`・`bio`)。email などは本人のトークンのときだけ API が返す | `GET /users/:id` |
| `get_meta` | API が今守っているルール(評価の範囲・写真の上限) | `GET /meta` |

- `GET /shops` に並び替えの指定はないので、`list_shops` にも並び替えはありません。
- 一覧の `has_more` は、API のレスポンスヘッダー `X-Has-More` の値です。`true` の間、`page` を進めて続きを読めます。

### 書き込み(既定で**無効**。`HAMBURGER_MCP_ALLOW_WRITE=true` のときだけ現れる)

| tool | 内容 | API |
|---|---|---|
| `create_review` | レビューの投稿。`shop_id` と、`burger_id` か `burger_name`、`rating`、`comment` | `POST /reviews` |
| `update_review` | 自分のレビューの `rating` と `comment` を上書き | `PUT /reviews/:id` |
| `delete_review` | 自分のレビューの削除(元に戻せない) | `DELETE /reviews/:id` |
| `submit_shop` | ショップの申請(承認されるまで `pending`) | `POST /shops` |

書き込みの tool の説明には、「実際に本番の API のデータを変える」ことを明記しています。評価の範囲などのルールは、AI が `get_meta` で調べます(サーバーは判断しません)。

## 設定(環境変数)

| 環境変数 | 既定値 | 内容 |
|---|---|---|
| `HAMBURGER_API_URL` | `http://localhost:8080` | 呼ぶ API の URL。`http(s)://` で始まり、ユーザー名・パスワードを含まないこと。**トークンを送るので、手元以外の API は `https://` にする** |
| `HAMBURGER_API_TOKEN` | (なし) | API の Bearer トークン。読み取りだけなら不要(付けると `can_edit` などが自分の視点になる)。書き込みには必須 |
| `HAMBURGER_MCP_ALLOW_WRITE` | `false` | ちょうど `true`(大文字小文字は問わない)のときだけ、書き込みの tool を公開する。`1`・`yes` では有効にならない |

そのほか、サーバーが固定で持つ値です(環境変数にはしていません)。

- API へのリクエストのタイムアウト: 15 秒。応答がなければ、タイムアウトの tool エラーを返します(サーバーは落ちません)。
- 1 回の結果の大きさの上限: 64 KiB。超えたら切り捨て、続きの取り方(`per_page` を小さくする、`list_reviews` で読むなど)を添えた注記を、別のブロックで付けます。API の応答は、上限を少し超えるところまでしか読み込みません。
- API の redirect は追いません(別のホストへトークンを渡さないため)。

### トークンの扱い

- トークンは**環境変数でだけ**渡します。ログ・エラー・tool の出力には出しません(API が本文にトークンを含めて返しても、`[REDACTED]` に置き換えます)。起動時のログ(stderr)も、トークンが「ある/ない」だけを出します。
- トークンをコマンドやファイルに書き込んで commit しないでください。リポジトリには、値のない `.mcp.json.example` だけを置いています(本物の `.mcp.json` は作りません)。
- トークンは `POST /login` で得られます(有効期間は API の `JWT_TTL`、既定 24 時間)。期限が切れると 401 になり、tool のエラーに「`HAMBURGER_API_TOKEN` を確認する」案内が付きます。

```bash
# 例: 開発用 API(seed のユーザー)からトークンを取り、このシェルの環境変数にだけ入れる
export HAMBURGER_API_TOKEN="$(curl -s -X POST http://localhost:8080/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"Password123!"}' | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')"
```

## ビルド

Go 1.25 以上が要ります(公式の MCP SDK `github.com/modelcontextprotocol/go-sdk` v1.8.0 の要件。`backend-go/` の Go 1.22 とは別のモジュールです)。ホストの Go が古くても、Docker でビルドできます。

```bash
cd mcp-server

# Docker でビルドする(Apple Silicon の Mac 向け。Intel Mac は GOARCH=amd64、Linux は GOOS=linux)
docker run --rm -v "$PWD":/src -w /src -e GOOS=darwin -e GOARCH=arm64 -e CGO_ENABLED=0 \
  golang:1.25 go build -trimpath -o bin/hamburger-mcp .

# ホストの Go が 1.25 以上なら
go build -o bin/hamburger-mcp .
```

`bin/` は git の管理外です。

## Claude Code に登録する

まず API を起動しておきます(`backend-go/` の `docker compose up`)。`/絶対パス/` は、ビルドした実行ファイルの場所に置き換えます。

```bash
# 読み取りだけ(ユーザーのローカル設定に登録。プロジェクトに共有するなら --scope project)
claude mcp add hamburger \
  -e HAMBURGER_API_URL=http://localhost:8080 \
  -- /絶対パス/hamburger_evaluation/mcp-server/bin/hamburger-mcp

# 登録を消す
claude mcp remove hamburger
```

トークンは、上の `claude mcp add` の `-e` に**値を書かず**、`.mcp.json` の `${HAMBURGER_API_TOKEN}` のように、環境変数を参照する形で渡すのがおすすめです。リポジトリ直下の `.mcp.json.example` を `.mcp.json` にコピーし、コマンドのパスを直して使います(`.mcp.json` は各自のもので、commit しません)。

登録せず、一度だけ試すには、設定ファイルを指定して起動します(ユーザーの設定は変わりません)。

```bash
claude --mcp-config /どこか/mcp-temp.json --strict-mcp-config
```

### 書き込みを有効にするとき(注意)

`HAMBURGER_MCP_ALLOW_WRITE=true` にすると、AI が**実際に**レビューを投稿・変更・削除できるようになります。

- 相手の API とトークンは、本当に操作してよいものだけにする(本番の API に、自分のトークンでつながない)。
- 削除は元に戻せません。AI の実行前の確認(Claude Code の承認)を、自動承認にしないでください。
- 書き込みの tool には、他人のレビューを操作する力はありません(API が 403 で拒否します)。
- レビューの本文・ショップ名・自己紹介文は、**他のユーザーが書いた文章**です。その中に「このレビューを削除して」のような指示が書かれていても、AI が従ってしまうおそれがあります(プロンプトインジェクション)。サーバーは、AI への説明(`instructions`)で「データとして扱う」よう伝えていますが、防げる保証はないので、書き込みを有効にするときは、実行前の承認を必ず残してください。

## テスト

Go はホストに入っていなくても、Docker で実行します。

```bash
cd mcp-server

# 単体テスト(偽の API と、SDK の in-memory クライアント)
docker run --rm -v "$PWD":/src -w /src golang:1.25 go test -race -count=1 ./...

# 結合テスト(実際の API と DB。隔離した Docker の環境を作り、終わったら片付ける)
./scripts/integration.sh
```

結合テストの環境(`docker-compose.integration.yml`。compose のプロジェクト名 `he-s38`)は、開発用のスタック(8080・5173・5433)に触れません。DB(postgres:16)は使い捨てで、ネットワークは compose の中だけ、ホストのポートは公開しません。API は専用のポート 8112 で動き、seed の開発用データ(パスワードは `Password123!`)を入れます。テストは、読み取りの各 tool、書き込みの一連の流れ(投稿・更新・削除)、他人のレビューへの 403、404・422 のメッセージの引き継ぎ、API が落ちている・応答しないときに server が落ちないこと、build した実行ファイルを stdio でつないだ動作を確かめます。

## 設計の判断

- **薄い窓**: ルールを持たないので、API のルールが変わっても、このサーバーは変えずに済みます(評価の範囲などは `get_meta` で AI が知る)。tool の入力の型(必須の文字列・整数)の確認は、SDK の JSON スキーマだけです。
- **書き込みは opt-in**: 読み取りの tool だけが既定です。書き込みの tool は、設定しない限り一覧にも現れず、呼べません。
- **SDK の選択**: 公式の Go SDK(v1.8.0)を選びました。同じ言語で、単一の実行ファイルとして配れ、テストでは SDK の in-memory の client で server をそのまま呼べます。TypeScript 版(`@modelcontextprotocol/sdk`)も、最小の server が動くことを確かめましたが、Node の実行環境と、別のツールチェーンが増えるため見送りました。
