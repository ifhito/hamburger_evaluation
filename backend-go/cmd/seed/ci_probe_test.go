package main

import "testing"

// CI がテストの失敗で赤くなることの確認用。わざと失敗させる(確認が済んだら、このファイルごと消す)。
func TestCiProbeAlwaysFails(t *testing.T) {
	t.Fatal("CI が、失敗するテストで赤くなることの確認用(わざと失敗させている)")
}
