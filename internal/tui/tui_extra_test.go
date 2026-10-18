package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kazpc1215/safersync/internal/core"
)

func TestBrowseEntriesAdvanced_filterAndHidden(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", ".hiddenDir", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// フィルタなし・隠し非表示
	items := browseEntriesAdvanced(root, root, "コピー元", false, false, "")
	names := []string{}
	for _, it := range items {
		if it.kind == pickDir {
			names = append(names, it.label)
		}
	}
	if len(names) != 2 {
		t.Fatalf("隠し非表示時は2件のはず: %v", names)
	}
	// 隠し表示
	items2 := browseEntriesAdvanced(root, root, "コピー元", false, true, "")
	names2 := []string{}
	for _, it := range items2 {
		if it.kind == pickDir {
			names2 = append(names2, it.label)
		}
	}
	if len(names2) != 3 {
		t.Fatalf("隠し表示時は3件のはず: %v", names2)
	}
	// フィルタ
	items3 := browseEntriesAdvanced(root, root, "コピー元", false, true, "alp")
	names3 := []string{}
	for _, it := range items3 {
		if it.kind == pickDir {
			names3 = append(names3, it.label)
		}
	}
	if len(names3) != 1 || !strings.Contains(names3[0], "alpha") {
		t.Fatalf("フィルタが効いていない: %v", names3)
	}
}

func TestModel_browseHiddenAndFilterToggle(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "vols")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(root, "", "")
	m.loadSourceMenu()
	m = stepKey(m, enter()) // "/" を選択 → browse
	if m.stage != stSourceDir {
		t.Fatalf("browse に遷移しない: %d", m.stage)
	}
	// h で隠し表示トグル
	before := m.showHidden
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.showHidden == before {
		t.Error("h で隠し表示が切り替わらない")
	}
	// / でフィルタ入力開始
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.prompting {
		t.Error("/ でフィルタ入力が始まらない")
	}
	// 文字入力 + Enter
	for _, r := range "hid" {
		m = stepKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.browseFilter != "hid" {
		t.Errorf("フィルタが保存されない: %q", m.browseFilter)
	}
}

func TestModel_ctrlCCancelConfirmation(t *testing.T) {
	m := newModel("/tmp", "", "")
	m.width, m.height = 80, 24
	// 1回目は確認状態になる
	m2 := stepKey(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m2.confirmCancel {
		t.Error("1回目の Ctrl-C で確認状態にならない")
	}
	// 2回目で中断
	m3 := stepKey(m2, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m3.interrupt {
		t.Error("2回目の Ctrl-C で中断されない")
	}
	if m3.finalRC != core.ExitInterrupted {
		t.Errorf("中断の終了コードは %d のはず: %d", core.ExitInterrupted, m3.finalRC)
	}
}

// 外部シグナル（SIGTERM / SIGHUP）も安全中断（130）として扱うこと
func TestModel_interruptRequest(t *testing.T) {
	m := newModel("/tmp", "", "")
	m.width, m.height = 80, 24
	m2, _ := m.Update(interruptRequestMsg{})
	fm := m2.(*model)
	if !fm.interrupt || fm.finalRC != core.ExitInterrupted {
		t.Errorf("シグナル受信で中断状態にならない: interrupt=%v finalRC=%d", fm.interrupt, fm.finalRC)
	}
}

// 差分を確認できなかったときは、開始できずに案内を出すこと
func TestModel_planErrorBlocksStart(t *testing.T) {
	m := newModel("/tmp", "", "")
	m.width, m.height = 80, 24
	m.stage = stPlanBusy
	m.prog = nil
	m2, _ := m.Update(planReadyMsg{plan: core.Plan{Err: errors.New("rsync を実行できませんでした")}})
	fm := m2.(*model)
	if fm.stage != stMessage {
		t.Errorf("エラー時はメッセージ画面になるはず: stage=%d", fm.stage)
	}
	if !strings.Contains(fm.msg, "差分を確認できませんでした") {
		t.Errorf("案内メッセージが無い: %q", fm.msg)
	}
}
