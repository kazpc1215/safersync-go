package tui

// tui.go - エントリポイント（bash 版 bin/safersync の tui_source_all + tui_main 相当）

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kazpc1215/safersync/internal/core"
)

// execCommand is a thin wrapper to keep imports tidy and testable.
var execCommand = exec.Command

// finalRCWithInterrupt は中断時のメッセージを出しつつ終了コードを返す。
func (m *model) finalRCWithInterrupt() int {
	if m.interrupt {
		// bash 版 cleanup 後のメッセージと同じ
		fmt.Fprintln(os.Stderr, "中断しました（次回は続きからコピーできます）")
	} else {
		// 完了を端末ベルと（可能なら）デスクトップ通知で知らせる
		fmt.Fprint(os.Stderr, "\a")
		if runtime.GOOS == "darwin" {
			title := "safersync"
			msg := "転送が完了しました"
			if m.finalRC != 0 {
				msg = "問題が発生しました（終了コード " + strconv.Itoa(m.finalRC) + "）"
			}
			_ = execCommand("osascript", "-e",
				`display notification "`+msg+`" with title "`+title+`"`).Run()
		}
	}
	return m.finalRC
}

// RunTui は対話ウィザードを実行し、終了コードを返す。// 呼び出し側（cli）は対話端末かどうかを確認してから呼ぶ。
func RunTui(mountRoot, from, to string) int {
	// 引数が無ければ、前回の TUI で使ったパスをプリフィルする（利便性向上）
	if from == "" {
		if hsrc, hdst := loadHistory(); hsrc != "" {
			from = hsrc
			to = hdst
		}
	}
	m := newModel(mountRoot, from, to)
	// 初期メニューの組み立て（プリフィル済みならそのステージのもの）
	switch m.stage {
	case stDest:
		m.loadDestMenu()
	case stOptions, stSource, stSourceDir:
		m.loadSourceMenu()
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.prog = p

	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ TUI を開始できませんでした:", err)
		return core.ExitError
	}
	if fm, ok := final.(*model); ok {
		saveHistory(fm.src, fm.dst)
		return fm.finalRCWithInterrupt()
	}
	saveHistory(m.src, m.dst)
	return m.finalRCWithInterrupt()
}
