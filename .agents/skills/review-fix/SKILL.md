---
name: review-fix
description: 未コミットの変更を reviewer エージェントと /code-review でレビューし、Critical/Warning の指摘を implementer エージェントで自動修正し、クリーンになるまで再レビューする。コミット前や、ユーザーが「レビューして直して」と頼んだときに使う。
version: 1.0.0
author: BurgerStack Agents
license: MIT
metadata:
  hermes:
    tags: [review, fix, automation, git]
    related_skills: [pr-hygiene, pr-self-review]
---

# Review Fix Loop

## 概要

このリポジトリの `reviewer`/`implementer` エージェントと、Claude Code の
`/code-review`(Skill の `code-review`)を使って、現在のワーキングツリーに
対するレビュー → 修正 → 再レビューのサイクルを自動化する。
ループは、Critical/Warning の指摘が出ないラウンドが来たとき、または
3 ラウンド経過で止まる。

## 実行方法

保存済みワークフローを起動する(このスキルがその呼び出しの認可となる):

- `Workflow` ツールに `name: "review-fix"` を指定する。
- ユーザーが指定したフォーカスエリアがあれば `args`(プレーンな文字列)で
  渡す。例: `args: "authorization checks in the reviews endpoints"`。
  [[focused-review]] スキルの特定のレンズに絞る場合はその名前を挙げる。例:
  `args: "apply the focused-review consistency lens"`。

Agent ツールでループを手動再実装しないこと。ラウンド上限と指摘スキーマの
単一の source of truth はワークフローである。

## `/code-review` の扱い

- Review の段階は、`reviewer` に加えて、**1 ラウンド目に `/code-review` を必ず 1 回**
  走らせる(`origin/main` との差分が対象。重いので 2 ラウンド目以降は `reviewer` だけ)。
  `reviewer` は Skill ツールを持たないので、ワークフローが、Skill を呼べる別のエージェントに
  任せる。`reviewer` 自身は `/code-review` を呼ばない(二重に呼ばない)。
- `/code-review` の結果は「未検証」と明記される。`reviewer` の指摘と、**別々のパスが同じ path:line**
  を指したときだけ 1 つにまとめ(同じパスの別々の指摘はまとめない)、**Warning 以上は必ず V1
  (`verifier`)に通す**。V1 を通らずに修正へ進めない。Suggestion(意見・整理・命名・重複の指摘)は、
  V1 に通さず、自動修正もせず、未検証のまま提示する。`code-review` が修正案を返さない指摘は、
  「修正案なし」として V1 に渡り、自明で局所的でない限り、自動修正の対象にならない。
- **対象のディレクトリに注意**: `/code-review` は、呼んだセッションの作業ディレクトリ(主ディレクトリ)の
  差分を見る(実測)。**`git worktree` で作業しているときは、`args` に `worktree: <絶対パス>` を含める**
  (例: `args: "worktree: /Users/hotake/Documents/he-xxx"`)。含めないと、別の木(主ディレクトリの
  未コミットの変更)を黙ってレビューする。`worktree:` を指定すると、`reviewer`・`verifier`・`implementer`
  にも、そのパスで作業するよう伝わる(指定しないと、みな主ディレクトリを見る)。ワークフローは、
  code-review を呼んだエージェントが報告した対象(`reviewedRoot`)を、指定した `worktree` と**コードで**
  比べ、違えば「別の木をレビューした」、差分が 0 件なら「差分が空」として未実行と扱う(対象を
  報告する部分は、エージェントの自己申告)。`worktree:` を指定しなかったときは、対象を確かめるよう
  `notices` で促す。
- 差分の基準は、`git fetch origin` のあと `git merge-base origin/main HEAD`(古いローカルの `main` から
  切った worktree で、新しいコミットの逆向きの差分が混ざらないように)。未追跡のファイルは
  `git diff` に出ないので、`reviewer` の担当。
- `/code-review` は時間がかかる(中くらいの差分で 10〜25 分)。バックグラウンドの fork として実行され、
  結果は通知として届く。待つ間は、ターンを閉じず、短いコマンド(`date && sleep 25`。`sleep` で
  始まる長いコマンドは、この環境で拒否される)を繰り返す。`ultra`(クラウド)は使わない。
- `/code-review` が使えない・失敗したときは、黙って飛ばさない。結果の `codeReview` に
  `{ ran: false, reason }`、`notices` に「code-review 未実行(理由)」が入る。

## ワークフローが返った後

日本語で報告する。ユーザーに見せるのは、ユーザーの判断が必要なものだけ:

0. **最初に `complete` と `codeReview` を確認する**。`complete: false`(= `codeReview.ran` が false)の
   結果は、`status` が `clean` などでも、**完了として扱わない**。`notices` の内容(未実行の理由)を
   利用者に伝え、**手動で `/code-review` を実行するよう促す**。`codeReview.ran` が true なら、
   `codeReview.findingCount` と `codeReview.findings`(生の指摘)、V1 の結果から、**PR のコメント**
   (指摘の件数・確度・直した内容)を作って残す。
1. **まず `userDecisions` を提示する**。P1 → P2 → P3 の順で、それぞれを
   選択肢・推奨・影響を添えた 1 つの判断質問として示す。冒頭は最大 5 件、
   あふれた分は付録へ。P1 は作業をブロックする。
2. **自動修正された指摘**: ラウンドごとに 1 行のサマリ(件数 + 種類)。
   ユーザーに再承認を求めない。
3. **`discarded`(偽陽性)**: 件数を 1 行で記す。エビデンスは求めに応じて
   提示できる。決して質問として提示しない。
4. `status: "converged"` — そのラウンドに確定した Critical がなかったため、
   修正を適用してループはもう一度のレビューラウンドなしに終了した
   (設計どおり。再開しないこと)。`status: "no-auto-fixable"` は残りが
   すべてユーザー待ちという意味 — 率直にそう言う。
   `status: "max-rounds-reached"` — 未解決項目を列挙して止まる。自分で
   ループを続けないこと。
5. 完全な検証は stop sensors(`python3 .claude/hooks/stop-sensors.py`)で
   引き続き実行されることを念押しする — このループは安価で絞った
   チェックしか走らせない。

## ガードレール

- このループは決してステージ・コミット・プッシュしない。それはメイン
  セッションと `pr-hygiene` スキルの領分。
- Suggestion は意図的に自動修正しない — 意見の自動適用はスコープクリープの
  始まり。代わりに提示する。
- 同じ指摘が 2 ラウンド生き残ったら、reviewer と implementer の見解の相違と
  みなし、最後のラウンドを浪費せずユーザーにエスカレーションする。
