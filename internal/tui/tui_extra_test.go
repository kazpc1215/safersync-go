package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

// プロンプト入力はマルチバイト（日本語 IME）や貼り付けも受け付けること
func TestPrompt_multibyteInput(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 100, 30
	m.stage = stDestDir
	m.prompting = true
	m.promptKind = pkNew
	m.browseDir = t.TempDir()

	m = stepKey(m, key("あいう"))
	if m.prompt != "あいう" {
		t.Errorf("日本語入力が反映されていない: %q", m.prompt)
	}
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.prompt != "あい" {
		t.Errorf("backspace が rune 単位で消していない: %q", m.prompt)
	}
	m = stepKey(m, key("x y")) // 貼り付け相当（複数 rune の一括入力）
	m = stepKey(m, space())
	if m.prompt != "あいx y " {
		t.Errorf("複数 rune・スペースの入力が壊れた: %q", m.prompt)
	}
}

// プロンプト中の q / g / ? は画面操作に奪われず、文字として入力されること
func TestPrompt_qgQuestionAreText(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 100, 30
	m.stage = stDestDir
	m.prompting = true
	m.prompt = ""

	for _, s := range []string{"q", "g", "?"} {
		m = stepKey(m, key(s))
	}
	if m.prompt != "qg?" {
		t.Errorf("q/g/? が文字として入力されていない: %q", m.prompt)
	}
	if m.showLog {
		t.Error("プロンプト中に g でログ画面が開いた")
	}
	if m.showHelp {
		t.Error("プロンプト中に ? でヘルプが開いた")
	}
}

// プロンプト中でないときは q / g / ? は従来どおり画面操作として動くこと
func TestPrompt_globalKeysStillWork(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 100, 30
	m.stage = stSource

	if m2 := stepKey(m, key("?")); !m2.showHelp {
		t.Error("非プロンプト時の ? でヘルプが開かない")
	}
	if m2 := stepKey(m, key("g")); !m2.showLog {
		t.Error("非プロンプト時の g でログが開かない")
	}
}

// オプション画面は 6 項目すべて（追加の除外パターンを含む）を描画すること
func TestOptionsView_allLines(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 100, 30
	m.stage = stOptions
	m.sel = 5

	v := m.View()
	if !strings.Contains(v, "追加の除外") {
		t.Errorf("オプション画面に「追加の除外」行が描画されていない:\n%s", v)
	}
	for i := 0; i < 6; i++ {
		if m.optionLine(i) == "" {
			t.Errorf("optionLine(%d) が空（項目数の不一致）", i)
		}
	}
}

// オプション画面のプロンプト（追加除外パターンの入力）が描画されること
func TestOptionsView_promptVisible(t *testing.T) {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 100, 30
	m.stage = stOptions
	m.prompting = true
	m.prompt = "テスト"
	m.promptLbl = "追加で除外するパターンを入力（例: *.tmp）。空でキャンセル"

	v := m.View()
	if !strings.Contains(v, "追加で除外するパターンを入力") {
		t.Errorf("プロンプトのラベルが描画されていない:\n%s", v)
	}
	if !strings.Contains(v, "テスト") {
		t.Errorf("入力中の文字が描画されていない:\n%s", v)
	}
}

// 差分スキャン/照合中の Ctrl-C（2回目）で、実行中のスキャンも止めること
func TestModel_interruptCancelsScan(t *testing.T) {
	m := newModel("/tmp", "", "")
	m.width, m.height = 80, 24
	m.stage = stPlanBusy
	canceled := false
	m.scanCancel = func() { canceled = true }

	m2 := stepKey(m, tea.KeyMsg{Type: tea.KeyCtrlC}) // 1回目: 確認状態
	if canceled {
		t.Fatal("1回目の Ctrl-C でスキャンがキャンセルされてしまった")
	}
	m3 := stepKey(m2, tea.KeyMsg{Type: tea.KeyCtrlC}) // 2回目: 中断
	if !canceled {
		t.Error("中断時に実行中のスキャンがキャンセルされない")
	}
	if !m3.interrupt || m3.finalRC != core.ExitInterrupted {
		t.Errorf("中断状態にならない: interrupt=%v finalRC=%d", m3.interrupt, m3.finalRC)
	}
}

// 終了時の保険（terminateChild）でもスキャンを止めること
func TestModel_terminateChildCancelsScan(t *testing.T) {
	m := newModel("/tmp", "", "")
	canceled := false
	m.scanCancel = func() { canceled = true }
	m.terminateChild()
	if !canceled {
		t.Error("終了時にスキャンがキャンセルされない")
	}
	// スキャンが無い場合は panic しない
	m2 := newModel("/tmp", "", "")
	m2.terminateChild()
}

// 履歴の保存先は XDG_STATE_HOME があればそれに従う
func TestHistoryPathXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/ss-state")
	t.Setenv("HOME", "/tmp/ss-home")

	if got := historyPath(); got != "/tmp/ss-state/safersync/tui.json" {
		t.Errorf("XDG_STATE_HOME を使うはず: %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "")
	if got := historyPath(); got != "/tmp/ss-home/.local/state/safersync/tui.json" {
		t.Errorf("既定は ~/.local/state のはず: %q", got)
	}
}

// コピー先がコピー元の内側なら決定時にエラーを返す（入れ子コピー防止）。
func TestCheckDst_insideSource(t *testing.T) {
	src := t.TempDir()
	m := newModel("/Volumes", src, "")

	if got := m.checkDst(src); got != "コピー元とコピー先が同じです" {
		t.Errorf("同一パスの検査: %q", got)
	}
	if got := m.checkDst(filepath.Join(src, "backup")); !strings.Contains(got, "コピー元の中") {
		t.Errorf("内側の検査: %q", got)
	}
	if got := m.checkDst(t.TempDir()); got != "" {
		t.Errorf("正常なコピー先を拒否した: %q", got)
	}
}

// SAFERSYNC_THEME / NO_COLOR のテーマ解決（NO_COLOR が最優先、未知は dark）
func TestThemeName(t *testing.T) {
	cases := []struct {
		theme   string
		noColor bool
		want    string
	}{
		{"", false, "dark"},
		{"dark", false, "dark"},
		{"light", false, "light"},
		{"high-contrast", false, "high-contrast"},
		{"unknown", false, "dark"},
		{"light", true, "none"},
		{"high-contrast", true, "none"},
		{"", true, "none"},
	}
	for _, c := range cases {
		if got := themeName(c.theme, c.noColor); got != c.want {
			t.Errorf("themeName(%q, %v) = %q, want %q", c.theme, c.noColor, got, c.want)
		}
	}
}

// NO_COLOR ではテーマに関わらず ANSI を付けない
func TestApplyThemeNoColor(t *testing.T) {
	defer applyTheme("", false) // dark に戻す
	applyTheme("high-contrast", true)
	if got := stTitle.Render("x"); got != "x" {
		t.Errorf("NO_COLOR なのに stTitle が色付き: %q", got)
	}
	if got := stErr.Render("x"); got != "x" {
		t.Errorf("NO_COLOR なのに stErr が色付き: %q", got)
	}
	if !themeNoColor {
		t.Error("themeNoColor が立っていない")
	}
}

// NO_COLOR では進捗バーにも色を付けない（以前はここだけ色が残っていた）
func TestProgressBarNoColor(t *testing.T) {
	defer applyTheme("", false)
	applyTheme("", true)
	got := progressBar(50, 10)
	if strings.Contains(got, "\x1b") {
		t.Errorf("NO_COLOR なのに進捗バーが ANSI を含む: %q", got)
	}
	if got != strings.Repeat("█", 5)+strings.Repeat("░", 5) {
		t.Errorf("NO_COLOR の進捗バー = %q", got)
	}
	if w := lipgloss.Width(got); w != 10 {
		t.Errorf("進捗バーの幅が 10 でない: %d (%q)", w, got)
	}
	// 色ありのときも幅は同じ（色は幅に数えない）
	applyTheme("", false)
	if w := lipgloss.Width(progressBar(0, 10)); w != 10 {
		t.Errorf("色ありの進捗バーの幅が 10 でない: %d", w)
	}
}

// 各テーマが実際に異なる描画になること（色プロファイルを固定して検証）。
// applyTheme を通したスタイルが「色なし（空）」だけになっていないことも確認する。
func TestApplyThemeRendersDistinct(t *testing.T) {
	defer applyTheme("", false) // dark に戻す
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	render := func(theme string, noColor bool) string {
		applyTheme(theme, noColor)
		return stOk.Render("x") + "|" + stErr.Render("x") + "|" + stCursor.Render("x")
	}

	none := render("", true)
	if strings.Contains(none, "\x1b") {
		t.Errorf("NO_COLOR で ANSI が残る: %q", none)
	}

	dark := render("", false)
	light := render("light", false)
	hc := render("high-contrast", false)
	for name, got := range map[string]string{"dark": dark, "light": light, "high-contrast": hc} {
		if !strings.Contains(got, "\x1b") {
			t.Errorf("%s テーマで色が付いていない: %q", name, got)
		}
	}
	if dark == light || dark == hc || light == hc {
		t.Errorf("テーマの描画が区別できない: dark=%q light=%q hc=%q", dark, light, hc)
	}
}
