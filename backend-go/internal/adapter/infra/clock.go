package infra

import "time"

// SystemClock は、実際の現在時刻を返す usecase.Clock である。
type SystemClock struct{}

// Now は現在時刻を返す。
func (SystemClock) Now() time.Time { return time.Now() }
