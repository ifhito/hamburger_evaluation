package photo

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// この test は、HEIC の同時実行の制限・時間切れ・デコーダの異常終了・ヘッダーの読み取りを、
// 本物のデコーダを使わずに確かめる(デコーダを差し替えて、ふるまいを作り出す)。

// stubDecoder は heicDecode を fn に差し替え、終わったら元に戻す。
func stubDecoder(t *testing.T, fn func() (image.Image, error)) {
	t.Helper()
	orig := heicDecode
	heicDecode = func(_ io.Reader) (image.Image, error) { return fn() }
	t.Cleanup(func() { heicDecode = orig })
}

// waitFor は、条件が満たされるまで(最大 5 秒)待つ。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("時間内に条件が満たされなかった: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDecodeHEIC(t *testing.T) {
	small := image.NewRGBA(image.Rect(0, 0, 4, 4))

	t.Run("多数の HEIC を同時に投げても、デコードは同時に 1 件しか走らない", func(t *testing.T) {
		var running, peak atomic.Int32
		stubDecoder(t, func() (image.Image, error) {
			n := running.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			running.Add(-1)
			return small, nil
		})
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := decodeHEIC(context.Background(), nil); err != nil {
					t.Errorf("decodeHEIC returned error: %v", err)
				}
			}()
		}
		wg.Wait()
		if got := peak.Load(); got != 1 {
			t.Errorf("同時に走ったデコードの最大 = %d, want 1", got)
		}
	})

	t.Run("デコーダが異常終了(panic)しても、1 件の失敗になり、次のデコードは普通にできる", func(t *testing.T) {
		stubDecoder(t, func() (image.Image, error) { panic("デコーダの内部エラー") })
		_, err := decodeHEIC(context.Background(), nil)
		if !errors.Is(err, ErrUnsupportedImage) {
			t.Fatalf("panic のとき err = %v, want ErrUnsupportedImage", err)
		}
		waitFor(t, "同時実行の枠が返る", func() bool { return len(heicSem) == 0 && len(decodeSem) == 0 })
		stubDecoder(t, func() (image.Image, error) { return small, nil })
		if _, err := decodeHEIC(context.Background(), nil); err != nil {
			t.Fatalf("panic の後のデコードが失敗した: %v", err)
		}
	})

	t.Run("デコードが終わらないときは、時間切れで先に戻るが、同時実行の枠はデコードが終わるまで手放さない", func(t *testing.T) {
		origTimeout := heicTimeout
		heicTimeout = 50 * time.Millisecond
		t.Cleanup(func() { heicTimeout = origTimeout })
		release := make(chan struct{})
		stubDecoder(t, func() (image.Image, error) { <-release; return small, nil })

		start := time.Now()
		_, err := decodeHEIC(context.Background(), nil)
		if !errors.Is(err, ErrUnsupportedImage) {
			t.Fatalf("時間切れのとき err = %v, want ErrUnsupportedImage", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("時間切れまでに %v かかった(デコードの終了を待ってしまっている)", elapsed)
		}
		// デコードはまだ走っているので、枠は埋まったまま(メモリの上限を守るため)。
		if len(heicSem) != 1 || len(decodeSem) != cap(decodeSem) {
			t.Errorf("時間切れの直後の枠 = heic %d / decode %d, want 1 / %d(すべて)", len(heicSem), len(decodeSem), cap(decodeSem))
		}
		close(release)
		waitFor(t, "デコードの終了後に枠が返る", func() bool { return len(heicSem) == 0 && len(decodeSem) == 0 })
	})

	t.Run("順番待ちに時間がかかっても、順番が回ってきたあとのデコードには、あらためて時間制限が与えられる", func(t *testing.T) {
		origTimeout := heicTimeout
		heicTimeout = 200 * time.Millisecond
		t.Cleanup(func() { heicTimeout = origTimeout })
		// 1 件目は 150 ミリ秒、2 件目は 120 ミリ秒かかる。2 件目は約 150 ミリ秒待ってから 120 ミリ秒でデコードするので、
		// 合計(約 270 ミリ秒)は制限(200 ミリ秒)を超えるが、待ちとデコードは、それぞれ制限内に収まる。
		var calls atomic.Int32
		stubDecoder(t, func() (image.Image, error) {
			if calls.Add(1) == 1 {
				time.Sleep(150 * time.Millisecond)
			} else {
				time.Sleep(120 * time.Millisecond)
			}
			return small, nil
		})
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() { _, err := decodeHEIC(context.Background(), nil); errs <- err }()
			time.Sleep(20 * time.Millisecond) // 1 件目が先に枠を取る
		}
		for i := 0; i < 2; i++ {
			if err := <-errs; err != nil {
				t.Errorf("decodeHEIC returned error: %v(待ちの時間が、デコードの制限に食い込んでいる)", err)
			}
		}
	})

	t.Run("リクエストがキャンセルされたら、時間切れではなくキャンセルとして戻る", func(t *testing.T) {
		release := make(chan struct{})
		stubDecoder(t, func() (image.Image, error) { <-release; return small, nil })
		ctx, cancel := context.WithCancel(context.Background())
		go func() { time.Sleep(20 * time.Millisecond); cancel() }()
		_, err := decodeHEIC(ctx, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		close(release)
		waitFor(t, "枠が返る", func() bool { return len(heicSem) == 0 && len(decodeSem) == 0 })
	})

	t.Run("枠が空くのを待つ間に時間切れになったら、混雑として扱い、取りかけた枠をすべて返す", func(t *testing.T) {
		origTimeout := heicTimeout
		heicTimeout = 50 * time.Millisecond
		t.Cleanup(func() { heicTimeout = origTimeout })
		// JPEG などが decodeSem の 1 件を使っている状態を作る(HEIC は heicSem と、残りの 1 件を取ったあと、
		// もう 1 件を待たされる)。
		decodeSem <- struct{}{}
		t.Cleanup(func() {
			for len(decodeSem) > 0 {
				<-decodeSem
			}
		})
		stubDecoder(t, func() (image.Image, error) { return small, nil })
		_, err := decodeHEIC(context.Background(), nil)
		if !errors.Is(err, ErrBusy) || errors.Is(err, ErrUnsupportedImage) {
			t.Fatalf("err = %v, want ErrBusy(混雑が原因で、写真が悪いわけではない)", err)
		}
		if len(heicSem) != 0 || len(decodeSem) != 1 {
			t.Errorf("時間切れの後の枠 = heic %d / decode %d, want 0 / 1(JPEG などが使っている 1 件だけが残る)", len(heicSem), len(decodeSem))
		}
	})
}

// isoBox は、ISO のボックスを組み立てる(大きさ 4 バイト + 種類 4 バイト + 本体)。
func isoBox(typ string, body ...[]byte) []byte {
	var content []byte
	for _, b := range body {
		content = append(content, b...)
	}
	out := make([]byte, 8, 8+len(content))
	binary.BigEndian.PutUint32(out, uint32(8+len(content)))
	copy(out[4:], typ)
	return append(out, content...)
}

// heifFile は、指定の寸法の宣言(ispe)を 1 つ以上持つ、最小の HEIC のコンテナを組み立てる。
func heifFile(dims ...[2]uint32) []byte {
	var ispes [][]byte
	for _, d := range dims {
		body := make([]byte, 12) // 版とフラグ 4 バイト + 横 + 縦
		binary.BigEndian.PutUint32(body[4:], d[0])
		binary.BigEndian.PutUint32(body[8:], d[1])
		ispes = append(ispes, isoBox("ispe", body))
	}
	ftyp := isoBox("ftyp", []byte("heic"), []byte{0, 0, 0, 0}, []byte("mif1"), []byte("heic"))
	meta := isoBox("meta", []byte{0, 0, 0, 0}, isoBox("iprp", isoBox("ipco", ispes...)))
	return append(ftyp, meta...)
}

func TestParseHEIF(t *testing.T) {
	t.Run("宣言された寸法のうち最大のものを読む(サムネイルなど複数の宣言がある)", func(t *testing.T) {
		info, err := parseHEIF(heifFile([2]uint32{320, 240}, [2]uint32{4032, 3024}, [2]uint32{512, 512}))
		if err != nil {
			t.Fatalf("parseHEIF returned error: %v", err)
		}
		if info.kind != heifHEIC || info.maxWidth != 4032 || info.maxHeight != 3024 || info.maxPixels != 4032*3024 {
			t.Errorf("info = %+v, want HEIC 4032x3024", info)
		}
	})

	t.Run("寸法が 32 ビットの最大でも、画素数があふれずに大きな値のまま読める", func(t *testing.T) {
		info, err := parseHEIF(heifFile([2]uint32{0xFFFFFFFF, 0xFFFFFFFF}))
		if err != nil {
			t.Fatalf("parseHEIF returned error: %v", err)
		}
		if info.maxPixels != uint64(0xFFFFFFFF)*uint64(0xFFFFFFFF) {
			t.Errorf("maxPixels = %d, want 32 ビット × 32 ビット", info.maxPixels)
		}
	})

	t.Run("壊れた入力は、範囲外を読まず、エラー(または対応しない種類)で終わる", func(t *testing.T) {
		valid := heifFile([2]uint32{100, 100})
		cases := map[string][]byte{
			"空":           {},
			"ftyp の途中":    valid[:10],
			"meta の途中":    valid[:len(valid)-5],
			"ispe が短い":    isoBox("ftyp", []byte("heic"), []byte{0, 0, 0, 0}, []byte("heic")),
			"大きさが 8 未満":   append(append([]byte(nil), valid[:28]...), 0, 0, 0, 4, 'm', 'e', 't', 'a'),
			"大きさが外側より大きい": append(append([]byte(nil), valid[:28]...), 0xFF, 0xFF, 0xFF, 0xFF, 'm', 'e', 't', 'a'),
			"拡張の大きさが外側より大きい":    append(append([]byte(nil), valid[:28]...), 0, 0, 0, 1, 'm', 'e', 't', 'a', 0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF),
			"ftyp が先頭にない":       append([]byte{0, 0, 0, 8, 'f', 'r', 'e', 'e'}, valid...),
			"大きさ 0(末尾まで)の meta": append(append([]byte(nil), valid[:28]...), 0, 0, 0, 0, 'm', 'e', 't', 'a', 0, 0, 0, 0),
		}
		for name, data := range cases {
			t.Run(name, func(t *testing.T) {
				info, err := parseHEIF(data)
				if err == nil && info.kind == heifHEIC {
					t.Errorf("壊れた入力が HEIC として読めてしまった: %+v", info)
				}
			})
		}
	})

	t.Run("同じ名前のボックスを大量に並べても、決まった個数で読むのをやめる", func(t *testing.T) {
		// 空の ispe 以外のボックスを 10 万個並べた ipco。数を数えて打ち切らないと、読むのに時間がかかる。
		var many []byte
		filler := isoBox("free")
		for i := 0; i < 100_000; i++ {
			many = append(many, filler...)
		}
		data := append(isoBox("ftyp", []byte("heic"), []byte{0, 0, 0, 0}, []byte("heic")),
			isoBox("meta", []byte{0, 0, 0, 0}, isoBox("iprp", isoBox("ipco", many)))...)
		start := time.Now()
		info, err := parseHEIF(data)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("読み取りに %v かかった", elapsed)
		}
		if err == nil && info.kind == heifHEIC {
			t.Errorf("寸法の宣言がないのに HEIC として読めた: %+v", info)
		}
	})
}

// FuzzParseHEIF は、どんな入力でも parseHEIF がパニックしないことを確かめる。通常の go test では、
// 種になる入力(本物の HEIC と組み立てたコンテナ)だけを実行する。
func FuzzParseHEIF(f *testing.F) {
	for _, name := range []string{"landscape.heic", "portrait_irot.heic"} {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
	f.Add(heifFile([2]uint32{100, 100}))
	f.Add([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseHEIF(data) // パニックしなければよい
	})
}
