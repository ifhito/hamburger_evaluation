// Package googleauth は、usecase.GoogleProvider を、OpenID Connect の認可コードの流れ(PKCE つき)で実装する。
// 認可の URL の作成・認可コードの交換・ID トークンの検証(署名・発行者・宛先・有効期限・nonce)を担い、
// プロトコルの細部は、実績のあるライブラリ(github.com/coreos/go-oidc/v3 と golang.org/x/oauth2)に任せる。
//
// 返すエラーには、トークン・認可コード・秘密の鍵を含めない(ライブラリのエラーの文言は、そのまま返さず、
// 原因の種類だけを表す固定の文言にする)。
package googleauth
