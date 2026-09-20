# Lens: Concurrency

プロセス内の並行性: 共有状態、goroutine のライフサイクル、キャンセル。
(データベースレベルの競合は consistency レンズの担当。)

## 探すもの

1. **同期されていない共有状態** — 複数の goroutine から mutex なしで
   書き込まれる map/slice/struct。`sync.Once` なしの遅延初期化。
   共有フィールドへの check-then-set。
2. **goroutine リーク** — 出口のない goroutine: 誰も読まないチャネルで
   永遠にブロック、あるいは `ctx.Done()` チェックのないループ。
3. **context 伝播の欠落** — 呼び出しチェーンの奥でリクエスト ctx の代わりに
   `context.Background()`。キャンセル済みリクエストより長生きする
   DB/HTTP 呼び出し。
4. **無制限のファンアウト** — ユーザー起因でスケールする入力に対して、
   ワーカープールもセマフォもなしにアイテムごとの goroutine。
5. **WaitGroup/チャネルの誤用** — `Wait` 後の `Add` の競合。close 後の
   チャネル送信。`range` が読むところで `close` を忘れる。
6. **フロントエンドの競合** — アンマウント後の state 更新。同一 SWR キーへの
   直列化されていない並行 mutation。非同期コールバックが古い state を
   捕まえる stale closure。

## 指摘しないもの

- 単一 goroutine のコードパス — 仮想の並行性を発明しない。
- 共有前に一度だけ書かれる値(main で構築されてから読まれる config)。
- もっともらしいインターリービングのない `go test -race` ノイズ候補 —
  Suggestion にして race detector の実行を促す。

## Grep の起点

```bash
grep -rn 'go func' backend-go/internal/ --include='*.go'
grep -rn 'context.Background()' backend-go/internal/ --include='*.go'
```

`go test -race ./...` が正式なチェック。このレンズがもっともらしいものを
見つけたら必ずその実行を提案すること。
