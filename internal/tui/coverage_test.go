package tui

// coverage_test.go - TUI の網羅率を上げるためのテスト。
// 純関数（ヘッダ/フッタ・オプション行・プレビュー/ダッシュボード/完了の各行）と
// 描画（View）、ステージ遷移、フォルダ作成・履歴・ボリューム表示を対象にする。
// rsync を実際に起動する経路（beginTransfer）は呼ばない。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kazpc1215/safersync/internal/core"
)

// tbase は描画に足りる最小の model を返す。
func tbase() *model {
	m := newModel("/Volumes", "", "")
	m.width, m.height = 100, 30
	return m
}

func TestView_eachStage(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(logFile, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		setup func(m *model)
		want  string
	}{
		{"source", func(m *model) { m.stage = stSource; m.loadSourceMenu() }, "コピー元の選択"},
		{"dest", func(m *model) { m.src = tmp; m.stage = stDest; m.loadDestMenu() }, "コピー先の選択"},
		{"sourceDir", func(m *model) { m.stage = stSourceDir; m.browse(tmp, "コピー元", false) }, "現在のフォルダ"},
		{"sourceDirPrompt", func(m *model) {
			m.stage = stSourceDir
			m.browse(tmp, "コピー元", false)
			m.prompting = true
			m.promptLbl = "新規フォルダ名"
		}, "新規フォルダ名"},
		{"destDir", func(m *model) { m.stage = stDestDir; m.browse(tmp, "コピー先", true) }, "現在のフォルダ"},
		{"options", func(m *model) { m.stage = stOptions }, "追加の除外"},
		{"optionsPrompt", func(m *model) {
			m.stage = stOptions
			m.prompting = true
			m.promptLbl = "除外パターンを入力"
		}, "除外パターンを入力"},
		{"planBusy", func(m *model) { m.stage = stPlanBusy }, "差分をスキャン"},
		{"verifyBusy", func(m *model) { m.stage = stVerifyBusy }, "照合をしています"},
		{"preview", func(m *model) {
			m.stage = stPreview
			m.src, m.dst = tmp, tmp
			m.plan = core.Plan{Files: 1, Bytes: 10}
			m.items = []string{"a", "b", "c"}
		}, "差分スキャン結果"},
		{"previewTab1", func(m *model) {
			m.stage = stPreview
			m.plan = core.Plan{Transfer: []string{">f 1 x"}}
			m.items = []string{"a", "b", "c"}
			m.previewTab = 1
		}, "転送対象"},
		{"previewTab2", func(m *model) {
			m.stage = stPreview
			m.plan = core.Plan{Skipped: []string{".f 1 y"}}
			m.items = []string{"a", "b", "c"}
			m.previewTab = 2
		}, "スキップ/その他"},
		{"previewFileList", func(m *model) {
			m.stage = stPreview
			m.plan = core.Plan{Files: 1, Lines: []string{">f 1 x"}}
			m.items = []string{"a", "b", "c"}
			m.showFileList = true
		}, "転送対象ファイル一覧"},
		{"transfer", func(m *model) { m.stage = stTransfer; m.startAt = time.Now() }, "ファイル転送中"},
		{"doneOK", func(m *model) {
			m.stage = stDone
			m.finalRC = 0
			m.plan = core.Plan{Files: 2}
			m.xferFiles = 2
			m.logFile = logFile
		}, "正常に完了"},
		{"doneErr", func(m *model) {
			m.stage = stDone
			m.finalRC = 1
			m.plan = core.Plan{Files: 2}
			m.xferRC = 1
			m.logFile = logFile
		}, "エラーが発生"},
		{"message", func(m *model) { m.message("テスト通知", stOptions) }, "テスト通知"},
		{"help", func(m *model) { m.showHelp = true }, "ヘルプ"},
		{"log", func(m *model) { m.logFile = logFile; m.showLog = true }, "ログ"},
		{"logEmpty", func(m *model) { m.logFile = filepath.Join(t.TempDir(), "none"); m.showLog = true }, "（ログはまだありません）"},
		{"small", func(m *model) { m.width, m.height = 20, 5 }, "ターミナル"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := tbase()
			c.setup(m)
			got := m.View()
			if got == "" {
				t.Fatal("View() が空")
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("View() に %q が無い:\n%s", c.want, got)
			}
		})
	}
}

func TestPreviewBodyLines_fileListEmpty(t *testing.T) {
	m := tbase()
	m.showFileList = true
	m.plan = core.Plan{} // Lines 空
	body := strings.Join(m.previewBodyLines(), "\n")
	if !strings.Contains(body, "転送対象はありません") {
		t.Errorf("空のファイル一覧の案内が無い:\n%s", body)
	}
}

func TestPreviewHeaderLines_warnings(t *testing.T) {
	m := tbase()
	m.spaceOK = false
	m.mounted = false
	m.warnMP = true
	m.src, m.dst = "/src", "/dst"
	m.srcScan = core.ScanResult{Files: 5, Bytes: 500}
	m.plan = core.Plan{Files: 2, Bytes: 100}
	body := strings.Join(m.previewHeaderLines(), "\n")
	for _, want := range []string{
		"空き容量が不足しています",
		"マウントされていません",
		"ボリュームの直下です",
		"転送対象: 2",
		"スキップ: 3",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("previewHeaderLines に %q が無い:\n%s", want, body)
		}
	}
}

func TestDashboardLines_variants(t *testing.T) {
	m := tbase()
	m.startAt = time.Now()
	m.frame = frameMsg{pct: 50, eta: -1, file: ""}
	body := strings.Join(m.dashboardLines(), "\n")
	if !strings.Contains(body, "ファイル名を取得中") {
		t.Errorf("ファイル名未取得の案内が無い:\n%s", body)
	}
	if !strings.Contains(body, "残り --") {
		t.Errorf("ETA 未取得は -- のはず:\n%s", body)
	}
	m.confirmCancel = true
	if s := strings.Join(m.dashboardLines(), "\n"); !strings.Contains(s, "もう一度 Ctrl-C") {
		t.Errorf("中断確認の表示が無い:\n%s", s)
	}
	m.confirmCancel = false
	m.frame = frameMsg{pct: 10, eta: 90, file: "a.txt", done: 5, total: 50, speed: 1}
	if s := strings.Join(m.dashboardLines(), "\n"); !strings.Contains(s, "約 1分30秒") {
		t.Errorf("ETA 表示が無い:\n%s", s)
	}
}

func TestDoneLines_variants(t *testing.T) {
	m := tbase()
	m.finalRC = 1
	m.plan = core.Plan{Files: 3}
	m.xferRC = 1
	m.xferFiles = 1
	m.logFile = "/tmp/x.log"
	body := strings.Join(m.doneLines(), "\n")
	for _, want := range []string{"エラーが発生", "再試行"} {
		if !strings.Contains(body, want) {
			t.Errorf("doneLines に %q が無い:\n%s", want, body)
		}
	}

	m2 := tbase()
	m2.finalRC = 130
	m2.interrupt = true
	if s := strings.Join(m2.doneLines(), "\n"); !strings.Contains(s, "中断しました") {
		t.Errorf("中断の表示が無い:\n%s", s)
	}

	m3 := tbase()
	m3.finalRC = 0
	m3.plan = core.Plan{Files: 0}
	if s := strings.Join(m3.doneLines(), "\n"); !strings.Contains(s, "すべて最新") {
		t.Errorf("転送なしの表示が無い:\n%s", s)
	}
}

func TestOptionLine_variants(t *testing.T) {
	m := tbase()
	m.caffeineAvail = false
	if s := m.optionLine(4); !strings.Contains(s, "利用不可") {
		t.Errorf("caffeinate 利用不可の表示が無い: %q", s)
	}
	m.caffeineAvail = true
	m.optCaffeinate = true
	if s := m.optionLine(4); !strings.Contains(s, "スリープ防止") {
		t.Errorf("optionLine(4) = %q", s)
	}
	if s := m.optionLine(5); !strings.Contains(s, "（なし）") {
		t.Errorf("除外なしの表示が無い: %q", s)
	}
	m.extraExcludes = []string{"*.raw", "*.bak"}
	if s := m.optionLine(5); !strings.Contains(s, "*.raw") {
		t.Errorf("追加除外の表示が無い: %q", s)
	}
	if s := m.optionLine(0); !strings.Contains(s, "LRV") {
		t.Errorf("optionLine(0) = %q", s)
	}
	if s := m.optionLine(2); !strings.Contains(s, "一時ファイル") {
		t.Errorf("optionLine(2) = %q", s)
	}
	if s := m.optionLine(3); !strings.Contains(s, "照合方法") {
		t.Errorf("optionLine(3) = %q", s)
	}
	if s := m.optionLine(99); s != "" {
		t.Errorf("範囲外は空のはず: %q", s)
	}
}

func TestHeaderFooterOf(t *testing.T) {
	m := tbase()
	m.stage = stSource
	if m.headerOf() == "" || m.footerOf() == "" {
		t.Error("stSource のヘッダ/フッタが空")
	}
	m.stage = stDest
	if !strings.Contains(m.headerOf(), "保存先") {
		t.Errorf("stDest のヘッダ = %q", m.headerOf())
	}
	for _, st := range []stage{stSourceDir, stDestDir, stOptions, stPreview, stDone} {
		m.stage = st
		if m.footerOf() == "" {
			t.Errorf("stage %v のフッタが空", st)
		}
	}
	m.stage = stTransfer
	if m.headerOf() != "" || m.footerOf() != "" {
		t.Errorf("stTransfer はヘッダ/フッタ空のはず: %q / %q", m.headerOf(), m.footerOf())
	}
}

func TestPreviewItems(t *testing.T) {
	m := tbase()
	m.spaceOK, m.mounted = true, true
	if it := m.previewItems(); len(it) != 3 || !strings.Contains(it[0], "開始") {
		t.Errorf("開始可能時: %v", it)
	}
	m.spaceOK = false
	if it := m.previewItems(); !strings.Contains(it[0], "開始できません") {
		t.Errorf("開始不可時: %v", it)
	}
}

func TestCheckSrcAndDst(t *testing.T) {
	m := tbase()
	dir := t.TempDir()
	if e := m.checkSrc(dir); e != "" {
		t.Errorf("存在するフォルダは OK のはず: %q", e)
	}
	if e := m.checkSrc(filepath.Join(dir, "nope")); e == "" {
		t.Error("存在しないフォルダはエラーのはず")
	}

	m.src = dir
	if e := m.checkDst(dir); !strings.Contains(e, "同じ") {
		t.Errorf("同一はエラーのはず: %q", e)
	}
	if e := m.checkDst(filepath.Join(dir, "sub")); !strings.Contains(e, "コピー元の中") {
		t.Errorf("内側はエラーのはず: %q", e)
	}
	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if e := m.checkDst(f); !strings.Contains(e, "フォルダではありません") {
		t.Errorf("ファイルはエラーのはず: %q", e)
	}
	if e := m.checkDst(t.TempDir()); e != "" {
		t.Errorf("正常な dst は OK のはず: %q", e)
	}
}

func TestVolumeLabelAndDestVolumes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "volA"), 0o755); err != nil {
		t.Fatal(err)
	}
	lbl := volumeLabel(volumeItem{path: filepath.Join(root, "volA"), note: "(外付け)"})
	if !strings.Contains(lbl, "volA") || !strings.Contains(lbl, "(外付け)") {
		t.Errorf("volumeLabel = %q", lbl)
	}
	if l := volumeLabel(volumeItem{path: "/", note: "内蔵ディスク", internal: true}); l == "" {
		t.Error("volumeLabel(/) が空")
	}

	// destVolumes は "/" と基点直下のボリュームを含み、同じボリュームには印を付ける
	src := filepath.Join(root, "src-nope")
	vols := destVolumes(root, src)
	var hasVolA, marked bool
	for _, v := range vols {
		if filepath.Base(v.path) == "volA" {
			hasVolA = true
			if strings.Contains(v.note, "同じボリューム") {
				marked = true
			}
		}
	}
	if !hasVolA {
		t.Errorf("destVolumes に volA が無い: %+v", vols)
	}
	if !marked {
		t.Errorf("同一ボリュームの印が付かない: %+v", vols)
	}
}

func TestHistoryRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if s, d := loadHistory(); s != "" || d != "" {
		t.Errorf("初期は空のはず: %q %q", s, d)
	}
	saveHistory("", "/dst") // src 空は保存しない
	if s, _ := loadHistory(); s != "" {
		t.Errorf("src 空は保存しないはず: %q", s)
	}
	saveHistory("/src", "/dst")
	if s, d := loadHistory(); s != "/src" || d != "/dst" {
		t.Errorf("roundtrip = %q %q", s, d)
	}
	if err := os.WriteFile(historyPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, d := loadHistory(); s != "" || d != "" {
		t.Errorf("壊れた JSON は空のはず: %q %q", s, d)
	}
}

func TestBrowseListWindowScroll(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 30; i++ {
		if err := os.MkdirAll(filepath.Join(root, "d"+itoa(i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := tbase()
	m.height = 20
	m.stage = stSourceDir
	m.browse(root, "コピー元", false)
	m.sel = 25
	entries, _, start, visible := m.browseListWindow()
	if visible < 3 {
		t.Errorf("visible = %d, want >= 3", visible)
	}
	if start <= 0 {
		t.Errorf("start = %d, want > 0（スクロールすること）", start)
	}
	if start+visible > len(entries) {
		t.Errorf("start+visible = %d が件数 %d を超える", start+visible, len(entries))
	}
}

func TestCreateDir(t *testing.T) {
	root := t.TempDir()
	p, err := createDir(root, "sub/2024")
	if err != nil || p != filepath.Join(root, "sub", "2024") {
		t.Fatalf("相対: %q %v", p, err)
	}
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		t.Fatalf("相対のフォルダが作られていない: %v", err)
	}
	abs := filepath.Join(t.TempDir(), "abs")
	if p, err := createDir(root, abs); err != nil || p != abs {
		t.Fatalf("絶対: %q %v", p, err)
	}
	t.Setenv("HOME", root)
	home, _ := os.UserHomeDir()
	if p, err := createDir(root, "~/tilde"); err != nil || p != filepath.Join(home, "tilde") {
		t.Fatalf("チルダ: %q %v (home=%s)", p, err, home)
	}
}

func TestModel_planReadySuccess(t *testing.T) {
	m := tbase()
	m.stage = stPlanBusy
	m.dst = t.TempDir()
	nm, _ := m.Update(planReadyMsg{srcFiles: 5, srcBytes: 100, plan: core.Plan{Files: 2, Bytes: 40}})
	gm := nm.(*model)
	if gm.stage != stPreview {
		t.Fatalf("stage = %v, want stPreview", gm.stage)
	}
	if gm.srcScan.Files != 5 || gm.srcScan.Bytes != 100 {
		t.Errorf("srcScan = %+v", gm.srcScan)
	}
	if len(gm.items) != 3 {
		t.Errorf("items = %v", gm.items)
	}
}

func TestModel_planReadyError(t *testing.T) {
	m := tbase()
	m.stage = stPlanBusy
	nm, _ := m.Update(planReadyMsg{plan: core.Plan{Err: errors.New("boom")}})
	gm := nm.(*model)
	if gm.stage != stMessage {
		t.Fatalf("stage = %v, want stMessage", gm.stage)
	}
}

func TestModel_transferDoneFailure(t *testing.T) {
	m := tbase()
	m.stage = stTransfer
	m.startAt = time.Now()
	nm, _ := m.Update(transferDoneMsg{rc: 1})
	gm := nm.(*model)
	if gm.stage != stDone {
		t.Fatalf("stage = %v, want stDone", gm.stage)
	}
	if gm.finalRC != 1 || gm.verifyRC != 1 {
		t.Errorf("rc: final=%d verify=%d", gm.finalRC, gm.verifyRC)
	}
}

func TestModel_transferDoneSuccessStartsVerify(t *testing.T) {
	m := tbase()
	m.stage = stTransfer
	nm, cmd := m.Update(transferDoneMsg{rc: 0})
	gm := nm.(*model)
	if gm.stage != stVerifyBusy {
		t.Fatalf("stage = %v, want stVerifyBusy", gm.stage)
	}
	if cmd == nil {
		t.Error("照合用の Cmd が返らない")
	}
}

func TestModel_verifyDone(t *testing.T) {
	m := tbase()
	m.stage = stVerifyBusy
	m.xferRC = 1
	m.startAt = time.Now()
	nm, _ := m.Update(verifyDoneMsg{rc: 1, output: "x"})
	gm := nm.(*model)
	if gm.stage != stDone {
		t.Fatalf("stage = %v, want stDone", gm.stage)
	}
	if gm.finalRC != 1 {
		t.Errorf("finalRC = %d, want 1（コピー失敗を優先）", gm.finalRC)
	}
	if gm.verifyOut != "x" {
		t.Errorf("verifyOut = %q", gm.verifyOut)
	}
}

func TestModel_messageEnterReturns(t *testing.T) {
	m := tbase()
	m.message("通知", stOptions)
	if m.stage != stMessage {
		t.Fatalf("stage = %v, want stMessage", m.stage)
	}
	m = stepKey(m, enter())
	if m.stage != stOptions {
		t.Errorf("stage = %v, want stOptions", m.stage)
	}
	if m.msg != "" {
		t.Errorf("msg = %q, want 空", m.msg)
	}
}

func TestModel_qQuitAtPreview(t *testing.T) {
	m := tbase()
	m.stage = stPreview
	m.sel = 2
	nm, cmd := m.Update(key("q"))
	gm := nm.(*model)
	if cmd == nil {
		t.Error("q で終了コマンドが返らない")
	}
	if gm.finalRC != 0 {
		t.Errorf("finalRC = %d, want 0", gm.finalRC)
	}
}

func TestModel_escBack(t *testing.T) {
	cases := []struct{ from, want stage }{
		{stDest, stSource},
		{stOptions, stDest},
		{stPreview, stOptions},
		{stSourceDir, stSource},
		{stDestDir, stDest},
	}
	for _, c := range cases {
		m := tbase()
		m.stage = c.from
		m.src = t.TempDir()
		m.dst = t.TempDir()
		m = stepKey(m, esc())
		if m.stage != c.want {
			t.Errorf("%v: esc → %v, want %v", c.from, m.stage, c.want)
		}
	}
	m := tbase()
	m.stage = stDone
	_, cmd := m.Update(esc())
	if cmd == nil {
		t.Error("stDone の esc は終了コマンドのはず")
	}
}

func TestModel_optionsEnterStartsPlan(t *testing.T) {
	m := tbase()
	m.stage = stOptions
	m.src, m.dst = t.TempDir(), t.TempDir()
	nm, cmd := m.Update(enter())
	gm := nm.(*model)
	if gm.stage != stPlanBusy {
		t.Fatalf("stage = %v, want stPlanBusy", gm.stage)
	}
	if cmd == nil {
		t.Error("差分スキャンの Cmd が返らない")
	}
}

func TestModel_optionsSpaceToggles(t *testing.T) {
	m := tbase()
	m.stage = stOptions
	m.sel = 0
	lrv := m.optLRV
	m = stepKey(m, space())
	if m.optLRV == lrv {
		t.Error("Space で optLRV が切り替わらない")
	}
	m.sel = 2
	tmp := m.optTMP
	m = stepKey(m, space())
	if m.optTMP == tmp {
		t.Error("Space で optTMP が切り替わらない")
	}
	m.sel = 5
	m = stepKey(m, space())
	if !m.prompting || m.promptKind != pkExclude {
		t.Errorf("sel=5 の Space は除外入力のはず: prompting=%v kind=%v", m.prompting, m.promptKind)
	}
}

func TestModel_previewTabCycles(t *testing.T) {
	m := tbase()
	m.stage = stPreview
	m.items = []string{"a", "b", "c"}
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.previewTab != 1 {
		t.Errorf("previewTab = %d, want 1", m.previewTab)
	}
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.previewTab != 0 {
		t.Errorf("previewTab = %d, want 0", m.previewTab)
	}
}

func TestModel_previewBackToOptions(t *testing.T) {
	m := tbase()
	m.stage = stPreview
	m.items = []string{"a", "b", "c"}
	m.sel = 1
	m = stepKey(m, enter())
	if m.stage != stOptions {
		t.Errorf("sel=1 の Enter は設定へ戻るはず: %v", m.stage)
	}
}

func TestModel_previewCancelQuits(t *testing.T) {
	m := tbase()
	m.stage = stPreview
	m.items = []string{"a", "b", "c"}
	m.sel = 2
	nm, cmd := m.Update(enter())
	gm := nm.(*model)
	if cmd == nil {
		t.Error("sel=2 の Enter は終了コマンドのはず")
	}
	if gm.finalRC != 0 {
		t.Errorf("finalRC = %d, want 0", gm.finalRC)
	}
}

func TestModel_promptCreateDir(t *testing.T) {
	root := t.TempDir()
	m := tbase()
	m.stage = stDestDir
	m.browse(root, "コピー先", true)
	m.sel = 1
	m = stepKey(m, key("n"))
	if !m.prompting || m.promptKind != pkNew {
		t.Fatalf("n で新規入力にならない: %v %v", m.prompting, m.promptKind)
	}
	for _, r := range "backup" {
		m = stepKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = stepKey(m, enter())
	if m.prompting {
		t.Error("Enter でプロンプトが閉じない")
	}
	if m.browseDir != filepath.Join(root, "backup") {
		t.Errorf("browseDir = %q, want %q", m.browseDir, filepath.Join(root, "backup"))
	}
	if fi, err := os.Stat(m.browseDir); err != nil || !fi.IsDir() {
		t.Error("フォルダが作られていない")
	}
}

func TestModel_promptFilter(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"apple", "banana"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := tbase()
	m.stage = stSourceDir
	m.browse(root, "コピー元", false)
	m = stepKey(m, key("/"))
	if m.promptKind != pkFilter {
		t.Fatalf("kind = %v, want pkFilter", m.promptKind)
	}
	for _, r := range "app" {
		m = stepKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = stepKey(m, enter())
	if m.browseFilter != "app" {
		t.Errorf("browseFilter = %q", m.browseFilter)
	}
	items := browseEntriesAdvanced(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate, m.showHidden, m.browseFilter)
	dirs := 0
	for _, it := range items {
		if it.kind == pickDir {
			dirs++
		}
	}
	if dirs != 1 {
		t.Errorf("フィルタ後のフォルダ数 = %d, want 1", dirs)
	}
}

func TestMouse_ignoredWhilePrompting(t *testing.T) {
	m := tbase()
	m.stage = stSource
	m.items = []string{"a", "b", "c"}
	m.prompting = true
	m.sel = 0
	m.updateMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.sel != 0 {
		t.Errorf("入力中はホイール無効のはず: sel=%d", m.sel)
	}
	m.prompting = false
	m.updateMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.sel != 1 {
		t.Errorf("ホイールで移動するはず: sel=%d", m.sel)
	}
}
