package infra

import "time"

// SystemClock は、実際の現在時刻を返す usecase.Clock である。本番ではこれを使い、テストでは
// 固定の時刻を返す実装に差し替える。
type SystemClock struct{}

// Now は現在時刻を返す。
func (SystemClock) Now() time.Time { return time.Now() }
