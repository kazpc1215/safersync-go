package tui

// runner.go - 転送・照合の非同期実行（bash 版 dashboard.sh 相当）
// goroutine からは tea.Program.Send で通知する（Update の受信側は model.go）。

import (
	"bytes"
	"os/exec"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kazpc1215/safersync/internal/core"
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
		return "[←/→ でも ↑/↓ でも可] [Enter] 決定   [Esc] 戻る"
	case stDone:
		return "[Enter] 終了"
	}
	return ""
}

// startPlanCmd は差分スキャン（プレビュー用）を背景で行う。
func startPlanCmd(m *model) tea.Cmd {
	return func() tea.Msg {
		j := m.job()
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
		s := core.ScanDir(j.Src, j.AllExcludePatterns())
		close(done)
		prog.Send(planReadyMsg{srcFiles: s.Files, srcBytes: s.Bytes, plan: plan})
		return nil
	}
}

// beginTransfer はダッシュボードを開始し、進捗通知の goroutine を起動する。
// Update から直接呼ぶ（rsync の起動はノンブロッキング）。
func (m *model) beginTransfer() error {
	job := m.job()
	if m.plan.Files > 0 {
		core.LogMsg("COPY start  " + job.Src + " -> " + job.Dst + " (" +
			strconv.Itoa(m.plan.Files) + " files) [tui]")
	}
	m.startAt = time.Now()
	m.xferFiles = m.plan.Files
	m.xferBytes = m.plan.Bytes

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
				switch rc {
				case 0:
					core.LogMsg("COPY done   " + job.Src + " -> " + job.Dst + " [tui]")
				default:
					core.LogMsg("COPY failed " + job.Src + " -> " + job.Dst + " (rc=" + strconv.Itoa(rc) + ") [tui]")
				}
				if m.xferFiles == 0 {
					rc = 0 // bash 版と同じ: 差分ゼロなら転送はスキップして照合へ
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
	return func() tea.Msg {
		job := m.job()
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
		if rc == 0 {
			core.LogMsg("VERIFY ok   " + job.Src + " vs " + job.Dst + " [tui]")
		} else {
			core.LogMsg("VERIFY diff " + job.Src + " vs " + job.Dst + " [tui]")
		}
		close(done)
		prog.Send(verifyDoneMsg{rc: rc, output: buf.String()})
		return nil
	}
}
