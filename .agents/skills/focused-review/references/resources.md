# Lens: Resources

メモリと CPU のインシデントは無制限増加の問題である: すべてのバッファ、
キャッシュ、プール、goroutine、ループには上限と解放パスが必要。各リソースに
問う: 「何がこれを増やすか?」「何がこれを減らすか?」 — 後者に答えが
ないことこそが指摘事項。

## 探すもの

1. **閉じられないリソース** — `resp.Body`、pgx の `rows`、ファイル、`Stop`
   のない `time.NewTicker`。ループ内の `defer` が解放を関数終了まで先送り。
   early return パスでの `Close` 漏れ。
2. **無制限のメモリ増加** — リクエストごとに追記されるパッケージレベルの
   map/slice。eviction も TTL もないキャッシュ。サイズ上限
   (`http.MaxBytesReader`)なしのリクエスト/レスポンスボディへの
   `io.ReadAll`。Go 側でフィルタするためにテーブル全体をメモリにロード。
3. **goroutine の積み上がり** — read/write/idle タイムアウトのないサーバー。
   ctx タイムアウトのない外部呼び出し。遅いクライアントごとに goroutine と
   メモリが OOM まで滞留する。
4. **ホットスピン** — sleep/バックオフなしでポーリングする `for {}`。
   バックオフも試行上限もないリトライループ。チェック対象の仕事より速い ticker。
5. **プールの設定ミス** — 共有ではなくリクエストごとに作られる
   コネクション/クライアント。`MaxConns` 未設定(負荷時の fd 枯渇)、
   または Postgres の `max_connections` より大きい。
6. **ホットパスのアロケーション churn** — ループ内の文字列連結
   (`strings.Builder` を使う)。ストリーミングや事前確保
   (`make(_, 0, n)`)で足りる所での大きな中間 slice の構築。
7. **フロントエンドのリーク** — `useEffect` の teardown で片付けられない
   `setInterval`/サブスクリプション/リスナー。ページネーションも仮想化も
   なしにレンダーされる無制限のリスト。切り詰められず増え続ける state 配列。

## 指摘しないもの

- プロセス終了が解放パスである短命なパス(CLI、テスト、マイグレーション)。
- 構造上有界なデータ(config のリスト、enum)。
- 測定なしの GC・アロケーションのマイクロチューニング — Suggestion +
  「pprof でプロファイルを」。憶測で Critical にしない。

## Grep の起点

```bash
grep -rn 'io.ReadAll\|NewTicker\|go func' backend-go/internal/ --include='*.go'
grep -rn 'setInterval\|addEventListener' frontend/src/
```
