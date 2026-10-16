package infra

import (
	"fmt"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// mailMessage は、送る 1 通のメール（プレーンテキスト・UTF-8）である。件名・本文の組み立ては
// この package の中だけで行い、usecase・domain は知らない（メールの文面の変更や、日本語の併記で、
// usecase を触らなくてよいようにするため）。
type mailMessage struct {
	To      string
	Subject string
	Body    string
}

// renderSignupConfirmation は、確認リンクつきのメールの文面を作る。本文には、リンクと有効期間だけを
// 入れる。利用者が入力した値（username など）は、この意図の値に含まれないので、入れようがない
// （第三者の email で signup した人が、本文に任意の文言を差し込めないようにするため）。
// 言語は n.Lang で、日本語(domain.LangJA)のときだけ日本語、それ以外は英語である。
func renderSignupConfirmation(n usecase.SignupConfirmation) mailMessage {
	if n.Lang == domain.LangJA {
		return mailMessage{
			To:      n.To,
			Subject: "BurgerStack のメールアドレスの確認",
			Body: "BurgerStack のアカウントの作成を完了するには、次のリンクを開いて、メールアドレスを確認してください。\n\n" +
				n.ConfirmURL + "\n\n" +
				"このリンクの有効期間は " + humanizeDurationJA(n.ValidFor) + "です。\n" +
				"心当たりがなければ、このメールは無視してください。\n",
		}
	}
	return mailMessage{
		To:      n.To,
		Subject: "Confirm your email address for BurgerStack",
		Body: "Please confirm your email address to finish creating your BurgerStack account:\n\n" +
			n.ConfirmURL + "\n\n" +
			"This link expires in " + humanizeDuration(n.ValidFor) + ".\n" +
			"If you didn't sign up, you can safely ignore this email.\n",
	}
}

// renderAlreadyRegistered は、「すでに登録済み」の通知の文面を作る。確認のリンク・トークンは
// 入れず、ログイン画面へのリンクだけを入れる。言語の選び方は renderSignupConfirmation と同じである。
func renderAlreadyRegistered(n usecase.AlreadyRegisteredNotice) mailMessage {
	if n.Lang == domain.LangJA {
		return mailMessage{
			To:      n.To,
			Subject: "BurgerStack のアカウントはすでにあります",
			Body: "このメールアドレスで BurgerStack への登録が試みられましたが、アカウントはすでにあります。\n\n" +
				"次のリンクからログインできます。\n\n" +
				n.SignInURL + "\n\n" +
				"心当たりがなければ、このメールは無視してください。\n",
		}
	}
	return mailMessage{
		To:      n.To,
		Subject: "You already have a BurgerStack account",
		Body: "Someone tried to sign up for BurgerStack with this email address, but an account already exists.\n\n" +
			"You can sign in here:\n\n" +
			n.SignInURL + "\n\n" +
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

// humanizeDurationJA は、有効期間を日本語の短い言い回しにする（"24 時間"、"90 分"）。
func humanizeDurationJA(d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%d 時間", int(d/time.Hour))
	}
	return fmt.Sprintf("%d 分", max(int((d+time.Minute-1)/time.Minute), 1))
}
