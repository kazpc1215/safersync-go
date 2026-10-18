package tui

// view.go - 描画（bash 版 view_*.sh と同じ1枚絵構成）

import (
	"os"
	"strings"
	"time"

	"github.com/kazpc1215/safersync/internal/core"
)

const (
	minWidth  = 50
	minHeight = 12
)

// View は現在のステージの全画面を描く。
func (m model) View() string {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}
	if m.showHelp {
		return frame(w, progTitle()+" ヘルプ", "",
			[]string{
				"  [↑/↓ or j/k] 移動",
				"  [Enter] 決定 / 開始",
				"  [Space] オプション切替",
				"  [/] フィルタ入力",
				"  [h] 隠しファイル表示切替",
				"  [Esc] 戻る",
				"  [Ctrl-C] 安全中断（2回で確定）",
				"  [q] 中止/終了",
				"  [?] このヘルプを閉じる",
				"  マウス: クリックで選択 / ホイールでスクロール",
			}, " [?] または [Esc] で閉じる")
	}
	if w < minWidth || h < minHeight {
		return frame(w, "safersync", "",
			[]string{"ターミナルを広げてください（現在 " + itoa(w) + "x" + itoa(h) + "）"}, "")
	}
	title := progTitle() + " " + m.titleOf(m.stage)
	if m.stage == stDone {
		if m.finalRC == 0 {
			title = progTitle() + " 完了"
		} else {
			title = progTitle() + " 問題あり"
		}
	}
	header := m.headerOf()
	footer := m.footerOf()
	body := []string{}

	switch m.stage {
	case stMessage:
		return frame(w, progTitle(), "",
			[]string{stErr.Render("  " + m.msg)}, " [Enter] で戻る")
	case stSource, stDest:
		for i, item := range m.items {
			pre := " " + cursor(i == m.sel) + " "
			body = append(body, pre+item)
		}
	case stSourceDir, stDestDir:
		if m.prompting {
			return frame(w, title, "",
				[]string{
					"",
					"  " + m.promptLbl,
					"",
					"  > " + m.prompt + "_",
				}, " [Enter] 決定   [Esc] キャンセル")
		}
		body = append(body, " 現在のフォルダ: "+m.browseDir)
		// パンくず表示
		parts := strings.Split(strings.Trim(m.browseDir, "/"), "/")
		if m.browseDir == "/" {
			parts = []string{"/"}
		}
		body = append(body, " パンくず: / "+strings.Join(parts, " / "))
		if m.browseFilter != "" {
			body = append(body, " フィルタ: "+m.browseFilter)
		}
		entries := browseEntriesAdvanced(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate, m.showHidden, m.browseFilter)
		// 画面高さに合わせてスクロール（長い一覧でも収まるように）
		headerLines := len(body)
		overhead := 8
		visible := m.height - overhead - headerLines
		if visible < 3 {
			visible = 3
		}
		if visible > len(entries) {
			visible = len(entries)
		}
		start := 0
		if m.sel >= visible {
			start = m.sel - visible + 1
		}
		if start+visible > len(entries) {
			start = len(entries) - visible
			if start < 0 {
				start = 0
			}
		}
		if start > 0 {
			body = append(body, "  ↑ "+itoa(start)+" 件省略")
		}
		for i := start; i < start+visible && i < len(entries); i++ {
			it := entries[i]
			pre := " " + cursor(i == m.sel) + " "
			body = append(body, pre+it.label)
		}
		if start+visible < len(entries) {
			body = append(body, "  ↓ "+itoa(len(entries)-start-visible)+" 件省略")
		}
		footer = "[↑/↓] 移動   [Enter] 選択   [/] 検索   [h] 隠し表示切替   [Esc] 戻る"
	case stOptions:
		for i := 0; i < 5; i++ {
			body = append(body, " "+cursor(i == m.sel)+" "+m.optionLine(i))
		}
	case stPlanBusy:
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		return frame(w, title, "",
			[]string{"  " + frames[m.spinner%len(frames)] + " 差分をスキャンしています..."}, "")
	case stVerifyBusy:
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		return frame(w, title, "",
			[]string{"  " + frames[m.spinner%len(frames)] + " コピー後の照合をしています..."}, "")
	case stPreview:
		if m.showFileList {
			body = append(body, " 転送対象ファイル一覧（l で閉じる）:")
			lines := m.plan.Lines
			if len(lines) == 0 {
				body = append(body, "  （転送対象はありません）")
			} else {
				visible := m.height - 10
				if visible < 3 {
					visible = 3
				}
				if visible > len(lines) {
					visible = len(lines)
				}
				start := m.fileListScroll
				if start+visible > len(lines) {
					start = len(lines) - visible
					if start < 0 {
						start = 0
					}
				}
				for i := start; i < start+visible && i < len(lines); i++ {
					body = append(body, "  "+lines[i])
				}
				if start > 0 {
					body = append(body, "  ↑ "+itoa(start)+" 件省略")
				}
				if start+visible < len(lines) {
					body = append(body, "  ↓ "+itoa(len(lines)-start-visible)+" 件省略")
				}
			}
			break
		}
		for _, l := range m.previewHeaderLines() {
			body = append(body, l)
		}
		for i, item := range m.items {
			body = append(body, " "+cursor(i == m.sel)+" "+item)
		}
	case stTransfer:
		for _, l := range m.dashboardLines() {
			body = append(body, l)
		}
	case stDone:
		for _, l := range m.doneLines() {
			body = append(body, l)
		}
	}
	return frame(w, title, header, body, footer)
}

const hideCursor = ""

func (m model) optionLine(i int) string {
	switch i {
	case 0:
		return checkbox(m.optLRV) + " プレビュー動画 (.LRV) を除外（容量・時間を節約）"
	case 1:
		return checkbox(m.optTHM) + " サムネイル画像 (.THM) を除外"
	case 2:
		return checkbox(m.optTMP) + " 一時ファイル (*.tmp, .DS_Store 等) も除外"
	case 3:
		return "照合方法: " + radio(!m.optChecksum) + " 高速照合（サイズ+更新日時・推奨）   " +
			radio(m.optChecksum) + " 厳密照合（ハッシュ）"
	case 4:
		if m.caffeineAvail {
			return checkbox(m.optCaffeinate) + " スリープ防止 (caffeinate) を有効にする"
		}
		return "[ ] スリープ防止 (caffeinate)（この環境では利用不可）"
	case 5:
		if len(m.extraExcludes) == 0 {
			return "追加の除外パターン: （なし）  [Space/Enter] 追加"
		}
		return "追加の除外: " + strings.Join(m.extraExcludes, ", ") + "  [Space/Enter] 追加"
	}
	return ""
}

// previewHeaderLines は差分スキャン結果と注意（bash 版 tui_view_preview）。
func (m model) previewHeaderLines() []string {
	skip := m.srcScan.Files - m.plan.Files
	if skip < 0 {
		skip = 0
	}
	skipb := m.srcScan.Bytes - m.plan.Bytes
	if skipb < 0 {
		skipb = 0
	}
	spaceMsg := "十分な空きがあります"
	if !m.spaceOK {
		spaceMsg = "空き容量が不足しています"
	}
	lines := []string{
		"",
		"コピー元: " + m.src,
		"コピー先: " + m.dst,
		"除外設定: " + m.excludesSummary(),
		"",
		"差分スキャン結果:",
		"  ・転送対象: " + itoa(m.plan.Files) + " ファイル（" + core.FmtBytes(m.plan.Bytes) + "）",
		"  ・スキップ: " + itoa(skip) + " ファイル（" + core.FmtBytes(skipb) + " - すでに転送済み）",
		"  ・転送先空き: " + core.FmtBytes(m.free) + "  [" + spaceMsg + "]",
	}
	if !m.mounted {
		lines = append(lines, "注意: コピー先のボリュームがマウントされていません")
	}
	if m.warnMP {
		lines = append(lines, "注意: コピー先がボリュームの直下です（中身が散らばります）")
	}
	lines = append(lines, "")
	return lines
}

// dashboardLines は転送ダッシュボード（bash 版 tui_render_dashboard と同じ構成）。
func (m model) dashboardLines() []string {
	w := m.width
	if w < minWidth {
		w = minWidth
	}
	inner := w - 8
	if inner < 10 {
		inner = 10
	}
	bar := progressBar(m.frame.pct, inner)
	etaTxt := "--"
	if m.frame.eta >= 0 {
		etaTxt = "約 " + core.FmtDuration(m.frame.eta)
	}
	fileLine := "  コピー中: " + m.frame.file
	if strings.TrimSpace(m.frame.file) == "" {
		fileLine = "  コピー中: （ファイル名を取得中...）"
	}
	return []string{
		"",
		fileLine,
		"",
		"  [" + bar + "]  " + itoa(m.frame.pct) + "%",
		"  " + core.FmtBytes(m.frame.done) + " / " + core.FmtBytes(m.frame.total) +
			"  (" + core.FmtBytes(m.frame.speed) + "/s)  残り " + etaTxt,
		"",
		"  [✔] 事前チェック（マウント・空き容量）",
		"  [▶] ファイル転送中...",
		"  [ ] コピー後データ整合性照合",
		"",
		"  経過時間: " + core.FmtDuration(int64(time.Since(m.startAt).Seconds())),
		"",
		func() string {
			if m.confirmCancel {
				return "  " + stWarn.Render("もう一度 Ctrl-C で中断します")
			}
			return "  [Ctrl-C] 安全に中断（次回続きから再開できます）"
		}(),
	}
}

// doneLines は完了レポート（bash 版 tui_view_done）。
func (m model) doneLines() []string {
	out := []string{""}
	var status, verifyTxt string
	titleOK := m.finalRC == 0
	if titleOK {
		status = "すべての処理が正常に完了しました"
		verifyTxt = stOk.Render("正常（すべてのファイルが一致）")
	} else {
		status = stErr.Render("エラーが発生しました（終了コード " + itoa(m.finalRC) + "）")
		verifyTxt = stWarn.Render("不一致または失敗（ログを確認してください）")
	}
	if m.plan.Files == 0 {
		status = "転送するものはありませんでした（すべて最新）"
	}
	out = append(out,
		"  "+status,
		"",
		"   ・転送完了: "+itoa(m.xferFiles)+" ファイル（"+core.FmtBytes(m.xferBytes)+" / "+core.FmtDuration(int64(m.elapsed.Seconds()))+"）",
		"   ・照合結果: "+verifyTxt,
		"   ・ログ保存: "+m.logFile,
	)
	if titleOK {
		out = append(out, "", "  "+stOk.Render("SDカード/メディアを安全に取り外すことができます。"))
	}
	if m.interrupt {
		out = append(out, "   "+stWarn.Render("中断しました（次回は続きからコピーできます）"))
	}
	// エラー時はログの末尾を表示
	if m.finalRC != 0 && m.transfer != nil {
		if data, err := os.ReadFile(m.transfer.LogFile()); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) > 0 {
				out = append(out, "", "  ログ末尾:")
				start := 0
				if len(lines) > 5 {
					start = len(lines) - 5
				}
				for _, l := range lines[start:] {
					out = append(out, "    "+l)
				}
			}
		}
	}
	return out
}
