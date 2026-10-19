package tui

// runner.go - 転送・照合の非同期実行（bash 版 dashboard.sh 相当）
// goroutine からは tea.Program.Send で通知する（Update の受信側は model.go）。

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kazpc1215/safersync-go/internal/core"
)

// hasCaffeinate は caffeinate コマンドの有無。
func hasCaffeinate() bool {
	_, err := exec.LookPath("caffeinate")
	return err == nil
}

// itoa は int→string（表示用）。
func itoa(n int) string { return strconv.Itoa(n) }

// headerOf は各ステージのヘッダ（frame に渡す）。
func (m model) headerOf() string {
	switch m.stage {
	case stSource:
		return "転送したいデータ（SDカードなど）を選んでください"
	case stDest:
		return "保存先（外付けHDDなど）を選んでください\nコピー元と同じボリュームも選べます"
	}
	return ""
}

// footerOf は各ステージのフッタ。
func (m model) footerOf() string {
	switch m.stage {
	case stSource:
		return "[↑/↓] 移動   [Enter] 選択   [q] 終了"
	case stSourceDir, stDestDir:
		return "[↑/↓] 移動   [Enter] 選択   [n] 新規フォルダ   [p] パス入力   [/] 検索   [h] 隠し   [Esc] 戻る"
	case stOptions:
		return "[↑/↓] 移動   [Space] 切り替え   [Enter] 次へ   [Esc] 戻る"
	case stPreview:
		if m.previewTab != 0 && !m.showFileList {
			return "[↑/↓] 詳細スクロール   [←/→] 項目   [Tab] 切替   [Enter] 決定   [Esc] 戻る"
		}
		return "[←/→ でも ↑/↓ でも可] [Enter] 決定   [Esc] 戻る"
	case stDone:
		return "[Enter] 終了"
	}
	return ""
}

// startPlanCmd は差分スキャン（プレビュー用）を背景で行う。
// Ctrl-C でスキャン中の rsync を止められるよう、キャンセル可能にしておく。
func startPlanCmd(m *model) tea.Cmd {
	j := m.job()
	ctx, cancel := context.WithCancel(context.Background())
	m.scanCancel = cancel
	j.Ctx = ctx

	return func() tea.Msg {
		prog := m.prog
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					prog.Send(spinnerTickMsg{})
				}
			}
		}()
		plan := j.PlanChanges()
		if ctx.Err() != nil {
			// キャンセル済みなら、重い SrcScan も planReady の送信も行わない
			// （終了処理と競合して不要なメッセージを送らない）。
			close(done)
			return nil
		}
		s := j.SrcScan()
		close(done)
		prog.Send(planReadyMsg{srcFiles: s.Files, srcBytes: s.Bytes, plan: plan})
		return nil
	}
}

// startRun はプレビューの「開始」から呼ばれ、--verify-only / --dry-run /
// 通常の転送を振り分ける。戻り値の tea.Cmd は非同期処理（照合など）用。
func (m *model) startRun() (tea.Cmd, error) {
	if m.optVerifyOnly {
		m.startAt = time.Now()
		m.xferFiles = 0
		m.xferBytes = 0
		m.stage = stVerifyBusy
		return startVerifyCmd(m), nil
	}
	if m.optDryRun {
		m.startAt = time.Now()
		m.xferFiles = m.plan.Files
		m.xferBytes = m.plan.Bytes
		m.dryRun = true
		m.finalRC = 0
		m.stage = stDone
		core.LogMsg("COPY dry-run " + m.src + " -> " + m.dst + " [tui]")
		return nil, nil
	}
	if err := m.beginTransfer(); err != nil {
		return nil, err
	}
	m.stage = stTransfer
	return nil, nil
}

// beginTransfer はダッシュボードを開始し、進捗通知の goroutine を起動する。
// Update から直接呼ぶ（rsync の起動はノンブロッキング）。
func (m *model) beginTransfer() error {
	job := m.job()
	if m.plan.Changes > 0 {
		core.LogMsg("COPY start  " + job.Src + " -> " + job.Dst + " (" +
			strconv.Itoa(m.plan.Files) + " files, " + strconv.Itoa(m.plan.Changes) + " changes) [tui]")
	}
	m.startAt = time.Now()
	m.xferFiles = m.plan.Files
	m.xferBytes = m.plan.Bytes
	// プレビューで走査済みのコピー元を引き継ぎ、転送開始時の再走査を省く
	// （大きなツリーほど効く）。srcScan はプレビュー（planReadyMsg）で必ず設定される。
	job.SetSrcScan(m.srcScan)
	// 進捗の分母は今回の転送量（総ファイルサイズではない）。
	job.PlanBytes = m.plan.Bytes
	job.PlanFiles = m.plan.Files

	t := job.StartTransfer()
	if err := t.StartError(); err != nil {
		t.Cleanup()
		return err
	}
	// 再試行などで前の転送が残っている場合は、その一時ファイルを片付ける
	if m.transfer != nil {
		m.transfer.Cleanup()
	}
	m.transfer = t

	prog := m.prog
	// 完了は1か所で待ち、その間に進捗フレームを流す（bash 版の while kill -0 相当）
	returned := make(chan int, 1)
	go func() { returned <- t.Wait() }()

	go func() {
		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()
		for {
			select {
			case rc := <-returned:
				switch {
				case rc == 0:
					core.LogMsg("COPY done   " + job.Src + " -> " + job.Dst + " [tui]")
				case core.IsVanishedExit(rc):
					// dry-run / 照合と同じく成功扱い（消えたファイルは次回拾える）
					core.LogMsg("COPY done   " + job.Src + " -> " + job.Dst + " (vanished files, rc=24) [tui]")
					rc = 0
				default:
					core.LogMsg("COPY failed " + job.Src + " -> " + job.Dst + " (rc=" + strconv.Itoa(rc) + ") [tui]")
				}
				prog.Send(transferDoneMsg{rc: rc})
				return
			case <-tick.C:
				pct, doneB, total, speed, eta := t.Frame()
				prog.Send(frameMsg{pct: pct, done: doneB, total: total, speed: speed, eta: eta, file: t.CurrentFile()})
			}
		}
	}()
	return nil
}

// startVerifyCmd は照合を背景実行し、出力を回収して送る。
func startVerifyCmd(m *model) tea.Cmd {
	job := m.job()
	// プレビューで走査済みのコピー元を引き継ぎ、照合時の集計で再走査しない
	job.SetSrcScan(m.srcScan)
	ctx, cancel := context.WithCancel(context.Background())
	m.scanCancel = cancel
	job.Ctx = ctx

	return func() tea.Msg {
		prog := m.prog
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					prog.Send(spinnerTickMsg{})
				}
			}
		}()
		var buf bytes.Buffer
		j := *job
		j.Out = &buf
		j.Err = &buf
		rc := core.VerifyOne(&j)
		// ログは core.VerifyOne が出す（ここで重ねて出すと二重になる）。
		close(done)
		prog.Send(verifyDoneMsg{rc: rc, output: buf.String()})
		return nil
	}
}
