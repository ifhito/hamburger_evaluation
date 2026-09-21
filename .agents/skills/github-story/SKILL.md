---
name: github-story
description: GitHub issue を PRD-lite 構造のユーザーストーリー(要件、仕様、受け入れ条件、完了の定義)として作成する。ユーザーがストーリーを起票したい、機能を issue として計画したい、議論をチケットにしたいときに使う。
allowed-tools: [Read, Grep, Glob, Bash(git log:*), Bash(gh issue:*), Bash(gh label:*)]
version: 1.0.0
author: BurgerStack Agents
license: MIT
metadata:
  hermes:
    tags: [github, issue, story, prd, planning]
    related_skills: [pr-hygiene, backend-go-boundaries, frontend-spa-boundaries]
---

# GitHub Story

## 概要

機能のアイデアを、契約として機能する GitHub issue に変える: オーケストレーターの
Frame ステップは受け入れ条件を直接取り込み、完了の定義はこのリポジトリの
パイプラインにマップされる。ストーリー issue はハーネス全体の上流成果物である。

## 手順

1. **引き出す** — 書き始める前に、ユーザーと以下を確定する(ストーリーを
   変えることだけを聞く):
   - この変更のユーザーは誰で、変更後に何ができるようになるのか?
   - 明示的にスコープ外なのは何か?
   - エラー/エッジの挙動: 失敗したら何が起きるのか?
   - API/スキーマへの影響はあるか?(仕様セクションとサイジングを左右する)
2. **下書き** — 下のテンプレートを埋める。何かを作成する前に下書きを
   ユーザーに見せる。
3. **分割チェック** — ストーリーがおよそ 3 個を超える実装タスクを要する、
   またはバックエンドとフロントエンドに独立した価値を持って触れる場合、
   親からリンクされた複数ストーリーへの分割を提案する。
4. **作成** — `gh issue create --title "<title>" --body-file <draft>` に
   ラベルを付ける: `story` + エリアラベル(`backend-go`、`frontend`)。
   issue の URL を報告する。

## ストーリーテンプレート

```markdown
## 背景 / 課題          ← why this matters, in the user's world
## ゴール               ← outcomes, not implementation
## 非ゴール             ← what this story deliberately does NOT do
## 要件                 ← functional requirements, numbered (R1, R2, …)
## 仕様                 ← concrete contract: endpoints + request/response
                          shapes (snake_case), data model changes, UI
                          behavior, authz rules. Unknowns go to 未解決の問い
## 受け入れ条件          ← numbered (AC1, AC2, …), each testable,
                          Given/When/Then form, MUST include error cases
                          (unauthorized, not found, validation)
## 完了の定義 (DoD)      ← checklist, see below
## 未解決の問い          ← open questions blocking or deferrable, owner per item
## 依存 / リスク         ← other stories, migrations, external services
```

## 要件を書くときの注意

- **frontend でドメインのルールを検証する要件を書かない。** ルール(何が有効か、誰に何が
  許されるか、導出)の判断は backend の `domain` だけが持ち、frontend は説明と表示(サーバーの
  422 メッセージ、`can_*` などの値の表示)だけを行う。「入力を即座にエラーにする」のような
  要件は、frontend に規則の複製を求めることになり、二重管理を生む。即時の入力補助が必要なら、
  HTML 標準の属性で足りるか、backend に判断を問い合わせる形にする。
- 権限の出し分けや「最終ページか」の判定が必要な要件は、backend が値を返す仕様にする
  (frontend に判断を再計算させない)。
- 永続化の要件は、usecase が repository を呼ぶ形で書かない(読み取りは `*Query`、書き込みは
  domain の書き込みオブジェクト。`backend-go-boundaries` を参照)。

## 受け入れ条件のルール

- 各条件は外部から観測可能であること(API レスポンス、UI の状態)—
  「コードがきれい」「正しく実装されている」は不可。
- エラーパスは第一級市民: ハッピーパスの条件しかないストーリーは不完全。
- テストとして表現できない条件は、受け入れ条件ではなく 未解決の問い に
  属する。

## 標準 DoD(調整はしても省略はしない)

- [ ] 受け入れ条件それぞれに対応するテストが存在し green
- [ ] backend-go: go-checks.sh 通過 / frontend: type-check + lint + test 通過
- [ ] レビューパイプライン完了(V1/V2 通過、P1 判断ゼロ)
- [ ] Draft PR → ready → merge 済み(PR に `Closes #<issue>`)
- [ ] ドキュメント更新(API 一覧・CLAUDE.md 等、該当時)

## 引き継ぎ

実装が始まるとき、オーケストレーターは issue を読み(`gh issue view <n>`)、
受け入れ条件をタスク仕様の acceptance criteria として使い、draft PR の
本文に `Closes #<n>` を入れる。実装開始前に 未解決の問い は空か、明示的に
先送りされていなければならない。
