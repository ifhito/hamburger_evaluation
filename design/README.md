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
