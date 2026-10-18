package tui

// mouse_test.go - マウス操作（ホイール移動・クリック選択）のテスト。
// selectRowAt の計算と実際の描画が一致することを、描画結果そのもので確かめる。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// cursorRow は描画結果からカーソル（▶）の行番号を返す。
func cursorRow(t *testing.T, v string) int {
	t.Helper()
	for i, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "▶") {
			return i
		}
	}
	t.Fatalf("カーソル行が見つからない:\n%s", v)
	return -1
}

func wheelDown() tea.MouseMsg { return tea.MouseMsg{Button: tea.MouseButtonWheelDown} }
func wheelUp() tea.MouseMsg   { return tea.MouseMsg{Button: tea.MouseButtonWheelUp} }
func clickAt(y int) tea.MouseMsg {
	return tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: y}
}

func selOf(t *testing.T, mm tea.Model) int {
	t.Helper()
	m, ok := mm.(*model)
	if !ok {
		t.Fatalf("model が返らない: %T", mm)
	}
	return m.sel
}

func TestMouseWheelMovesSelection(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 80, 24
	m.stage = stSource
	m.items = []string{"A", "B", "C"}
	m.sel = 1

	if mm, _ := m.updateMouse(wheelDown()); selOf(t, mm) != 2 {
		t.Errorf("ホイール下で進まない: sel=%d", selOf(t, mm))
	}
	if mm, _ := m.updateMouse(wheelDown()); selOf(t, mm) != 0 {
		t.Errorf("末尾の次が先頭に戻らない: sel=%d", selOf(t, mm))
	}
	// 端では ↑/↓ キーと同じく回り込む（キー操作とそろえる）
	if mm, _ := m.updateMouse(wheelUp()); selOf(t, mm) != 2 {
		t.Errorf("先頭の前が末尾に回り込まない: sel=%d", selOf(t, mm))
	}
	if mm, _ := m.updateMouse(wheelUp()); selOf(t, mm) != 1 {
		t.Errorf("ホイール上で戻らない: sel=%d", selOf(t, mm))
	}
}

func TestMouseClickSelectsVolume(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 80, 24
	m.stage = stDest // ヘッダが2行あるステージ
	m.items = []string{"vol1", "vol2", "vol3"}
	m.sel = 0

	row := cursorRow(t, m.View())
	if mm, _ := m.updateMouse(clickAt(row + 2)); selOf(t, mm) != 2 {
		t.Errorf("クリックした項目が選ばれない: sel=%d", selOf(t, mm))
	}
	// ヘッダ（0行目）のクリックでは動かない
	m.sel = 0
	if mm, _ := m.updateMouse(clickAt(0)); selOf(t, mm) != 0 {
		t.Errorf("ヘッダのクリックで選択が動いた: sel=%d", selOf(t, mm))
	}
	// 一覧より下のクリックでも動かない
	if mm, _ := m.updateMouse(clickAt(row + 10)); selOf(t, mm) != 0 {
		t.Errorf("余白のクリックで選択が動いた: sel=%d", selOf(t, mm))
	}
}

func TestMouseClickSelectsOption(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 80, 30
	m.stage = stOptions

	row := cursorRow(t, m.View())
	if mm, _ := m.updateMouse(clickAt(row + 3)); selOf(t, mm) != 3 {
		t.Errorf("照合方法の行が選ばれない: sel=%d", selOf(t, mm))
	}
	// 6 行より下（余白）では動かない
	if mm, _ := m.updateMouse(clickAt(row + 6)); selOf(t, mm) != 3 {
		t.Errorf("余白のクリックで選択が動いた: sel=%d", selOf(t, mm))
	}
}

func TestMouseClickSelectsBrowseEntry(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"s1", "s2", "s3"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := newModel("/Volumes", "", "")
	m.width, m.height = 80, 30
	m.stage = stSourceDir
	m.browseDir, m.browseRoot, m.pickLabel = root, root, "コピー元"

	row := cursorRow(t, m.View())
	if mm, _ := m.updateMouse(clickAt(row + 1)); selOf(t, mm) != 1 {
		t.Errorf("2番目のフォルダが選ばれない: sel=%d", selOf(t, mm))
	}
	// 上のヘッダ（現在のフォルダ・パンくず）では動かない
	if mm, _ := m.updateMouse(clickAt(row - 1)); selOf(t, mm) != 1 {
		t.Errorf("ヘッダのクリックで選択が動いた: sel=%d", selOf(t, mm))
	}
}

// スクロール中（「↑ N 件省略」の行がある）でもクリック位置が合うこと
func TestMouseClickSelectsBrowseEntryScrolled(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := newModel("/Volumes", "", "")
	m.width, m.height = 80, 14 // visible = 14-8-2 = 4 件
	m.stage = stSourceDir
	m.browseDir, m.browseRoot, m.pickLabel = root, root, "コピー元"
	m.sel = 9 // 末尾 → start=6 でスクロールする

	v := m.View()
	if !strings.Contains(v, "件省略") {
		t.Fatalf("スクロール表示になっていない:\n%s", v)
	}
	row := cursorRow(t, v) // sel=9（末尾）の行
	if mm, _ := m.updateMouse(clickAt(row - 2)); selOf(t, mm) != 7 {
		t.Errorf("スクロール中のクリック位置がずれている: sel=%d, want 7", selOf(t, mm))
	}
}

func TestMouseClickSelectsPreviewItem(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 80, 30
	m.stage = stPreview
	m.items = []string{"今すぐ開始する", "設定を戻す"}
	m.sel = 0

	row := cursorRow(t, m.View())
	if mm, _ := m.updateMouse(clickAt(row + 1)); selOf(t, mm) != 1 {
		t.Errorf("「設定を戻す」の行が選ばれない: sel=%d", selOf(t, mm))
	}
	if mm, _ := m.updateMouse(clickAt(row)); selOf(t, mm) != 0 {
		t.Errorf("「今すぐ開始する」の行が選ばれない: sel=%d", selOf(t, mm))
	}

	// ファイル一覧の表示中は決定メニューが無いので動かない
	m.showFileList = true
	m.sel = 0
	if mm, _ := m.updateMouse(clickAt(row + 1)); selOf(t, mm) != 0 {
		t.Errorf("ファイル一覧中のクリックで選択が動いた: sel=%d", selOf(t, mm))
	}

	// タブを切り替えると項目の行が動くが、クリック位置はそれに追従する
	m.showFileList = false
	m.previewTab = 1
	m.plan.Transfer = []string{"modified x.txt"}
	m.sel = 0
	row1 := cursorRow(t, m.View())
	if mm, _ := m.updateMouse(clickAt(row1 + 1)); selOf(t, mm) != 1 {
		t.Errorf("転送対象タブで「設定を戻す」が選べない: sel=%d", selOf(t, mm))
	}
}

func TestMouseIgnoredWhilePromptingOrHelp(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 80, 30
	m.stage = stOptions
	m.sel = 0

	// 入力中はホイールもクリックも無効
	m.prompting = true
	if mm, _ := m.updateMouse(wheelDown()); mm.(*model).sel != 0 {
		t.Errorf("入力中にホイールで動いた: sel=%d", mm.(*model).sel)
	}
	if mm, _ := m.updateMouse(clickAt(5)); mm.(*model).sel != 0 {
		t.Errorf("入力中にクリックで動いた: sel=%d", mm.(*model).sel)
	}

	// ヘルプ表示中も無効
	m.prompting = false
	m.showHelp = true
	if mm, _ := m.updateMouse(clickAt(5)); mm.(*model).sel != 0 {
		t.Errorf("ヘルプ中にクリックで動いた: sel=%d", mm.(*model).sel)
	}
}
