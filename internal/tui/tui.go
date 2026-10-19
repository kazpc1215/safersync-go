package tui

// tui.go - エントリポイント（bash 版 bin/safersync の tui_source_all + tui_main 相当）

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

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

// terminateChild は実行中の rsync を止めて終了を待つ（TUI 終了時の保険）。
// 子プロセスは別プロセスグループで動くため、これをしないと rsync が孤児として
// コピーを続けてしまう。
func (m *model) terminateChild() {
	m.cancelScan() // スキャン/照合中の dry-run rsync も止める
	if m.transfer != nil {
		m.transfer.Terminate(2 * time.Second)
	}
}

// RunTui は対話ウィザードを実行し、終了コードを返す。// 呼び出し側（cli）は対話端末かどうかを確認してから呼ぶ。
func RunTui(mountRoot, from, to string) int {
	// 引数で指定されていない方だけ、前回使ったパスを補う（利便性向上）。
	// 明示指定（例: --to X）を履歴で上書きしない。
	from, to = fillFromHistory(from, to)
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

	// 外部シグナルでも転送中の rsync を孤児にしない。
	//   SIGINT / SIGTERM は Bubble Tea も処理するが、SIGHUP（ターミナル終了）は
	//   誰も捕まえないため、ここで受け取って安全に中断する。
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	var signaled atomic.Bool
	sigDone := make(chan struct{})
	go func() {
		select {
		case <-sigDone:
		case <-sigCh:
			signaled.Store(true)
			p.Send(interruptRequestMsg{})
		}
	}()

	final, err := p.Run()
	close(sigDone)
	signal.Stop(sigCh)

	fm, ok := final.(*model)
	if !ok {
		fm = m
	}
	// 終了理由（正常終了 / 中断 / シグナル / エラー）にかかわらず、
	// 実行中の rsync を確実に止めてから終了する。
	fm.terminateChild()
	if fm.transfer != nil {
		// rsync 出力の一時ファイルを片付ける（/tmp に溜めない）
		fm.transfer.Cleanup()
	}
	saveHistory(fm.src, fm.dst)

	switch {
	case signaled.Load():
		// 外部シグナルによる終了（Bubble Tea が先に quit した場合も含む）
		fm.interrupt = true
		fm.finalRC = core.ExitInterrupted
		return fm.finalRCWithInterrupt()
	case err == nil:
		return fm.finalRCWithInterrupt()
	case errors.Is(err, tea.ErrInterrupted):
		// SIGINT（端末の Ctrl-C が生の入力として届かない場合）
		fm.interrupt = true
		fm.finalRC = core.ExitInterrupted
		return fm.finalRCWithInterrupt()
	default:
		fmt.Fprintln(os.Stderr, "✗ TUI を実行できませんでした:", err)
		return core.ExitError
	}
}
