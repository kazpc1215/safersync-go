package tui

// tui.go - エントリポイント（bash 版 bin/safersync の tui_source_all + tui_main 相当）

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kazpc1215/safersync/internal/core"
)

// finalRCWithInterrupt は中断時のメッセージを出しつつ終了コードを返す。
func (m *model) finalRCWithInterrupt() int {
	if m.interrupt {
		// bash 版 cleanup 後のメッセージと同じ
		fmt.Fprintln(os.Stderr, "中断しました（次回は続きからコピーできます）")
	}
	return m.finalRC
}

// RunTui は対話ウィザードを実行し、終了コードを返す。// 呼び出し側（cli）は対話端末かどうかを確認してから呼ぶ。
func RunTui(mountRoot, from, to string) int {
	m := newModel(mountRoot, from, to)
	// 初期メニューの組み立て（プリフィル済みならそのステージのもの）
	switch m.stage {
	case stDest:
		m.loadDestMenu()
	case stOptions, stSource, stSourceDir:
		m.loadSourceMenu()
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.prog = p

	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ TUI を開始できませんでした:", err)
		return core.ExitError
	}
	if fm, ok := final.(*model); ok {
		return fm.finalRCWithInterrupt()
	}
	return m.finalRCWithInterrupt()
}
