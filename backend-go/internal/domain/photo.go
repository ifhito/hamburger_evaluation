package domain

// レビューの写真について、API・frontend と共有する上限。frontend は値を自分では持たず、GET /meta に従って、
// 送る前に縮小する。写真の解釈（デコード・縮小・向きの補正・形式の判定）は internal/photo が行う。
// デコードの前の寸法・画素数の検査（decompression bomb のガード）、デコード後のメモリの上限、同時に処理する件数、
// 保存するときの画質のような、コンテナのメモリやデコーダーの特性に応じて調整しうる実装上の値は、
// 業務の規則ではないので、internal/photo に置く。
const (
	// MaxPhotoBytes は、アップロードできる写真のファイルの大きさの上限(バイト。5 MiB)である。
	// 使う場所: GET /meta の photo.max_bytes、handler の受け付けの判定とその 422 のメッセージ
	// （readPhotoPart・msgPhotoTooLarge）、レビュー投稿の本文の上限の導出（middleware の
	// maxReviewRequestBodyBytes）。internal/photo は使わない。
	MaxPhotoBytes int64 = 5 << 20
	// MaxPhotoEdge は、保存する写真の長辺の上限(ピクセル)である。これより大きい写真は、縮小して保存する
	// (小さい写真は拡大しない)。使う場所: GET /meta の photo.max_edge（frontend が、送る前の縮小の目安にする）、
	// internal/photo の縮小。エラーメッセージには使わない。
	MaxPhotoEdge = 1600
)
