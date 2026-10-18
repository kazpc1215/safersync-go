package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kazpc1215/safersync/internal/core"
)

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func enter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func esc() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEsc} }
func space() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeySpace} }
func down() tea.KeyMsg  { return tea.KeyMsg{Type: tea.KeyDown} }
func up() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyUp} }

// stepKey は Update にキーを渡して返す。
func stepKey(m *model, k tea.KeyMsg) *model {
	nm, _ := m.Update(k)
	return nm.(*model)
}

func opts() *core.Options { return core.NewOptions() }

func TestBrowseEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 起点 = root: ".." は無い。隠しも無い。予約3項目は常に有る
	items := browseEntries(root, root, "コピー元", false)
	if items[0].kind != pickDir || items[0].label != "a/" {
		t.Errorf("先頭に a/ が無い: %+v", items)
	}
	for _, it := range items {
		if strings.HasPrefix(it.label, ".") {
			t.Errorf("隠しフォルダが出ている: %+v", it)
		}
	}
	if items[len(items)-1].kind != pickSelect {
		t.Errorf("最後に「選択」項目が必要: %+v", items[len(items)-1])
	}

	// 下に入ると ".." が出る
	items2 := browseEntries(filepath.Join(root, "a"), root, "コピー元", false)
	if items2[0].kind != pickUp || items2[0].path != root {
		t.Errorf("..\n が無い/誤り: %+v", items2)
	}
}

func TestFrameJapaneseWidth(t *testing.T) {
	// 日本語（2桁）を含む行でも、枠の幅がそろうこと
	w := 40
	lines := []string{"日本語テスト", "abc", "マルチバイトと ascii mix"}
	out := frame(w, "タイトル", "ヘッダ\n複数行", lines, "フッタ日本語")
	got := strings.Split(out, "\n")
	for _, l := range got {
		if lw := lipgloss.Width(l); lw != w {
			t.Errorf("行の幅が不一致: %q (want %d got %d)", l, w, lw)
		}
	}
}

func TestBoxRowTruncateJapanese(t *testing.T) {
	s := strings.Repeat("あ", 20) + "あい"
	row := boxRow(10, s)
	if lw := lipgloss.Width(row); lw != 10 {
		t.Errorf("切り詰め後の幅が 10 でない: %d (%q)", lw, row)
	}
}

func TestSourceVolumes_excludesSymlinks(t *testing.T) {
	// ダミー基点: リンクは除外され、"実フォルダ" だけ入る
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "realvol"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "linked")); err != nil {
		t.Skip("symlink を作れない環境")
	}
	vols := sourceVolumes(root)
	var got []string
	for _, v := range vols {
		got = append(got, v.path)
	}
	for _, want := range []string{"/", filepath.Join(root, "realvol")} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q in %v", want, got)
		}
	}
	for _, g := range got {
		if g == filepath.Join(root, "linked") {
			t.Errorf("シンボリックリンクが除外されていない: %v", got)
		}
	}
}

func TestModel_sourceFlow(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "vol", "DCIM"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tmp, "vols")
	if err := os.MkdirAll(filepath.Join(root, "HDD"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(root, "", "")
	// Clone of tui.go init menu assembly:
	m.loadSourceMenu()
	m.width, m.height = 80, 24

	// 1) ボリューム選択: 先頭 "/"（内蔵）を Enter
	m = stepKey(m, enter())
	if m.stage != stSourceDir {
		t.Fatalf("ステージがフォルダ選択に遷移しない: %d", m.stage)
	}
	if m.browseRoot != "/" {
		t.Errorf("browseRoot=/ のはず: %q", m.browseRoot)
	}

	// 2) ".." は無い（"/" が起点）。 ESC で戻る
	got := browseEntries(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate)
	if got[0].kind == pickUp {
		t.Errorf("/ では .. を出さない")
	}
	m = stepKey(m, esc())
	if m.stage != stSource {
		t.Errorf("Esc でボリューム選択に戻るはず")
	}
}

func TestModel_browseCursorMoves(t *testing.T) {
	// フォルダ選択画面で ↑/↓ でカーソルが動くこと（回帰: m.items を参照して動かなかった）
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "vol", "DCIM"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tmp, "vols")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(root, "", "")
	m = stepKey(m, enter()) // 先頭 "/"（内蔵）を Enter
	if m.stage != stSourceDir {
		t.Fatalf("ステージがフォルダ選択に遷移しない: %d", m.stage)
	}
	n := len(browseEntries(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate))
	if n < 4 {
		t.Fatalf("フォルダ選択の項目が足りない: %d", n)
	}
	if m.sel != 0 {
		t.Fatalf("初期カーソルは0のはず: %d", m.sel)
	}
	m = stepKey(m, down())
	if m.sel != 1 {
		t.Errorf("↓ でカーソルが動かない: sel=%d", m.sel)
	}
	m = stepKey(m, key("j"))
	if m.sel != 2 {
		t.Errorf("j でカーソルが動かない: sel=%d", m.sel)
	}
	m = stepKey(m, up())
	if m.sel != 1 {
		t.Errorf("↑ でカーソルが動かない: sel=%d", m.sel)
	}
}

func TestModel_destCreateAndOptionToggle(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "vols")
	if err := os.MkdirAll(filepath.Join(root, "HDD", "DCIM"), 0o755); err != nil {
		t.Fatal(err)
	}
	// --from プリフィル → vol/dst へ
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(root, src, "")
	if m.stage != stDest {
		t.Fatalf("プリフィルでコピー先選択から始まるはず: %d", m.stage)
	}
	m.loadDestMenu()

	// 最初のボリューム（"/"）を選択 → フォルダ選択
	m = stepKey(m, enter())
	if m.stage != stDestDir {
		t.Fatalf("destDir へ遷移していない: %d", m.stage)
	}
	if !m.pickCreate {
		t.Error("コピー先では新規フォルダ作成が許可のはず")
	}

	// 「このフォルダをコピー先にする」: items のラベルをひとつ進めて Enter → こういう細い検査は
	// browseEntries の予約3項目が末尾固定という仕様に依存する
	entries := browseEntries(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate)
	var selectIdx int
	for i, it := range entries {
		if it.kind == pickSelect {
			selectIdx = i
		}
	}
	m2 := m
	m2.sel = selectIdx
	m2 = stepKey(m2, enter())
	if m2.stage != stOptions {
		t.Fatalf("オプションへ遷移していない: %d", m2.stage)
	}

	// Space でオプションをトグル
	before := m2.optLRV
	m3 := stepKey(m2, space())
	if m3.optLRV == before {
		t.Error("Space で .LRV のトグルができていない")
	}
	// ESC で他ボリューム選択へ戻る
	m4 := stepKey(m3, esc())
	if m4.stage != stDest {
		t.Errorf("options からの Esc は dest に戻る: %d", m4.stage)
	}
}

func TestExcludes(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.optLRV = true
	m.optTHM = true
	m.optTMP = false
	got := m.excludes()
	want := "*.LRV *.THM"
	if strings.Join(got, " ") != want {
		t.Errorf("want %q got %q", want, strings.Join(got, " "))
	}
	m.optTMP = true
	if !strings.Contains(m.excludesSummary(), "*.tmp") {
		t.Logf("summary: %s", m.excludesSummary())
	}
	_ = opts
}
