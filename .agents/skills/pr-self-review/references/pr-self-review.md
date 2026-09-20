# PR Self Review リファレンス

## スコープ

このスキルはワーキングツリーだけをレビューする。PR をマージ・クローズ・
書き換えるかどうかは判断しない。

## 手順

1. `git status --short --branch --untracked-files=all` を確認する。
2. `git diff --check` を確認する。
3. `git diff --stat` を確認する。
4. ステージ済みの変更があれば `git diff --cached --stat` を確認する。
5. ファイルを次に分類する:
   - 意図した実装、
   - その実装を支えるテスト/チェック/ドキュメント、
   - 無関係または既存のローカルファイル。
6. 変更エリアに基づいて不足している検証を報告する:
   - backend: gofmt、go vet、go build、go test、
   - frontend: type-check、lint、test、
   - build/API 境界: frontend build。
7. 境界の規約を確認する(差分に該当する場合):
   - frontend にドメインのルールの判断(検証・権限の条件・定数・導出)が入っていないか。判断は
     backend の `domain` だけが持ち、frontend は backend が返した値とサーバーのエラーを表示する。
   - usecase が repository を宣言・保持・呼び出していないか(`.repo.` の呼び出し、`*Repository` の
     型・フィールド、`domain.*Repository` の参照)。読み取りは `*Query`、書き込みは domain の
     書き込みオブジェクトを通す。

## シークレットのパス

`.env*`、`secrets/**` の内容は
決して読まない・含めない。
