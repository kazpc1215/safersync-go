// Package tui は Bubble Tea による対話ウィザード。
// bash 版 lib/tui/（代替画面・罫線フレーム・日本語幅）と同じ画面構成で、
// 描画・キー解析は Bubble Tea に任せる。
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// 罫線フレーム（bash 版 tui_frame と同じ構成）
//   ┌─ タイトル ──────┐
//   │ （ヘッダ行…）    │
//   ├────────────────┤
//   │ body 行 […]      │
//   │                  │
//   ├────────────────┤
//   │ フッタ           │
//   └────────────────┘
// ---------------------------------------------------------------------------

func boxTop(w int, title string) string {
	inner := w - 2
	label := "─ " + stTitle.Render(title) + " "
	lw := lipgloss.Width(label)
	avail := inner - lw
	if avail < 0 {
		// タイトルを切り詰める（bash 版と同じ）
		t2 := title
		if limit := inner - 3; limit > 0 && lipgloss.Width("─ "+t2+" ") > inner {
			for lipgloss.Width(t2) > limit && len(t2) > 0 {
				r := []rune(t2)
				t2 = string(r[:len(r)-1])
			}
		}
		label = "─ " + t2 + " "
		lw = lipgloss.Width(label)
		avail = inner - lw
	}
	if avail < 0 {
		avail = 0
	}
	return "┌" + label + strings.Repeat("─", avail) + "┐"
}

func boxSep(w int) string {
	inner := w - 2
	if inner < 0 {
		inner = 0
	}
	return "├" + strings.Repeat("─", inner) + "┤"
}

func boxBottom(w int) string {
	inner := w - 2
	if inner < 0 {
		inner = 0
	}
	return "└" + strings.Repeat("─", inner) + "┘"
}

func boxRow(w int, text string) string {
	inner := w - 4
	if inner < 0 {
		inner = 0
	}
	wt := lipgloss.Width(text)
	if wt > inner {
		// 表示幅以内で切り詰める（文字の途中で切らない）
		r := []rune(text)
		for lipgloss.Width(string(r)) > inner && len(r) > 0 {
			r = r[:len(r)-1]
		}
		text = string(r)
		wt = lipgloss.Width(text)
	}
	pad := inner - wt
	if pad < 0 {
		pad = 0
	}
	return "│ " + text + strings.Repeat(" ", pad) + " │"
}

func boxBlank(w int) string { return boxRow(w, "") }

// frame はヘッダ・body・フッタを1枚の罫線フレームに組み立てる。
func frame(w int, title, header string, body []string, footer string) string {
	var b strings.Builder
	b.WriteString(boxTop(w, title) + "\n")
	if header != "" {
		b.WriteString(boxBlank(w) + "\n")
		for _, line := range strings.Split(header, "\n") {
			b.WriteString(boxRow(w, line) + "\n")
		}
	}
	b.WriteString(boxSep(w) + "\n")
	for _, line := range body {
		b.WriteString(boxRow(w, line) + "\n")
	}
	b.WriteString(boxBlank(w) + "\n")
	b.WriteString(boxSep(w) + "\n")
	b.WriteString(boxRow(w, footer) + "\n")
	b.WriteString(boxBottom(w))
	return b.String()
}

// ---------------------------------------------------------------------------
// 部品
// ---------------------------------------------------------------------------

func checkbox(on bool) string {
	if on {
		return stOk.Render("[✔]")
	}
	return "[ ]"
}

func radio(on bool) string {
	if on {
		return stCursor.Render("(●)")
	}
	return "( )"
}

func cursor(on bool) string {
	if on {
		return stCursor.Render("▶")
	}
	return " "
}

func progressBar(pct, w int) string {
	if w <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * w / 100
	// 進捗に応じて色を変える（0-50: 青, 50-90: 黄, 90-100: 緑）
	color := "12" // 青
	switch {
	case pct >= 90:
		color = "10" // 緑
	case pct >= 50:
		color = "11" // 黄
	}
	filledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	return filledStyle.Render(strings.Repeat("█", filled)) + stDim.Render(strings.Repeat("░", w-filled))
}

// スタイル（画面に少し彩りを added。非TTYでは ANSI は出ない）
var (
	stTitle  = lipgloss.NewStyle().Bold(true)
	stCursor = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "4", Dark: "12"}) // 青
	stOk     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "2", Dark: "10"}) // 緑
	stWarn   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "3", Dark: "11"}) // 黄
	stErr    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "1", Dark: "9"})  // 赤
	stDim    = lipgloss.NewStyle().Faint(true)
)
