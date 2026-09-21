package domain

// レビューの写真を受け付ける規則(大きさの上限)。API は、これらの値を GET /meta で frontend に伝え、
// 上限を超えたときのエラーメッセージにも使う。frontend は、値を自分では持たず、GET /meta に従って、
// 送る前に縮小する。写真の解釈(デコード・縮小・向きの補正・形式の判定)は internal/photo が行い、
// ここの値を使う。メモリの保護(デコード後の大きさの上限・同時に処理する件数)や、保存するときの
// 画質のような、実装上の調整値は、業務の規則ではないので、internal/photo に置く。
const (
	// MaxPhotoBytes は、アップロードできる写真のファイルの大きさの上限(バイト。5 MiB)である。
	MaxPhotoBytes int64 = 5 << 20
	// MaxPhotoEdge は、保存する写真の長辺の上限(ピクセル)である。これより大きい写真は、縮小して保存する
	// (小さい写真は拡大しない)。frontend は、この値(GET /meta)を目安に、送る前に縮小する。
	MaxPhotoEdge = 1600
	// MaxPhotoDimension は、写真の横・縦の上限(ピクセル)である。画像のヘッダーが宣言する大きさで判定する。
	MaxPhotoDimension = 10000
	// MaxPhotoPixels は、写真の画素数(横 × 縦)の上限である。実際のカメラの出力(24 メガピクセル)まで受け付ける。
	MaxPhotoPixels = 24_000_000
)
