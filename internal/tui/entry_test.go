package tui

// entry_test.go - 未到達だった TUI の入口（Init / finalRCWithInterrupt）のテスト。
// 実際に Bubble Tea を走らせる経路（RunTui / beginTransfer）は対話端末と
// 実行中プログラムが要るため対象外。

import "testing"

func TestModelInitReturnsNil(t *testing.T) {
	if cmd := tbase().Init(); cmd != nil {
		t.Errorf("Init は nil を返すはず: %v", cmd)
	}
}

func TestFinalRCWithInterrupt(t *testing.T) {
	m := tbase()
	m.interrupt = true
	m.finalRC = 3
	if got := m.finalRCWithInterrupt(); got != 3 {
		t.Errorf("finalRC をそのまま返すはず: got %d", got)
	}

	m2 := tbase() // 非中断（finalRC はゼロ値 = 0）
	if got := m2.finalRCWithInterrupt(); got != 0 {
		t.Errorf("非中断時は finalRC(0) を返すはず: got %d", got)
	}
}
