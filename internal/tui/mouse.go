package tui

// mouse.go - マウス操作（ホイールで移動・クリックで選択）。ヘルプに載せている機能。

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// updateMouse はマウスイベントを処理する。
// ホイールは ↑/↓ キーと同じ動きにそろえ（ログ表示中のスクロールも含む）、
// 左クリックは一覧の項目選択にする。入力中・ヘルプ表示中は使わない。
func (m *model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.prompting || m.showHelp {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.updateKey(tea.KeyMsg{Type: tea.KeyUp}) // ログ表示中はスクロールになる
	case tea.MouseButtonWheelDown:
		return m.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.showLog {
		return m, nil // ログはホイールのみ
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		m.selectRowAt(msg.Y)
	}
	return m, nil
}

// listTop は一覧の先頭行が画面の何行目か（0 始まり）を返す。
// frame() の組み立て（上罫線 → [空行+ヘッダ行] → 区切り線 → body）と同じ順で数える。
func (m model) listTop() int {
	top := 1 // 上罫線
	if h := m.headerOf(); h != "" {
		top += 1 + len(strings.Split(h, "\n")) // 空行 + ヘッダ行
	}
	return top + 1 // 区切り線
}

// selectRowAt はクリックされた行 y（0 始まり）に対応する項目へ選択を移す。
// 一覧の外（ヘッダ・フッタ・余白）をクリックしても何もしない。
func (m *model) selectRowAt(y int) {
	switch m.stage {
	case stSource, stDest:
		if i := y - m.listTop(); i >= 0 && i < len(m.items) {
			m.sel = i
		}
	case stOptions:
		if i := y - m.listTop(); i >= 0 && i < numOptions {
			m.sel = i
		}
	case stSourceDir, stDestDir:
		entries, headerCount, start, _ := m.browseListWindow()
		first := m.listTop() + headerCount
		if start > 0 {
			first++ // 「↑ N 件省略」の1行
		}
		if i := y - first; i >= 0 && start+i < len(entries) {
			m.sel = start + i
		}
	case stPreview:
		if m.showFileList {
			return // ファイル一覧の表示中は決定メニューが無い
		}
		lines := m.previewBodyLines()
		first := m.listTop() + len(lines) - len(m.items)
		if i := y - first; i >= 0 && i < len(m.items) {
			m.sel = i
		}
	}
}
