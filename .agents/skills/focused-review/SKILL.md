---
name: focused-review
description: 汎用のフォーカスレビュー・レンズ集 — fail-loud、consistency、concurrency、performance、resources(メモリ/CPU/リーク)、security、test quality。差分をこれらの観点のいずれかでレビューするとき、または差分が触れる範囲に合うレンズを選ぶときに使う。
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [review, reliability, consistency, performance, security, testing]
    related_skills: [review-fix, pr-self-review, backend-go-boundaries]
---

# Focused Review

## 概要

汎用のレビューレンズのカタログ。各レンズは 1 種類の欠陥に注意を固定する。
焦点を絞ったパスは、何でも見ようとするレビューが読み飛ばすものを見つける。
差分が触れる範囲に合うレンズを選び、そのリファレンスだけを読み、その
チェックリストでレビューする。

## レンズの選び方

| レンズ | 差分がこれに触れるなら読む | リファレンス |
|------|---------------------------|-----------|
| Fail loud | エラーハンドリング、外部呼び出し、パース、非同期処理 | `references/fail-loud.md` |
| Consistency | 書き込み、トランザクション、マイグレーション、派生データ、frontend と backend のルールの重複、usecase から repository への依存 | `references/consistency.md` |
| Concurrency | goroutine、チャネル、共有状態、キャッシュ | `references/concurrency.md` |
| Performance | クエリ、一覧エンドポイント、データを回すループ、UI のフェッチ | `references/performance.md` |
| Resources | バッファ、キャッシュ、プール、goroutine のライフサイクル、長時間処理 | `references/resources.md` |
| Security | 認証・認可、入力処理、SQL、ファイルパス、シークレット | `references/security.md` |
| Test quality | 新規/変更されたテスト、またはテストなしの挙動変更 | `references/test-quality.md` |

デフォルトでは、差分が最も明らかに触れている 2〜3 個のレンズに絞る。
全レンズを走らせるのは、網羅的なパスを明示的に求められたときだけ。

## 共通ルール(すべてのレンズに適用)

出力:

- 指摘は Critical / Warning / Suggestion に分類し、それぞれに `filepath:line`、
  具体的な故障(入力/状態 → 誤った結果)、修正方法を付ける。
- レンズが何も見つけなければ、1 行でそう言う。水増しは決してしない。
- 選んだレンズの外の問題は通常のレビューパスに回す — 1 行でメモするに
  とどめ、深掘りしない。

偽陽性の抑制:

- 各リファレンスには「do not flag」リストがある。それを尊重する。
- 確信のない指摘は、何を測定・テストすべきかを添えた Suggestion にする。
  憶測で Critical にしない。
- gofmt、go vet、ESLint が捕まえるスタイルの細かい指摘は報告しない。

## リポジトリへの接地

リファレンス内の具体的なパターンはこのリポジトリのスタックを前提とする: Go
(`backend-go/`、net/http + sqlc + pgx)と React/TypeScript(`frontend/`、
SWR + axios)。原則はスタック非依存。スタックが変わったらパターンを更新すること。
