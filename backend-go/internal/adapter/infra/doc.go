// Package infra は infrastructure に関する関心事（環境変数からの設定、database pool の構築、
// JWT の発行/検証、パスワードのハッシュ化、メールの送信）を保持する。
//
// # メール送信の腐敗防止層
//
// このパッケージは、外部のメール送信プロバイダー（SMTP。将来は Resend の API など）に対する
// 腐敗防止層である。プロバイダーの形式を知るのは、ここだけである。
//
//   - usecase は、ドメインの意図（usecase.Mailer の SendSignupConfirmation・SendAlreadyRegistered。
//     宛先・リンクの URL・有効期間・冪等キーのような、意味のある値）だけを渡す。件名・本文・書式は渡さない
//   - 文面（件名・本文・有効期間の言い回し）は、ここ（mail_templates.go）が組み立てる。文面を変える、
//     日本語を併記する、といった変更で、usecase を触らなくてよい
//   - SMTP のプロトコル・MIME・接続の保護の方式（暗黙の TLS・STARTTLS）・応答コードは、SMTPMailer が
//     受け止め、外へ出さない。プロバイダーのエラーは、domain の言葉（一時的な失敗・恒久的な失敗。
//     domain.MailFailure）に翻訳して、送信の記録（domain.MailDeliveries）に残す
//   - 非同期の送信（AsyncMailer）は、冪等キーで記録を作り、同じ要求はメールを 1 通しか出さない。送信の
//     失敗・遅延・キューの満杯は、呼び出し側（signup の応答）には見えない
//
// usecase と domain が、net/smtp・MIME・プロバイダーの用語を import・参照しないことは、構造検査
// （usecase の TestPersistenceInterfaceNaming）で固定している。
package infra
