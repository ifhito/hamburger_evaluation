# Phase 2: b2

- 計測日: 2026-09-21 23:04
- エンドポイント: `https://s3.us-east-005.backblazeb2.com`
- バケット: `burger-image`

## 接続と Region の扱い

- `Region: "auto"` の PUT: **失敗**
- エラー: `operation error S3: PutObject, https response error StatusCode: 403, RequestID: 0aa566743838ae7b, HostID: adcpui2siblFvPncJbg4=, api error InvalidAccessKeyId: Malformed Access Key Id`

原因は上のエラーを参照。
FAILED: PutObject: operation error S3: PutObject, https response error StatusCode: 403, RequestID: 0aa566743838ae7b, HostID: adcpui2siblFvPncJbg4=, api error InvalidAccessKeyId: Malformed Access Key Id
exit status 1

## ハマった点

(記入)
