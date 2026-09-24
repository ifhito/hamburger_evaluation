# Phase 4: Back4App Containers(無料プラン)

- 計測日: 2026-09-22
- URL: `https://hamburgerevaluation-8ljd5w0o.b4a.run`
- 種別: **無料プラン**(カード不要)
- メモリ: **256MB**
- 起動モデル: 常時起動

## レイテンシ

| エンドポイント | n | p50 | p95 | 最大 |
|---|---|---|---|---|
| `GET /meta`(DB なし) | 60 | 193ms | 221ms | 744ms |
| `GET /up`(DB あり) | 60 | 421ms | 656ms | 1,265ms |
| `GET /shops` | 60 | 421ms | 644ms | 688ms |

DB 往復の差分は **228ms**。Neon はシンガポールなので、アプリは米国にあると推定される。
配信は東京の CloudFront エッジ(`x-amz-cf-pop: NRT57`)から出ているが、
コンテナ本体は別の場所にある。

## 写真のアップロード: 失敗

| 画像 | 結果 |
|---|---|
| 24MP(4.6MB) 単発 | **502**(12.3 秒) |
| 24MP 同時 2 本 | **502** |
| 直後の `/up` | **502**(復帰せず) |

**256MB では 24MP の写真が単発でも通らない。** Phase 0 のローカル計測で
単発のピークが 260MiB だったため、予想どおりの結果である。

**Render と違い、自動で復帰しなかった。** 502 が続いた。

## Dockerfile を指定できない

Back4App Containers は **Dockerfile 名を指定できず、既定の `Dockerfile` を見る**。
当初は本番用を `Dockerfile.prod` に置いていたため、開発用(起動時に `go build` する)が
ビルドされて失敗した。

この制約を受けて、リポジトリの Dockerfile を次のように入れ替えた。

| ファイル | 用途 |
|---|---|
| `Dockerfile` | **本番用**(distroless、30.9MB) |
| `Dockerfile.dev` | 開発用。`docker-compose.yml` が使う |
| `Dockerfile.prod` | 互換のための別名 |

**本番用こそ既定の名前に置くべき**という教訓である。

## 一時 URL の期限切れ(2026-09-22 に発生)

計測の途中で、アプリにアクセスできなくなった。

```text
Temporary URL Expired
Your App temporary URL has expired. Please upgrade to a paid plan to continue
developing, or redeploy your app to finish your product experimentation.
```

**無料プランで払い出される URL は一時的なもので、一定期間で失効する。**
継続するには再デプロイするか、有料プランに上げる必要がある。

料金表には「600 時間/月・5 プロジェクト・256MB」と書かれているが、
**URL が期限切れになることは書かれていない**。実際に使ってみるまで分からない制約である。

これにより、Back4App の無料プランは**継続的な運用には使えない**。
「試しに動かしてみる」用途に限られる。

## 評価

無料プランでカード不要という条件は良いが、実用には 2 つの壁がある。

1. **一時 URL が期限切れになる。** 継続運用できない
2. 256MB では API を直接叩かれると写真で落ちる(ブラウザ経由なら通る)

**「試しに動かす」用途に限られる。** 継続的に公開するなら有料プランが必要になる。
