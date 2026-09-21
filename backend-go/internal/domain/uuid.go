package domain

// IsUUID は s が UUID の正規形（小文字の 16 進数とハイフンによる 8-4-4-4-12 の 36 文字）かどうかを
// 返す。大文字やハイフンのない形は、正規形ではないので不正な形式として扱う（別の表記が同じ ID の
// 別名になって、比較や URL が食い違うことを避けるため）。バージョンやバリアントのビットは見ない。
// ID の形式の判断は domain だけが持ち、handler や frontend は判定を持たない。
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
				return false
			}
		}
	}
	return true
}
