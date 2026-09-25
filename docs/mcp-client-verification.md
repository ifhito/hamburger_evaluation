# MCP接続の検証記録

利用者向け案内は `/mcp`。接続URLは `https://burger-stack.com/api/mcp`。
設定やトークンそのものを記録しない。一般向け案内では管理者scopeを要求しない。

## 2026年9月26日の確認

- Codexの既存の運営者接続で `get_meta` と `list_shops(per_page: 1)` が成功。
- ローカルの `codex --version` は0.156.1。`codex mcp login --help` で `--scopes` と `--oauth-client-registration` を確認。
- CLIの設定例は [公式MCPガイド](https://developers.openai.com/codex/mcp/) と上記ヘルプに基づく。設定ファイルを手動編集する例にし、最初の認証で読み取りscopeを明示する。
- 新規一般アカウントでのOAuth、書き込み、失効・解除は未検証。ChatGPT・Claudeでの接続も未検証。
- DCRのみのクライアントは非対応。CIMDまたは事前登録が必要。ブラウザから別Originで直接アクセスする方式はCORS非対応。

既存接続の読み取り成功を、外部ユーザーの新規接続成功とは数えない。

## 外部ユーザーの確認手順（未実施）

運営者以外の一般ユーザーが自分のアカウント・AIアプリで行う。パスワードやトークンを運営者に渡さない。
本番に架空のレビューを作らない。投稿確認には本人が実際に食べた記録を使い、投稿前に内容を確認する。

1. 未ログインで `/mcp` を開き、URLコピーと手動選択を確認する。
2. 案内のCodex設定と読み取り用ログインを行う。アプリ名・バージョン・プラン・workspace制限・確認日を記録する。
3. 自分のアカウントで認証し、要求権限を確認して許可する。ショップ検索とレビュー取得が成功することを確認する。
4. 読み取りのみのまま書き込みを試すと拒否され、実データが増えないことを確認する（拒否の検証はまずローカル統合テストで行う）。
5. 書き込みも許可して、本人が確認した実食レビューを投稿する。プロフィールで本人のレビューとして表示されることを確認する。
6. プロフィールの「接続済みのアプリ」で接続を解除する。キャッシュされた回答ではなく新たなツール要求が拒否されることを確認する。
7. 再認証すると再び読み取りできることを確認する。
8. 管理操作の一般ユーザー拒否、トークンなし・期限切れ・失効、DCR拒否は既存のbackendテストも併用する。管理操作の成功を期待した本番試行はしない。

結果は個人情報を除いてIssue #228 / PRに記載し、この検証記録を更新する。公開案内には検証状況を掲載しない（ユーザーの希望）。
AC2〜AC6の外部接続に関わる確認を終えるまでIssueを完了扱いにしない。

## 既存の自動検証箇所

今回backendは変更していない。以下は参照先であり、今回実行済みという意味ではない。

- `backend-go/internal/adapter/handler/mcp_test.go`: 一般ユーザーの管理操作拒否、他人のレビュー更新拒否、解除後401。
- `backend-go/internal/adapter/oauthserver/flow_test.go`: 読み取りトークンの書き込み拒否、解除、PKCE、CIMD。
- `backend-go/internal/usecase/oauth_consent_test.go`: 他人の許可を再利用しない、自分の接続だけを解除。
- `frontend/src/domains/oauth/components/ConnectedApps.test.tsx`: 解除の確認と失敗表示。

## 公開案内の内容

ClaudeのカスタムコネクタとClaude Codeの設定を追加。
参照: [Claude公式ヘルプ](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp)、[Claude Code公式MCPガイド](https://code.claude.com/docs/en/mcp)。
Claude Codeは `.mcp.json` の `oauth.scopes` で閲覧権限を明示し、投稿時に書き込みを追加する。
ページから確認状況・確認日・CLIヘルプ確認の記述を除去。接続制約はトラブル対処に残す。
