package usecase

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ErrGoogleFlowMissing は、Google から戻ってきた要求に対応する、進行中の手続き(手続きを始めたブラウザの cookie に
// 封じた値)がないことを表す。この場合は、何も保存せずに、失敗として扱う(手続きを始めていない要求で、DB に書き込ませない)。
var ErrGoogleFlowMissing = errors.New("google login flow is missing")

// googleDiscardExpiredLimit は、手続きのたびに日和見的に削除する、期限切れの「画面へ渡すコード」の最大件数である
// (専用のバックグラウンドジョブは作らない)。
const googleDiscardExpiredLimit = 20

// GoogleFlowSecrets は、1 回のサインインの手続きの間だけ使う、秘密の値である。手続きを始めるときに作られ、
// 戻ってきたときの確認に使う。利用者の画面(SPA)には渡さず、ブラウザの cookie に封じて持ち回る。
type GoogleFlowSecrets struct {
	// State は、戻ってきた要求が、こちらが始めた手続きのものかを確かめる値(なりすましの防止)。
	State string
	// Nonce は、ID トークンが、この手続きのために発行されたことを確かめる値(使い回しの防止)。
	Nonce string
	// Verifier は、認可コードの横取りを防ぐ仕組み(PKCE)の、検証用の値である。
	Verifier string
}

// GoogleProvider は、Google とのやり取り(認可の URL の作成、認可コードの交換、ID トークンの検証)の窓口である。
// プロトコルの細部(署名の検証・鍵の取得など)は実装が担い、この契約の外へ出さない。実装が返すエラーの文言に、
// トークン・認可コード・秘密の鍵を含めてはならない。
type GoogleProvider interface {
	// Begin は、Google の認可の画面の URL と、この手続きの秘密の値を作る。
	Begin(ctx context.Context) (authURL string, secrets GoogleFlowSecrets, err error)
	// Complete は、認可コードを Google と交換し、ID トークンを検証して、Google が確かめた利用者の情報を返す。
	// 署名・発行者・宛先・有効期限・nonce の検証に失敗したときは、エラーを返す。
	Complete(ctx context.Context, code string, secrets GoogleFlowSecrets) (domain.ExternalIdentity, error)
}

// IdentityQuery は、外部のサービスのアカウントとの結び付きを読む、読み取り専用の窓口である(利用者の読み取りは
// UserQuery に置く)。書き込みのメソッドは置かない(書き込みは domain.UserIdentities を通す)。
type IdentityQuery interface {
	// GetIdentityByProviderUserID は、provider の subject に結び付いた記録を返す。なければ(wrap された)
	// domain.ErrIdentityNotFound を返す。
	GetIdentityByProviderUserID(ctx context.Context, provider, subject string) (domain.UserIdentity, error)
	// ListIdentitiesByUser は、利用者に結び付いた記録を、結び付けた順に返す。
	ListIdentitiesByUser(ctx context.Context, userID string) ([]domain.UserIdentity, error)
}

// LoginHandoffQuery は、画面へ渡すコードの中身を読む、読み取り専用の窓口である。UnitOfWork の中では、
// トランザクションに結び付いた実装が渡されるので、同じトランザクションでロックした行を、そのまま読める。
// 書き込みのメソッドは置かない(書き込みは domain.LoginHandoffs を通す)。
type LoginHandoffQuery interface {
	// GetLoginHandoffByCodeHash は、codeHash の、期限内のコードの中身を返す。期限切れ・存在しない・
	// すでに使われた(削除された)ものは、(wrap された)domain.ErrLoginHandoffInvalid を返す。
	GetLoginHandoffByCodeHash(ctx context.Context, codeHash string) (domain.LoginHandoff, error)
}

// GoogleFlow は、手続きを始めたときに決まり、終わるまで持ち回る値である。利用者の画面には渡さず、ブラウザの
// cookie に封じる。
type GoogleFlow struct {
	Secrets GoogleFlowSecrets
	// ReturnTo は、手続きのあとに戻る先である(domain.SanitizeReturnTo を通ったもの。空は既定の画面)。
	ReturnTo string
	// LinkUserID は、結び付けの手続きのとき、結び付ける利用者の ID である。サインインの手続きでは空である。
	LinkUserID string
}

// GoogleBegin は、手続きの開始の結果である。
type GoogleBegin struct {
	// AuthURL は、利用者を送る、Google の認可の画面の URL である。
	AuthURL string
	Flow    GoogleFlow
}

// GoogleCallback は、Google から戻ってきた要求の内容である。
type GoogleCallback struct {
	Code  string
	State string
	// ProviderError は、Google が付けてきた error(利用者が拒否した、など)。空でなければ、手続きは失敗である。
	ProviderError string
}

// GoogleRedeemed は、「画面へ渡すコード」を使った結果である。
type GoogleRedeemed struct {
	Outcome domain.LoginHandoffOutcome
	// User と Token は、Outcome がサインインの成功(domain.OutcomeSignedIn)のときだけ入る。Token は、この時点で発行する。
	User  domain.User
	Token string
	// ReturnTo は、手続きのあとに戻る先である(空は既定の画面)。
	ReturnTo string
}

// GoogleIdentityView は、プロフィールに見せる、外部のサービスとの結び付きである。
type GoogleIdentityView struct {
	Provider  string
	Email     string
	CreatedAt time.Time
	// CanUnlink は、解除してよいか(解除したあとも、サインインする方法が残るか。domain.CanUnlinkIdentity)である。
	CanUnlink bool
}

// GoogleLogins は、Google のアカウントでのサインイン・新規登録・結び付けの use case を実装する。読み取りは
// query、書き込みは domain の書き込みオブジェクトだけを通し、repository には依存しない。新規登録(ユーザーの作成と
// 結び付きの記録)は、2 つの集約を更新するので、UnitOfWork の中で組み立てる。
//
// 手続きの結果(成功・拒否・重複など)は、すべて「画面へ渡すコード」に入れて、画面へ渡す。画面は、そのコードを API と
// 交換して、結果を受け取る(Redeem)。ログインの証(JWT)は、URL には載せず、交換の応答でだけ返す。
type GoogleLogins struct {
	provider   GoogleProvider
	query      IdentityQuery
	users      UserQuery
	uow        UnitOfWork
	handoffs   *domain.LoginHandoffs
	identities *domain.UserIdentities
	issuer     TokenIssuer
}

// NewGoogleLogins は GoogleLogins を組み立てる。
func NewGoogleLogins(provider GoogleProvider, query IdentityQuery, users UserQuery, uow UnitOfWork, handoffs *domain.LoginHandoffs, identities *domain.UserIdentities, issuer TokenIssuer) *GoogleLogins {
	return &GoogleLogins{provider: provider, query: query, users: users, uow: uow, handoffs: handoffs, identities: identities, issuer: issuer}
}

// Begin は、サインインの手続きを始める。returnTo は、domain.SanitizeReturnTo で確かめて、アプリの中のパス
// でなければ、既定の画面(空)にする。
func (g *GoogleLogins) Begin(ctx context.Context, returnTo string) (GoogleBegin, error) {
	return g.begin(ctx, domain.SanitizeReturnTo(returnTo), "")
}

// BeginLink は、**ログイン済みの利用者**の、結び付けの手続きを始める。結び付ける利用者は、呼び出した本人(viewer。
// 認証済みの要求から決まる)で、手続きの秘密と一緒に、そのブラウザの cookie に封じられる。開始の URL やコードで、
// 別のブラウザに利用者を伝える経路は作らない(別のブラウザで開かせて、被害者の Google を攻撃者のアカウントに
// 結び付ける攻撃を防ぐため。手続きは、始めたブラウザの cookie がなければ、戻ってきても失敗する)。
func (g *GoogleLogins) BeginLink(ctx context.Context, viewer domain.User, returnTo string) (GoogleBegin, error) {
	return g.begin(ctx, domain.SanitizeReturnTo(returnTo), viewer.ID)
}

func (g *GoogleLogins) begin(ctx context.Context, returnTo, linkUserID string) (GoogleBegin, error) {
	authURL, secrets, err := g.provider.Begin(ctx)
	if err != nil {
		return GoogleBegin{}, fmt.Errorf("begin google login: %w", err)
	}
	return GoogleBegin{AuthURL: authURL, Flow: GoogleFlow{Secrets: secrets, ReturnTo: returnTo, LinkUserID: linkUserID}}, nil
}

// Complete は、Google から戻ってきた要求を処理し、結果を入れた「画面へ渡すコード」と、そのコードを使える相手を
// 確かめる「結び付けの値」(どちらも平文)を返す。手続きの結果は、成功も失敗も、すべてこのコードで画面へ渡す。
// 結び付けの値は、手続きを始めたブラウザの cookie にだけ持たせる(コードだけでは、交換できない。Redeem)。
// 進行中の手続きがない(flow が空)ときは、何も保存せず、ErrGoogleFlowMissing を返す。次の順に確かめる: 手続きを始めた側と state が一致するか → 認可コードを
// 交換して ID トークンを検証(署名・発行者・宛先・有効期限・nonce)→ メールが確認済みか。
//
// サインインの手続きでは、識別に Google の sub を使う。sub が結び付いていればサインイン、結び付いておらず同じメールの
// 利用者がいなければ新規登録、同じメールの利用者がいれば、自動では結び付けず、案内の結果(domain.OutcomeAccountExists)にする。
// 結び付けの手続きでは、その利用者に sub を結び付ける。
func (g *GoogleLogins) Complete(ctx context.Context, flow GoogleFlow, cb GoogleCallback) (domain.IssuedLoginHandoff, error) {
	if flow.Secrets.State == "" {
		return domain.IssuedLoginHandoff{}, ErrGoogleFlowMissing
	}
	outcome, userID := g.resolve(ctx, flow, cb)
	issued, err := g.handoffs.Issue(ctx, outcome, userID, flow.ReturnTo)
	if err != nil {
		return domain.IssuedLoginHandoff{}, fmt.Errorf("complete google login: %w", err)
	}
	if _, err := g.handoffs.DiscardExpired(ctx, googleDiscardExpiredLimit); err != nil {
		log.Printf("google login: discard expired handoffs: %v", err)
	}
	return issued, nil
}

// resolve は、戻ってきた要求の結果(種類と、利用者を伴うときはその ID)を決める。
func (g *GoogleLogins) resolve(ctx context.Context, flow GoogleFlow, cb GoogleCallback) (domain.LoginHandoffOutcome, string) {
	if cb.ProviderError != "" || cb.Code == "" ||
		subtle.ConstantTimeCompare([]byte(cb.State), []byte(flow.Secrets.State)) != 1 {
		return domain.OutcomeFailed, ""
	}
	ident, err := g.provider.Complete(ctx, cb.Code, flow.Secrets)
	if err != nil {
		log.Printf("google login: verify identity: %v", err)
		return domain.OutcomeFailed, ""
	}
	if err := ident.Validate(); err != nil {
		log.Printf("google login: reject identity: %v", err)
		return domain.OutcomeFailed, ""
	}
	if flow.LinkUserID != "" {
		return g.link(ctx, flow.LinkUserID, ident)
	}
	return g.signIn(ctx, ident)
}

func (g *GoogleLogins) signIn(ctx context.Context, ident domain.ExternalIdentity) (domain.LoginHandoffOutcome, string) {
	if outcome, userID, done := g.signInLinked(ctx, ident); done {
		return outcome, userID
	}

	// 結び付きがない: 同じメールの利用者がいれば、自動では結び付けない(他人のアカウントへの侵入を防ぐ)。
	if _, err := g.users.GetActiveUserByEmailIgnoreCase(ctx, ident.Email); err == nil {
		return domain.OutcomeAccountExists, ""
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		log.Printf("google login: get user by email: %v", err)
		return domain.OutcomeFailed, ""
	}

	var user domain.User
	err := g.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		user, err = tx.Users.Create(ctx, domain.CreateUserParams{
			Email:    ident.Email,
			Username: domain.UsernameFromProfile(ident.Name),
			// パスワードでサインインする方法は持たない(明示する)。新しいユーザーは、決して admin にならない。
			Passwordless: true,
			Admin:        false,
		})
		if err != nil {
			return err
		}
		_, err = tx.UserIdentities.Create(ctx, domain.CreateUserIdentityParams{
			UserID: user.ID, Provider: ident.Provider, ProviderUserID: ident.ProviderUserID, Email: ident.Email,
		})
		return err
	})
	switch {
	case err == nil:
		return domain.OutcomeSignedIn, user.ID
	case errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrIdentityTaken):
		// 同じ Google アカウントの初回のサインインが並行して、先に作られた(負けた側)ときは、結び付きを引き直して、
		// サインインさせる。「メールが使われている」と案内すると、パスワードを持たない本人のアカウントなのに、
		// パスワードでのサインインを勧めてしまう。
		if outcome, userID, done := g.signInLinked(ctx, ident); done {
			return outcome, userID
		}
		if errors.Is(err, domain.ErrEmailTaken) {
			// 確認から作成までの間に、別のメールの利用者ができた(または、退会済みのメール)。
			return domain.OutcomeAccountExists, ""
		}
		log.Printf("google login: create user: %v", err)
		return domain.OutcomeFailed, ""
	default:
		log.Printf("google login: create user: %v", err)
		return domain.OutcomeFailed, ""
	}
}

// signInLinked は、外部のアカウント(sub)がすでに利用者に結び付いているとき、その利用者としてのサインインの
// 結果を返す(done が true)。結び付きがなければ、done は false である。退会済みの利用者の結び付きでは、
// サインインさせない(失敗として返す)。
func (g *GoogleLogins) signInLinked(ctx context.Context, ident domain.ExternalIdentity) (outcome domain.LoginHandoffOutcome, userID string, done bool) {
	linked, err := g.query.GetIdentityByProviderUserID(ctx, ident.Provider, ident.ProviderUserID)
	switch {
	case err == nil:
		user, err := g.users.GetActiveUserByID(ctx, linked.UserID)
		if err != nil {
			if !errors.Is(err, domain.ErrUserNotFound) {
				log.Printf("google login: get linked user: %v", err)
			}
			return domain.OutcomeFailed, "", true
		}
		return domain.OutcomeSignedIn, user.ID, true
	case !errors.Is(err, domain.ErrIdentityNotFound):
		log.Printf("google login: get identity: %v", err)
		return domain.OutcomeFailed, "", true
	}
	return "", "", false
}

func (g *GoogleLogins) link(ctx context.Context, userID string, ident domain.ExternalIdentity) (domain.LoginHandoffOutcome, string) {
	if _, err := g.users.GetActiveUserByID(ctx, userID); err != nil {
		if !errors.Is(err, domain.ErrUserNotFound) {
			log.Printf("google link: get user: %v", err)
		}
		return domain.OutcomeFailed, ""
	}
	existing, err := g.query.GetIdentityByProviderUserID(ctx, ident.Provider, ident.ProviderUserID)
	switch {
	case err == nil:
		if existing.UserID == userID {
			return domain.OutcomeLinked, userID // すでに結び付いている(同じ操作の繰り返し)
		}
		return domain.OutcomeIdentityTaken, ""
	case !errors.Is(err, domain.ErrIdentityNotFound):
		log.Printf("google link: get identity: %v", err)
		return domain.OutcomeFailed, ""
	}
	_, err = g.identities.Create(ctx, domain.CreateUserIdentityParams{
		UserID: userID, Provider: ident.Provider, ProviderUserID: ident.ProviderUserID, Email: ident.Email,
	})
	switch {
	case err == nil:
		return domain.OutcomeLinked, userID
	case errors.Is(err, domain.ErrIdentityTaken):
		return domain.OutcomeIdentityTaken, ""
	case errors.Is(err, domain.ErrIdentityAlreadyLinked):
		return domain.OutcomeAlreadyLinked, ""
	default:
		log.Printf("google link: create identity: %v", err)
		return domain.OutcomeFailed, ""
	}
}

// Redeem は、「画面へ渡すコード」(平文)を 1 回だけ使って、手続きの結果を返す。コードと一緒に、手続きを終えた
// ブラウザだけが持つ「結び付けの値」(binder。Complete が返したもの)を渡さなければならない。**結び付けの値が
// 合わない・ないときは、コードを使えず、コードも消費しない**(コードだけを別のブラウザへ持ち込んでも交換できない。
// 攻撃者が自分の Google で進めた手続きの URL を、被害者に踏ませて、被害者を攻撃者のアカウントでログインさせる、
// ログイン CSRF を防ぐ。しかも、被害者の試みで、攻撃者でない本来のブラウザのコードが、使えなくならない)。
// 期限切れ・存在しない・使用済み・結び付けの値が合わないは、(wrap された)domain.ErrLoginHandoffInvalid を返す
// (区別できない)。サインインの成功のときは、この時点で、ログインの証(JWT)を発行する。
//
// 手順は、1 つのトランザクションの中で「コードをロックする → 内容を読む → 利用者の取得・トークンの発行 → コードを
// 削除する」の順に行う。**後続の処理が失敗したら全体を取り消す**ので、DB の一時的なエラーやトークンの発行の失敗で
// 500 になっても、コードは期限まで有効なままで、画面が同じコードで再試行できる(先に消してしまうと、利用者は、
// 最初からやり直すことになる)。先頭でロックするので、同じコードの並行する交換は 1 件ずつに直列になり、
// 2 件目は、行が消えているのを見て、無効になる(成功するのは 1 回だけ)。
func (g *GoogleLogins) Redeem(ctx context.Context, rawCode, binder string) (GoogleRedeemed, error) {
	var res GoogleRedeemed
	err := g.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.LoginHandoffs.Lock(ctx, rawCode); err != nil {
			return err
		}
		h, err := tx.PendingHandoff.GetLoginHandoffByCodeHash(ctx, domain.HashLoginHandoffCode(rawCode))
		if err != nil {
			return err
		}
		if !h.BoundTo(binder) {
			return fmt.Errorf("get login handoff: %w", domain.ErrLoginHandoffInvalid) // 全体を取り消す(コードは消費しない)
		}
		res = GoogleRedeemed{Outcome: h.Outcome, ReturnTo: h.ReturnTo}
		if h.Outcome == domain.OutcomeSignedIn {
			// トランザクションの接続で読む(プールから、もう 1 つ接続を取ると、並行する交換で、接続を取り合って止まる)。
			user, err := tx.UserReads.GetActiveUserByID(ctx, h.UserID)
			switch {
			case err == nil:
				token, err := g.issuer.Issue(user.ID)
				if err != nil {
					return fmt.Errorf("issue token: %w", err)
				}
				res.User, res.Token = user, token
			case errors.Is(err, domain.ErrUserNotFound):
				// コードを作ってから、交換までの間に退会した。サインインさせず、失敗として返す(コードは使い切る)。
				res.Outcome = domain.OutcomeFailed
			default:
				return fmt.Errorf("get user: %w", err)
			}
		}
		return tx.LoginHandoffs.Discard(ctx, h.ID)
	})
	if err != nil {
		return GoogleRedeemed{}, fmt.Errorf("redeem google login: %w", err)
	}
	return res, nil
}

// ListIdentities は、ログイン済みの利用者の、外部のサービスとの結び付きを返す。
func (g *GoogleLogins) ListIdentities(ctx context.Context, viewer domain.User) ([]GoogleIdentityView, error) {
	identities, err := g.query.ListIdentitiesByUser(ctx, viewer.ID)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	hasPassword, err := g.users.GetActiveUserHasPassword(ctx, viewer.ID)
	if err != nil {
		return nil, fmt.Errorf("list identities: has password: %w", err)
	}
	views := make([]GoogleIdentityView, 0, len(identities))
	for _, id := range identities {
		views = append(views, GoogleIdentityView{
			Provider:  id.Provider,
			Email:     id.Email,
			CreatedAt: id.CreatedAt,
			CanUnlink: domain.CanUnlinkIdentity(hasPassword, len(identities)) == nil,
		})
	}
	return views, nil
}

// Unlink は、ログイン済みの利用者の、provider の結び付きを解除する。解除したあとに、サインインする方法が
// なくなる(パスワードがなく、ほかの結び付きもない)ときは、(wrap された)domain.ErrCannotUnlinkIdentity を、
// 結び付きがなければ(wrap された)domain.ErrIdentityNotFound を返す。
func (g *GoogleLogins) Unlink(ctx context.Context, viewer domain.User, provider string) error {
	identities, err := g.query.ListIdentitiesByUser(ctx, viewer.ID)
	if err != nil {
		return fmt.Errorf("unlink identity: list: %w", err)
	}
	found := false
	for _, id := range identities {
		found = found || id.Provider == provider
	}
	if !found {
		return fmt.Errorf("unlink identity: %w", domain.ErrIdentityNotFound)
	}
	hasPassword, err := g.users.GetActiveUserHasPassword(ctx, viewer.ID)
	if err != nil {
		return fmt.Errorf("unlink identity: has password: %w", err)
	}
	if err := domain.CanUnlinkIdentity(hasPassword, len(identities)); err != nil {
		return err
	}
	return g.identities.Discard(ctx, viewer.ID, provider)
}
