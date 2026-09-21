# Google でのサインインの準備(Google Cloud と環境変数)

このアプリで「Google でサインイン」を使うには、Google Cloud で **OAuth クライアント**(このアプリを Google に知らせる登録)を 1 つ作り、その **クライアント ID** と **秘密の鍵** を、API の環境変数に渡します。所要は 10〜15 分です。

> **秘密の鍵の扱い**: クライアントの秘密の鍵は、パスワードと同じ秘密です。リポジトリ・チャット・PR・スクリーンショット・ログに、書いたり貼ったりしません。`.env` などのファイルに書く場合も、Git に入れません(`.gitignore` に入っていることを確かめてください)。漏らしたときは、Google Cloud で、その場で鍵を作り直します。

## 1. Google Cloud のプロジェクトを用意する

1. <https://console.cloud.google.com/> を開き、Google アカウントでサインインします。
2. 画面上部のプロジェクトの選択から、**新しいプロジェクト**を作ります(名前は自由。例: `burgerstack-dev`)。作ったプロジェクトを選んでおきます。

## 2. 同意画面を設定する(初回だけ)

Google が利用者に見せる「このアプリがあなたのアカウントにアクセスしようとしています」の画面の設定です。メニューの **Google Auth Platform**(古い画面では **API とサービス → OAuth 同意画面**)を開きます。

| 項目 | 入れる値 |
|---|---|
| アプリ名 | アプリの名前(例: `BurgerStack`)。同意画面に出ます |
| ユーザーサポートメール | 自分のメールアドレス |
| 対象(ユーザーの種類) | **外部** |
| デベロッパーの連絡先メール | 自分のメールアドレス |

- 公開ステータスは **テスト**のままにします(自分たちだけが使う間は、Google の審査は要りません)。
- **テストユーザー**に、サインインを試す Google アカウントのメールアドレスを追加します(テストの間は、ここに載せたアカウントだけが使えます)。
- スコープ(アクセスする情報)は、`openid`・`email`・`profile` だけです。この 3 つは、審査が要らない、基本のものです。**それ以外は足しません**(このアプリは、Google のデータを読みません)。

## 3. OAuth クライアントを作る

1. メニューの **クライアント**(古い画面では **API とサービス → 認証情報 → 認証情報を作成 → OAuth クライアント ID**)を開きます。
2. アプリケーションの種類は **ウェブ アプリケーション**にします。名前は自由です(例: `burgerstack-local`)。
3. **承認済みのリダイレクト URI** に、次の値を**そのまま**追加します(手元で動かす場合)。

   ```
   http://localhost:5173/api/auth/google/callback
   ```

   - **画面(5173)の `/api/…` を通る形**にします。画面の `/api` は、API(8080)へ転送されます。手続きの cookie(コードを使える相手を確かめる cookie を含む)は、画面から API を呼ぶ道筋(`/api/…`)で往復するので、戻り先も、同じ `/api` を含めます。**API に直接(`http://localhost:8080/auth/google/callback`)戻すと、画面からのサインインが、毎回「リンクが無効」になります**(交換の cookie の Path が、画面が呼ぶ `/api/auth/google/exchange` と合わないため)。
   - **完全に一致**させる必要があります(`http` か `https` か、ポート、末尾の `/` の有無、`localhost` か `127.0.0.1` か)。1 文字でも違うと、Google が `redirect_uri_mismatch` で断ります。
   - 「承認済みの JavaScript 生成元」は、空で構いません(このアプリは、画面から Google のスクリプトを読み込みません)。
4. 作成すると、**クライアント ID** と **クライアントシークレット**(秘密の鍵)が表示されます。この 2 つを控えます(シークレットは、あとから画面で確かめられます)。

## 4. API に環境変数を渡す

API を起動するシェルで、次を設定します(値は、控えたものに置き換えます。`docker compose` は、この環境変数を、コンテナへ渡します)。

```bash
export GOOGLE_CLIENT_ID='xxxxxxxx.apps.googleusercontent.com'
export GOOGLE_CLIENT_SECRET='ここに秘密の鍵'          # 秘密。履歴に残したくなければ、read -s を使う
export GOOGLE_REDIRECT_URL='http://localhost:5173/api/auth/google/callback'   # 手順 3 で登録した値と同じ
```

`GOOGLE_CLIENT_SECRET` を、履歴に残さずに設定するには:

```bash
read -rs GOOGLE_CLIENT_SECRET && export GOOGLE_CLIENT_SECRET   # 入力は画面に出ない。貼り付けて Enter
```

`GOOGLE_CLIENT_ID` を設定すると、機能が有効になります。`GOOGLE_CLIENT_SECRET` と `GOOGLE_REDIRECT_URL` は、有効にするなら必須で、足りないと、API は、起動時に、足りない変数の名前(値ではなく)を出して止まります。

**URL の制約**: 有効にするときは、`GOOGLE_REDIRECT_URL` と `APP_BASE_URL`(既定は `http://localhost:5173`)は、**https の URL、または開発用のループバック(`localhost`・`127.0.0.1`・`[::1]`)の http** だけを受け付けます。`GOOGLE_REDIRECT_URL` の path は、`/auth/google/callback` で終わらなければなりません(前に `/api` などの接頭辞が付くのは構いません)。外部のホストの http だと、認可コードや、1 回限りのコード(ログインの証に交換できる)が、平文で流れるので、起動時に断ります(変数の名前だけが出ます)。

## 5. データベースを作り直す(初回だけ)

この機能で、`users` の作りが変わりました(パスワードなしのアカウントを作れるようにするため)。まだ実運用前なので、追加の変換ではなく、**最初の定義を直しています**。すでに適用済みの開発用のデータベースには、この変更は反映されないので、作り直します(**中のデータは消えます**。テスト用のデータだけであることを確かめてください)。

```bash
cd backend-go
docker compose run --rm migrate drop -f
docker compose run --rm migrate up
docker compose run --rm seed      # 開発用のデータを入れ直す
```

## 6. 起動して確かめる

```bash
cd backend-go && docker compose up -d --build
```

まず、API が、Google でのサインインを有効にしているかを確かめます(この確認は、画面がなくてもできます)。

```bash
curl -s http://localhost:8080/meta | python3 -c "import sys,json; print(json.load(sys.stdin)['login_providers'])"
```

`['google']` と出れば有効です。空(`[]`)なら、環境変数が API に届いていません(手順 4 を見直して、`docker compose up -d` し直します)。

### ブラウザで確かめる

```bash
cd frontend && docker compose up -d --build
```

1. <http://localhost:5173/> を開き、サインインの画面に **「Sign in with Google」** のボタンが出ることを確かめます。
2. ボタンを押し、テストユーザーに入れた Google アカウントでサインインします。
3. 初めてなら、新規登録されて、アプリに戻ります(メールの確認は要りません)。ユーザー名は、Google の名前から決まります(名前がなければ `user-` で始まる名前になります)。プロフィールで変えられます。
4. すでに、同じメールアドレス(大文字小文字は区別しません)でパスワードのアカウントがあるときは、**自動では結び付けません**。案内が出るので、パスワードでサインインし、プロフィールの「Google」から結び付けます(結び付けの手続きは、押した**そのブラウザ**で始まります。別のブラウザで開いても、結び付きません)。
5. 手続きが終わると、ブラウザの cookie(`google_login_flow_…`・`google_login_handoff_…`)は、残りません(開発者ツールの Application → Cookies で確かめられます)。

## うまくいかないとき

| 症状 | 原因と対処 |
|---|---|
| `Error 400: redirect_uri_mismatch` | 手順 3 の「承認済みのリダイレクト URI」と、`GOOGLE_REDIRECT_URL` が、完全には一致していません。両方を見比べて、揃えます |
| `Access blocked: … has not completed the Google verification process` / `Error 403: access_denied` | そのアカウントが「テストユーザー」に入っていません。手順 2 で追加します |
| ボタンが出ない | `GET /meta` の `login_providers` が空です。API を起動したシェルで、`GOOGLE_CLIENT_ID` を設定してから、`docker compose up -d` し直します。ブラウザが、古い `GET /meta` の応答(最大 1 時間キャッシュされます)を持っているだけのこともあります(ハードリロード) |
| 「The Google sign-in link is invalid or has expired」が毎回出る | `GOOGLE_REDIRECT_URL`(と、Google Cloud の「承認済みのリダイレクト URI」)が、画面の `/api` を通る形(`http://localhost:5173/api/auth/google/callback`)になっていません。API に直接(`:8080`)戻していないか、見直します。API の起動ログに、`config: warning: GOOGLE_REDIRECT_URL (…) は、APP_BASE_URL (…) と別のオリジンです` と出ていれば、これです(`docker compose logs api-go`) |
| サインインが「失敗」になる(手続きの途中で戻される) | 画面を開いた場所と、API の場所を、**`localhost` で統一**します(`127.0.0.1` と混ぜると、手続きの cookie が届きません)。ブラウザの cookie を無効にしていないかも見ます |
| 起動時に `GOOGLE_… is required` | 有効にしたのに、足りない変数があります(変数の名前だけが出ます) |

## 本番に公開するとき

- 公開の **https の URL**(例: `https://app.example.com/api/auth/google/callback`)を、「承認済みのリダイレクト URI」に足し、`GOOGLE_REDIRECT_URL` も、その値にします。`http` は使いません(起動時に断ります。cookie に Secure が付くのは、`https` のときです)。
- **画面と API は、同じサイト(同じホスト。`/api` を API へ転送する構成)で公開します。** 手続きの cookie(`google_login_flow_…`)は、サインインを始めた画面(`/api/auth/google/start`、または結び付けの `POST /api/me/identities/google/link`)の応答で、そのブラウザに設定され、Google からの戻り(`GOOGLE_REDIRECT_URL`)の要求にだけ送られます(Path は戻り先の path そのもの)。戻りの応答は、コードを使える相手を確かめる cookie(`google_login_handoff_…`。Path は `…/api/auth/google/exchange`)を設定し、画面の `POST /api/auth/google/exchange` にだけ送られます(コードだけを別のブラウザへ持ち込んでも、交換できません)。どちらの cookie も、手続きごとに別の名前なので、複数のタブで並行しても、互いを上書きしません。**`GOOGLE_REDIRECT_URL` の path は、必ず `/auth/google/callback` で終わらせてください**(起動時に断ります。交換の cookie の Path を、ここから導くためです)。戻り先のホストが、画面のホストと違うと、cookie が届かず、手続きは、失敗します。
- 公開ステータスを「本番」にすると、テストユーザー以外でも使えます。今回の 3 つのスコープだけなら、Google の審査は要りません。
- 秘密の鍵は、環境変数か、ホスティング先の秘密の管理の仕組み(Secret Manager など)で渡します。定期的に作り直せるようにしておきます。

## 開発者向け: 本物の Google なしで動かす

自動テストは、テストの中に立てた、OpenID Connect の提供元の代役(`internal/testutil/fakeoidc`)を使うので、Google の認証情報は要りません。手元で、代役に向けて動かしたいときだけ、`GOOGLE_OIDC_ISSUER` に、代役の URL(`https` か、ループバックの `http`)を設定します。**本番では設定しません。**
