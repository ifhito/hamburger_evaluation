# Phase 3: resend

- 計測日: 2026-09-22 00:00
- SMTP: `smtp.resend.com:587`(`starttls`)
- 差出人: `onboarding@resend.dev`
- 宛先: 実行ごとに + エイリアスで一意にしている(記録時は伏せる)

## 結果

| 項目 | 結果 |
|---|---|
| `POST /signup` | 202 |
| `mail_deliveries.status` | `failed` |
| 送信までの時間 | 3 秒 |
| 失敗の種類 | `permanent` |
| 理由 | `smtp: end data: 550 "You can only send testing emails to your own email address (hito01010101@gmail.com). To send emails to other recipients, please verify a domain at resend.com/domains, and change t` |

## 冪等性

同じアドレスで 2 回サインアップした。

| 項目 | 結果 |
|---|---|
| 2 回目の `POST /signup` | 202 |
| `mail_deliveries` の増加 | 1 行 |

2 回目は `already_registered` の通知になる想定(種別が違うので行は増える)。

## 到達できないときの挙動

ポート 47777(確実に閉じている)に向けて送らせた。2525 は Mailjet などが
正規に受け付けるため、遮断の模擬には使えない。

| 項目 | 結果 |
|---|---|
| `status` | `failed` |
| `failure_kind` | `temporary` |
| 理由 | `smtp: connect: dial tcp 54.205.195.44:47777: i/o timeout` |

`temporary` なら再試行で直りうる、`permanent` なら直らないと判定されている。

## 受信の確認(手で記録する)

| 項目 | 結果 |
|---|---|
| 受信箱に届いたか | (記入) |
| 送信から受信までの体感 | (記入) |
| 迷惑メールに入ったか | (記入) |
| 差出人の表示 | (記入) |

## SPF / DKIM の設定

| 項目 | 結果 |
|---|---|
| 独自ドメインが要るか | (記入) |
| DNS レコードの本数 | (記入) |
| 反映までの時間 | (記入) |

## ハマった点

(記入)
