package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// discardExpiredLimit は、signup のたびに日和見的に削除する、期限切れの確認待ちの
// 最大件数である（専用のバックグラウンドジョブは作らない）。
const discardExpiredLimit = 20

// Mail は、送る 1 通のメールである（プレーンテキスト）。
type Mail struct {
	To      string
	Subject string
	Body    string
}

// Mailer は、メールを送る契約である（送信のみ）。実装は呼び出しを待たせない（非同期）ので、
// 送信の成否・遅延は呼び出し側に見えず、応答時間から登録の有無を推測されない。失敗は
// 実装がログに出す。
type Mailer interface {
	Send(mail Mail)
}

// SignupConfig は、signup の確認メールと確認トークンの設定である。
type SignupConfig struct {
	// BaseURL は、確認メールのリンクの生成元（frontend の URL。末尾に / を付けない）である。
	BaseURL string
	// TokenTTL は、確認トークンの有効期間である。
	TokenTTL time.Duration
}

// SignupInput は signup use case の入力である。PasswordConfirmation は
// 任意であり（リクエストにそのフィールドがなかった場合は nil）、存在する
// ときは、Rails の has_secure_password に合わせて Password と等しくなければ
// ならない。
type SignupInput struct {
	Username             string
	Email                string
	Password             string
	PasswordConfirmation *string
}

// validate は Rails parity の full message を返す。valid なら空である。
// メッセージは username、認証情報（domain.ValidateCredentials。email、password の順）、
// confirmation の順に並ぶ。登録の有無には依存しない検証だけを行う（「登録済み」を
// 示すエラーは返さない）。
func (in SignupInput) validate() []string {
	var msgs []string
	if in.Username == "" {
		msgs = append(msgs, "Username can't be blank")
	}
	msgs = append(msgs, domain.ValidateCredentials(in.Email, in.Password)...)
	if in.PasswordConfirmation != nil && *in.PasswordConfirmation != in.Password {
		msgs = append(msgs, "Password confirmation doesn't match Password")
	}
	return msgs
}

// Signups は、メール確認つきの signup の use case を実装する。アカウントは、確認メールの
// リンクを開いて初めて作られる。読み取りは query、書き込みは domain の書き込みオブジェクト
// （domain.SignupVerifications）だけを通し、repository には依存しない。
type Signups struct {
	query         UserQuery
	verifications *domain.SignupVerifications
	hasher        PasswordHasher
	mailer        Mailer
	issuer        TokenIssuer
	cfg           SignupConfig
}

func NewSignups(query UserQuery, verifications *domain.SignupVerifications, hasher PasswordHasher, mailer Mailer, issuer TokenIssuer, cfg SignupConfig) *Signups {
	return &Signups{query: query, verifications: verifications, hasher: hasher, mailer: mailer, issuer: issuer, cfg: cfg}
}

// Request は signup の入力を検証し、確認メールを送る手配をする。**登録済みの email でも
// 未登録の email でも、成功したときは同じ結果（nil）を返す**ので、応答から登録の有無を
// 判別できない（アカウント列挙の防止）。検証の失敗は *domain.ValidationError で、登録の
// 有無には依存しない。
//
// どの分岐でも、bcrypt のハッシュ計算を行ってから分岐する（応答時間の差を bcrypt 1 回分の
// 揺らぎにとどめる）。メールの送信は Mailer が非同期に行う。登録済みなら、状態を変えず、
// 「すでに登録済み」の通知を送る（確認リンクは含まない）。未登録なら、確認待ちを最新の入力で
// 置き換えて確認メールを送る。前回の送信から domain.SignupResendInterval 以内なら、
// 確認待ちを変えず、メールも送らない。
func (s *Signups) Request(ctx context.Context, input SignupInput) error {
	if msgs := input.validate(); len(msgs) > 0 {
		return &domain.ValidationError{Messages: msgs}
	}
	digest, err := s.hasher.Hash(input.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	s.discardExpired(ctx)

	_, err = s.query.GetActiveUserByEmail(ctx, input.Email)
	switch {
	case err == nil:
		s.mailer.Send(s.alreadyRegisteredMail(input.Email))
		return nil
	case !errors.Is(err, domain.ErrUserNotFound):
		return fmt.Errorf("get user by email: %w", err)
	}

	token, err := domain.NewSignupToken()
	if err != nil {
		return err
	}
	accepted, err := s.verifications.Create(ctx, domain.CreateSignupVerificationParams{
		Email:          input.Email,
		Username:       input.Username,
		PasswordDigest: digest,
		TokenHash:      token.Hash,
		TTL:            s.cfg.TokenTTL,
	})
	if err != nil {
		return fmt.Errorf("create signup verification: %w", err)
	}
	if accepted {
		s.mailer.Send(s.confirmationMail(input.Email, token.Raw))
	}
	return nil
}

// Confirm は、確認メールのリンクの平文トークンでアカウントを作成し、新しい認証トークンとともに
// そのユーザーを返す（従来の signup の成功と同じ結果）。期限切れ・存在しない・改ざん・使用済みの
// トークンは、確認までの間に同じ email のユーザーが作られていた場合も含め、すべて
// domain.ErrSignupTokenInvalid になる（区別できない）。
func (s *Signups) Confirm(ctx context.Context, rawToken string) (domain.User, string, error) {
	user, err := s.verifications.Confirm(ctx, rawToken)
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return domain.User{}, "", fmt.Errorf("confirm signup: %w", domain.ErrSignupTokenInvalid)
		}
		return domain.User{}, "", fmt.Errorf("confirm signup: %w", err)
	}
	token, err := s.issuer.Issue(user.ID)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("issue token: %w", err)
	}
	return user, token, nil
}

// discardExpired は、期限切れの確認待ちを上限つきで削除する。失敗しても signup の結果には
// 影響させない（掃除は日和見的で、次の signup で再び試みられる）。
func (s *Signups) discardExpired(ctx context.Context) {
	if _, err := s.verifications.DiscardExpired(ctx, discardExpiredLimit); err != nil {
		log.Printf("signup: discard expired verifications: %v", err)
	}
}

// confirmationMail は、確認リンクつきのメールを作る。本文には利用者が入力した値（username など）を
// 入れない（第三者の email で signup した人が、本文に任意の文言を差し込めないようにするため）。
func (s *Signups) confirmationMail(to, rawToken string) Mail {
	link := s.cfg.BaseURL + "/signup/confirm?token=" + url.QueryEscape(rawToken)
	return Mail{
		To:      to,
		Subject: "Confirm your email address",
		Body: "Please confirm your email address to finish creating your account:\n\n" +
			link + "\n\n" +
			"This link expires in " + humanizeDuration(s.cfg.TokenTTL) + ".\n" +
			"If you didn't sign up, you can safely ignore this email.\n",
	}
}

// alreadyRegisteredMail は、登録済みの email への通知を作る。確認トークンは含めず、
// ログイン画面へのリンクだけを入れる。
func (s *Signups) alreadyRegisteredMail(to string) Mail {
	return Mail{
		To:      to,
		Subject: "You already have an account",
		Body: "Someone tried to sign up with this email address, but an account already exists.\n\n" +
			"You can sign in here:\n\n" +
			s.cfg.BaseURL + "/signin\n\n" +
			"If this wasn't you, you can safely ignore this email.\n",
	}
}

// humanizeDuration は、有効期間を英語の短い言い回しにする（"24 hours"、"90 minutes"）。
func humanizeDuration(d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		hours := int(d / time.Hour)
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	minutes := int((d + time.Minute - 1) / time.Minute)
	if minutes <= 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}
