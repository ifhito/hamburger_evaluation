package photo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"time"

	"github.com/gen2brain/heic"
)

// HEIC(iPhone などの既定の写真形式)の受け付けに関する上限と道具。
//
// HEIC は「HEIF」というコンテナ(ISO のボックス構造)に、動画と同じ HEVC 圧縮の画像を入れた形式で、
// 標準ライブラリにはデコーダがない。デコーダには libheif を WASM(ブラウザ向けのバイナリ形式)に
// コンパイルして、純 Go の実行系(wazero)で動かすライブラリを使う。WASM の中で動くので、cgo が
// 要らず、デコーダの不具合が起きても、影響は WASM の中(仮想のメモリ)に閉じる。

const (
	// maxHEICPixels は、HEIC の宣言寸法の上限(ピクセル数)である。JPEG などの上限(2,400 万)より
	// 小さいのは、デコードのメモリが大きいため。実測では、HEIC のデコードは 1 ピクセルあたり約 31
	// バイトを使い(1,200 万画素で約 360 MiB、2,400 万画素で約 760 MiB)、コンテナの上限(1 GiB)に
	// 収まるように、1,600 万画素(約 500 MiB)までにしている。iPhone の標準の写真(約 1,200 万画素)は
	// 通る。
	maxHEICPixels = 16_000_000
)

// heicTimeout は、HEIC の待ち行列とデコードを合わせた時間の上限である(1,600 万画素のデコードは約 1 秒)。
// サーバーの書き込みの時間切れ(10 秒)より前に、こちらから 422 を返せるように 8 秒にしている。
// テストで短くできるように変数にしてある。
var heicTimeout = 8 * time.Second

// heicSem は、HEIC のデコードを同時に 1 件に制限する。1 件だけで最大約 500 MiB を使うので、
// 2 件が重なると、コンテナのメモリの上限を超えるおそれがある。JPEG などと共有する decodeSem
// (2 件)とは別に持ち、HEIC は「heicSem → decodeSem」の順に取る(取る順序を常に同じにして、
// 待ち合わせが循環しないようにする)。
var heicSem = make(chan struct{}, 1)

// heicDecode は HEIC を画像にデコードする関数である。テストで差し替えられるように変数にしてある。
var heicDecode = func(r io.Reader) (image.Image, error) { return heic.Decode(r) }

// ErrDimensionsTooLarge は、写真の宣言寸法(横・縦・画素数)が上限を超えることを表す。
// ErrUnsupportedImage の一種でもある(errors.Is で両方に一致する)ので、寸法だけを別の
// メッセージにしたい呼び出し側は、先にこちらを調べる。
var ErrDimensionsTooLarge = fmt.Errorf("%w: dimensions exceed the limit", ErrUnsupportedImage)

// WarmUp は HEIC のデコーダ(WASM の読み込みとコンパイル。約 0.3 秒)を先に済ませる。
// 起動時に別の goroutine で呼んでおくと、サーバーの起動を遅らせず、最初の HEIC の投稿だけが
// 遅くなることもない(先に投稿が来ても、コンパイルの終了を待つだけで、二重には行われない)。
// 初期化の失敗(panic)は、ここでは握りつぶす。投稿のたびに同じ失敗が起きるが、decodeHEIC が受け止めて
// 1 件ずつ「対応しない画像」として断るので、起動時にプロセスを落とすよりよい。
func WarmUp() {
	defer func() { _ = recover() }()
	heic.Init()
}

// heifKind は、ファイルの先頭の ftyp ボックス(ファイルの種類を宣言する部分)から見た種類である。
type heifKind int

const (
	heifNotSupported heifKind = iota // 対応しない(AVIF・画像の連続(動画)など)
	heifHEIC                         // HEVC の静止画(HEIC)
)

// heifInfo は、HEIF のコンテナのヘッダーから、純 Go で読み取った情報である。デコーダを呼ぶ前に
// 寸法を確かめるために使う。
type heifInfo struct {
	kind heifKind
	// 宣言された画像の寸法(ispe ボックス)のうち、最大の横・縦・画素数。1 枚の画像に、サムネイルなど
	// 複数の宣言があるので、最大値で判断する。
	maxWidth, maxHeight uint32
	maxPixels           uint64
}

// looksLikeHEIF は、先頭の 8 バイト目からの 4 バイトが "ftyp" かどうかを返す(ISO のボックス構造の
// ファイルに共通の目印)。http.DetectContentType は HEIC を判別できないので、自前で見る。
func looksLikeHEIF(head []byte) bool {
	return len(head) >= 12 && string(head[4:8]) == "ftyp"
}

// heicBrands は、HEVC の静止画として扱うブランド(ftyp が名乗る種類)である。mif1 は、HEIC と AVIF が
// 共通で持つ名前なので、avif・avis を名乗るものは、先に対応しないものとして除く。
var heicBrands = map[string]bool{"heic": true, "heix": true, "heim": true, "heis": true, "hevc": true, "hevx": true, "mif1": true}

// parseHEIF は、HEIF ファイルのコンテナを、デコーダを使わずに、境界を確かめながら読む。読むのは、
// ftyp(種類)と、meta → iprp → ipco の中にある ispe(画像の寸法)だけである。壊れた・細工された
// ファイルでも、範囲外を読まず、ボックスの数にも上限を置いて、少ない時間で終わる。
func parseHEIF(data []byte) (heifInfo, error) {
	var info heifInfo
	const maxBoxes = 4096
	seen := 0

	ftyp, ok := nextBox(data, 0, len(data))
	if !ok || ftyp.typ != "ftyp" || ftyp.bodyEnd-ftyp.bodyStart < 8 {
		return info, errors.New("missing ftyp box")
	}
	isAVIF, isHEIC := false, false
	body := data[ftyp.bodyStart:ftyp.bodyEnd]
	// 先頭の 4 バイトが主なブランド、次の 4 バイトが版で、その後ろは互換ブランドが 4 バイトずつ並ぶ。
	for i := 0; i+4 <= len(body); i += 4 {
		if i == 4 {
			continue
		}
		switch b := string(body[i : i+4]); {
		case b == "avif" || b == "avis":
			isAVIF = true
		case heicBrands[b]:
			isHEIC = true
		}
	}
	if isAVIF || !isHEIC {
		return info, nil // kind は heifNotSupported のまま
	}

	// トップレベルのボックスを順に見て、meta を探す。
	var meta box
	found := false
	for off := ftyp.end; off < len(data) && seen < maxBoxes; seen++ {
		b, ok := nextBox(data, off, len(data))
		if !ok {
			return info, errors.New("malformed box")
		}
		if b.typ == "meta" {
			meta, found = b, true
			break
		}
		off = b.end
	}
	if !found || meta.bodyEnd-meta.bodyStart < 4 {
		return info, errors.New("missing meta box")
	}
	// meta は「フルボックス」で、本体の先頭に 4 バイト(版とフラグ)がある。
	ipco, ok := findChild(data, meta.bodyStart+4, meta.bodyEnd, []string{"iprp", "ipco"}, &seen, maxBoxes)
	if !ok {
		return info, errors.New("missing ipco box")
	}
	for off := ipco.bodyStart; off < ipco.bodyEnd && seen < maxBoxes; seen++ {
		b, ok := nextBox(data, off, ipco.bodyEnd)
		if !ok {
			return info, errors.New("malformed box in ipco")
		}
		if b.typ == "ispe" {
			// ispe もフルボックス: 4 バイトの版とフラグ、横 4 バイト、縦 4 バイト。
			if b.bodyEnd-b.bodyStart < 12 {
				return info, errors.New("short ispe box")
			}
			w := binary.BigEndian.Uint32(data[b.bodyStart+4:])
			h := binary.BigEndian.Uint32(data[b.bodyStart+8:])
			info.maxWidth = max(info.maxWidth, w)
			info.maxHeight = max(info.maxHeight, h)
			info.maxPixels = max(info.maxPixels, uint64(w)*uint64(h))
		}
		off = b.end
	}
	if info.maxPixels == 0 {
		return info, errors.New("missing ispe box")
	}
	info.kind = heifHEIC
	return info, nil
}

// box は、ISO のボックス 1 つ(先頭の「大きさ・種類」と本体)の位置である。
type box struct {
	typ                     string
	bodyStart, bodyEnd, end int
}

// nextBox は data の off 位置から、limit を超えない範囲で、ボックスの見出しを読む。大きさが
// 範囲に収まらない・見出しより小さいものは、壊れているものとして false を返す。
func nextBox(data []byte, off, limit int) (box, bool) {
	if off < 0 || limit > len(data) || limit-off < 8 {
		return box{}, false
	}
	size := uint64(binary.BigEndian.Uint32(data[off:]))
	typ := string(data[off+4 : off+8])
	header := 8
	switch size {
	case 1: // 大きさが 8 バイトの拡張の欄にある
		if limit-off < 16 {
			return box{}, false
		}
		size = binary.BigEndian.Uint64(data[off+8:])
		header = 16
	case 0: // 大きさ 0 は「ここから外側のボックスの終わりまで」
		size = uint64(limit - off)
	}
	if size < uint64(header) || size > uint64(limit-off) {
		return box{}, false
	}
	end := off + int(size)
	return box{typ: typ, bodyStart: off + header, bodyEnd: end, end: end}, true
}

// findChild は、start〜limit の範囲のボックスから、path の名前を順にたどって、最後のボックスを返す。
func findChild(data []byte, start, limit int, path []string, seen *int, maxBoxes int) (box, bool) {
	cur := box{bodyStart: start, bodyEnd: limit}
	for _, name := range path {
		found := false
		for off := cur.bodyStart; off < cur.bodyEnd && *seen < maxBoxes; *seen++ {
			b, ok := nextBox(data, off, cur.bodyEnd)
			if !ok {
				return box{}, false
			}
			if b.typ == name {
				cur, found = b, true
				break
			}
			off = b.end
		}
		if !found {
			return box{}, false
		}
	}
	return cur, true
}

// processHEIF は HEIC の写真を検証してデコードし、縮小した JPEG に変換する。
// 流れは、(1)コンテナのヘッダーから種類と寸法を読み、(2)寸法が上限を超えるものを、デコーダを呼ばずに
// 断り、(3)デコード(同時 1 件・時間制限つき)、(4)長辺 maxEdge に縮小、(5)JPEG に再エンコードする。
// 向きの補正は、HEIC の場合は要らない: 写真の向きは、コンテナの「回転」の情報(irot)で持たれ、
// デコーダがそれを適用した状態の画像を返すため。ここで、EXIF の向きを重ねて適用すると、二重に回転する。
func processHEIF(ctx context.Context, data []byte) (Processed, error) {
	info, err := parseHEIF(data)
	if err != nil {
		return Processed{}, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	if info.kind != heifHEIC {
		return Processed{}, fmt.Errorf("%w: unsupported HEIF brand", ErrUnsupportedImage)
	}
	if info.maxWidth > maxDimension || info.maxHeight > maxDimension || info.maxPixels > maxHEICPixels {
		return Processed{}, fmt.Errorf("%w: %dx%d declared", ErrDimensionsTooLarge, info.maxWidth, info.maxHeight)
	}
	img, err := decodeHEIC(ctx, data)
	if err != nil {
		return Processed{}, err
	}
	if b := img.Bounds(); b.Dx() <= 0 || b.Dy() <= 0 || uint64(b.Dx())*uint64(b.Dy()) > maxHEICPixels {
		return Processed{}, fmt.Errorf("%w: decoded size %dx%d", ErrUnsupportedImage, b.Dx(), b.Dy())
	}
	return encodeJPEG(shrink(img))
}

// decodeHEIC は、同時実行の枠を取ってから HEIC をデコードする。デコードは別の goroutine で行い、
// 時間切れ・キャンセルのときは、待たずに戻る(WASM のデコードは途中で止められない)。ただし、枠は
// デコードが実際に終わるまで手放さないので、時間切れのデコードが溜まって、メモリの上限を超えることは
// ない。デコーダの中の異常終了(panic)は、ここで受け止めて 1 件の失敗として扱い、プロセスは落とさない。
func decodeHEIC(parent context.Context, data []byte) (image.Image, error) {
	ctx, cancel := context.WithTimeout(parent, heicTimeout)
	defer cancel()

	select {
	case heicSem <- struct{}{}:
	case <-ctx.Done():
		return nil, decodeWaitError(parent, ctx)
	}
	select {
	case decodeSem <- struct{}{}:
	case <-ctx.Done():
		<-heicSem
		return nil, decodeWaitError(parent, ctx)
	}

	type result struct {
		img image.Image
		err error
	}
	done := make(chan result, 1)
	decode := heicDecode // goroutine を作る前に読んでおく(差し替え中のテストとの競合を避ける)
	go func() {
		defer func() { <-decodeSem; <-heicSem }()
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("%w: heic decoder panic: %v", ErrUnsupportedImage, r)}
			}
		}()
		img, err := decode(bytes.NewReader(data))
		if err != nil {
			err = fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
		}
		done <- result{img, err}
	}()
	select {
	case r := <-done:
		return r.img, r.err
	case <-ctx.Done():
		return nil, decodeWaitError(parent, ctx)
	}
}

// decodeWaitError は、待ち・デコードが打ち切られた理由を、呼び出し側の扱いに合わせて返す。
// 呼び出し側(リクエスト)のキャンセルは、そのまま返す(すでに相手がいない)。こちらの時間切れは、
// 「処理しきれない写真」として、対応しない写真(ErrUnsupportedImage)と同じ扱いにする。
func decodeWaitError(parent, ctx context.Context) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	return fmt.Errorf("%w: decode timed out", ErrUnsupportedImage)
}
