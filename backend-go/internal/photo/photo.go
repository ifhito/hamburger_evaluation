// Package photo は、アップロードされたレビュー写真を検証して正規化する。
// 実際の画像フォーマットを magic bytes から判別し（クライアントが申告した
// content type は無視する）、decompression bomb を防ぎ、長辺が maxEdge に収まる
// ように縮小し、再エンコードする。依存するのは標準ライブラリと
// golang.org/x/image だけなので、内向きの依存ルールに違反することなく
// usecase から import してよい。
package photo

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	"golang.org/x/image/draw"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	_ "golang.org/x/image/webp" // image.Decode に webp を登録する（pure-Go で decode のみ）
)

// ErrUnsupportedImage は、サイズ上限内で decode できる jpeg/png/webp では
// ないアップロードを表す。handler はこれを 422 にマップする。
var ErrUnsupportedImage = errors.New("unsupported image")

// ErrDimensionsTooLarge は、寸法(横・縦・画素数)が上限を超える写真を表す。ErrUnsupportedImage の
// 一種でもある(errors.Is でどちらにも一致する)。直し方が形式の違いとは違う(画像を小さくする)ので、
// handler が別のメッセージにできるように、区別している。
var ErrDimensionsTooLarge = fmt.Errorf("%w: dimensions exceed the limit", ErrUnsupportedImage)

// ErrHEIFNotSupported は、HEIC / HEIF(iPhone の既定の形式)の写真を表す。対応しない形式の一種でも
// ある(ErrUnsupportedImage でもある)。デコーダの依存とメモリの負担が、得られる価値に見合わないので、
// 受け付けない。handler は、対応しない理由が分かる別のメッセージにする。
var ErrHEIFNotSupported = fmt.Errorf("%w: HEIC/HEIF is not supported", ErrUnsupportedImage)

const (
	// maxEdge は出力の最長辺である。これより大きい画像は縮小され、小さい
	// 画像は決して拡大されない。値(業務の規則)は domain が持つ。
	maxEdge = domain.MaxPhotoEdge
	// maxDimension と maxPixels は、画像ヘッダで宣言されたサイズの上限で
	// あり、完全な decode の前にチェックされる（decompression bomb のガード）。
	// 値(業務の規則)は domain が持つ。
	maxDimension = domain.MaxPhotoDimension
	maxPixels    = domain.MaxPhotoPixels
	// maxDecodedBytes は、decode 後のピクセルバッファの推定メモリサイズ
	// （bytesPerPixel × 宣言されたピクセル数）の上限である。これにより、
	// 16-bit の画像が、ピクセル数のガードをすり抜けて約 2 倍大きい decode を
	// 行うことはできない。
	maxDecodedBytes = 128 << 20
	jpegQuality     = 85
)

// decodeSem は、decode/shrink/encode の同時実行を 2 件のアップロードに
// 制限する。これはメモリのガードである。decode は写真リクエストの中で
// メモリを大量に消費する部分であり（最悪の場合 1 件のアップロードあたり約
// maxDecodedBytes、つまり処理中で約 2×128MiB）、上限がなければ、少数の
// 同時アップロードでもコンテナの 1GiB 制限（GOMEMLIMIT 920MiB）を超えかねない。
// acquire 自体にはタイムアウトがなく、リクエストは失敗せずに短時間キューに
// 並ぶが、そのキュー待ちはリクエスト context のキャンセルによって
// 制限されている。キャンセルされた ctx は待機を止める。
var decodeSem = make(chan struct{}, 2)

// bytesPerPixel は、ヘッダの color model から、ピクセルあたりの decode 後の
// メモリコストを保守的に見積もる。16-bit のモデル（RGBA64/NRGBA64/Gray16）は
// 1 ピクセルあたり 8 バイトとして扱う。Gray16 は通常 2 バイトだが、tRNS chunk
// があると NRGBA64 として decode され、color model からは区別できないため、
// 上限の 8 に揃える。それ以外は 4 と仮定する（8-bit の RGBA/NRGBA で、よくある
// ケースの中で最悪のもの）。
func bytesPerPixel(m color.Model) int64 {
	switch m {
	case color.RGBA64Model, color.NRGBA64Model, color.Gray16Model:
		return 8
	default:
		return 4
	}
}

// Processed は、保存できる状態に正規化された画像である。元のアップロードの
// バイト列は破棄される。
type Processed struct {
	// Data は再エンコードされた画像を保持する。
	Data []byte
	// ContentType は "image/jpeg"（jpeg と webp のソース）または "image/png"
	// である。
	ContentType string
	// Ext は ".jpg" または ".png" で、ストレージキーを組み立てるために使う。
	Ext string
}

// Process は r から 1 件のアップロード画像を読み込み、呼び出し側の読み取り
// 上限（現状は handler の 5 MiB + 1 の上限）まで、ペイロードの「全体」を
// メモリにバッファリングし、正規化した結果を返す。magic bytes によれば
// jpeg/png/webp でないペイロード、decode に失敗するペイロード、bomb ガードを
// 超える寸法を宣言するペイロードは、wrap された ErrUnsupportedImage を返す。
// decode 用 semaphore の待機は ctx によって制限される。キャンセル
// （クライアントの切断、サーバのシャットダウン）があると、永遠にキューに
// 並ぶ代わりに ctx.Err() を返す。リクエストがキューに並んでいる間もサーバの
// WriteTimeout は進み続ける。この相互作用は設計上、変更していない。
func Process(ctx context.Context, r io.Reader) (Processed, error) {
	br := bufio.NewReader(r)
	head, err := br.Peek(512)
	if err != nil && len(head) == 0 {
		return Processed{}, fmt.Errorf("%w: empty or unreadable payload", ErrUnsupportedImage)
	}
	ct := http.DetectContentType(head)
	switch ct {
	case "image/jpeg", "image/png", "image/webp":
	default:
		if looksLikeHEIF(head) {
			return Processed{}, ErrHEIFNotSupported
		}
		return Processed{}, fmt.Errorf("%w: detected %s", ErrUnsupportedImage, ct)
	}
	data, err := io.ReadAll(br)
	if err != nil {
		return Processed{}, fmt.Errorf("read image: %w", err)
	}
	// EXIF Orientation は元のアップロードのバイト列から取得しなければならない。
	// 下の再エンコードで metadata はすべて失われるため、保存されるピクセル
	// 自体が正立している必要がある。判別（sniff）で JPEG とされたものだけを
	// チェックし、png/webp は EXIF を処理せずにそのまま通す。jpegOrientation は
	// best-effort であり、壊れているものについては 1（補正なし）を返す。
	orientation := 1
	if ct == "image/jpeg" {
		orientation = jpegOrientation(data)
	}
	// ここから先の処理はメモリを大量に使う（decode + shrink + encode の
	// バッファ）。decodeSem が、同時にこれを行うアップロードの数を制限する。
	// 待機はタイムアウトではなく、リクエスト context のキャンセルによって
	// 制限される。
	select {
	case decodeSem <- struct{}{}:
	case <-ctx.Done():
		return Processed{}, ctx.Err()
	}
	defer func() { <-decodeSem }()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Processed{}, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	if cfg.Width > maxDimension || cfg.Height > maxDimension || cfg.Width*cfg.Height > maxPixels {
		return Processed{}, fmt.Errorf("%w: %dx%d exceeds the size limit", ErrDimensionsTooLarge, cfg.Width, cfg.Height)
	}
	if int64(cfg.Width)*int64(cfg.Height)*bytesPerPixel(cfg.ColorModel) > maxDecodedBytes {
		return Processed{}, fmt.Errorf("%w: %dx%d exceeds the decode memory limit", ErrDimensionsTooLarge, cfg.Width, cfg.Height)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Processed{}, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	// shrink の後に orient する。そうすればこの変換が触るピクセルが少なくなり、
	// 長辺は回転しても変わらないので、shrink を先に行っても正しい。
	img = orient(shrink(img), orientation)
	var out bytes.Buffer
	if format == "png" {
		if err := png.Encode(&out, img); err != nil {
			return Processed{}, fmt.Errorf("encode png: %w", err)
		}
		return Processed{Data: out.Bytes(), ContentType: "image/png", Ext: ".png"}, nil
	}
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Processed{}, fmt.Errorf("encode jpeg: %w", err)
	}
	return Processed{Data: out.Bytes(), ContentType: "image/jpeg", Ext: ".jpg"}, nil
}

// shrink は、長辺が最大でも maxEdge になるように img を縮小し、アスペクト比を
// 保つ。すでに上限内にある画像は変更せずにそのまま通す（拡大はしない）。
func shrink(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) <= maxEdge {
		return img
	}
	var dw, dh int
	if w >= h {
		dw, dh = maxEdge, max(1, h*maxEdge/w)
	} else {
		dw, dh = max(1, w*maxEdge/h), maxEdge
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// heifBrands は、HEIC / HEIF の写真が、ファイルの先頭の ftyp ボックスに書く「主なブランド」である。
var heifBrands = map[string]bool{
	"heic": true, "heix": true, "hevc": true, "hevx": true, "heim": true, "heis": true,
	"hevm": true, "hevs": true, "mif1": true, "msf1": true, "heif": true,
}

// looksLikeHEIF は、先頭のバイト列が HEIC / HEIF の写真かどうかを、ftyp ボックスの主なブランドだけで
// 判断する(4 バイトの大きさ、"ftyp"、4 バイトの主なブランド)。AVIF は主なブランドが "avif" なので、
// 一致しない。中身の検査ではなく、断るときのメッセージを変えるためだけの判断である。
func looksLikeHEIF(head []byte) bool {
	return len(head) >= 12 && string(head[4:8]) == "ftyp" && heifBrands[string(head[8:12])]
}
