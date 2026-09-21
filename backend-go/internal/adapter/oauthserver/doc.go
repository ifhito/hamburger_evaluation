// Package oauthserver は、OAuth の認可サーバーを、認可ライブラリ(ory/fosite)の上に組み立てる adapter である。
//
// AI アプリ(クライアント)が、利用者の許可を得て API を使うための許可の証(トークン)を発行する。
// 認可コード + PKCE の流れ、トークンの署名・期限・入れ替え、再利用の検知などのプロトコルの細部は、
// ライブラリが担う。何を許すか(範囲・宛先)、いつまで有効か、どの URL から取得してよいかといった
// 業務上の判断は domain が持ち、ここはそれをライブラリの設定と保存先に結び付けるだけである。
// ライブラリの型は、このパッケージの外に出さない(usecase・handler はライブラリを知らない)。
//
// 保存先(storage.go)は、ライブラリが要求する保存の契約を、domain の書き込みオブジェクト
// (OAuthTokenSessions)と usecase の読み取りの窓口(OAuthTokenSessionQuery)で実装する。
package oauthserver
