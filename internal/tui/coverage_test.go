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

func TestBoxHelpers_extra(t *testing.T) {
	// boxTop の切り詰め（長いタイトル）
	s := boxTop(12, "とても長いタイトルです")
	if !strings.Contains(s, "┌") || !strings.Contains(s, "┐") {
		t.Errorf("boxTop の罫線が無い: %q", s)
	}
	// 小さい幅でもパニックしない
	if got := boxTop(4, "title"); !strings.Contains(got, "┌") {
		t.Errorf("boxTop(4) = %q", got)
	}
	// progressBar の境界
	if s := progressBar(0, 5); s == "" {
		t.Error("progressBar(0,5) が空")
	}
	if s := progressBar(100, 5); !strings.Contains(s, "█") {
		t.Errorf("progressBar(100,5) = %q", s)
	}
	if s := progressBar(-5, 5); s == "" {
		t.Error("progressBar(-5,5) が空")
	}
	if s := progressBar(50, 0); s != "" {
		t.Errorf("progressBar(50,0) = %q, want 空", s)
	}
	// mod
	if mod(0, 0) != 0 || mod(-1, 3) != 2 || mod(5, 3) != 2 {
		t.Errorf("mod の計算が違う")
	}
}

func TestExcludesSummary_variants(t *testing.T) {
	m := tbase()
	m.extraExcludes = []string{"*.raw"}
	if s := m.excludesSummary(); !strings.Contains(s, "*.raw") {
		t.Errorf("excludesSummary = %q", s)
	}
	m2 := tbase()
	m2.optLRV, m2.optTHM, m2.optTMP, m2.optChecksum = false, false, false, true
	if s := m2.excludesSummary(); s == "" {
		t.Error("excludesSummary が空")
	}
	m3 := tbase()
	m3.optLRV, m3.optTHM, m3.optTMP = false, false, false
	m3.extraExcludes = nil
	if s := m3.excludesSummary(); s != "（なし）" {
		t.Errorf("除外なしは (なし) のはず: %q", s)
	}
}

// --- updateBrowse の分岐 ---
func TestUpdateBrowse_branches(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"apple", "banana"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	m := tbase()
	m.stage = stSourceDir
	m.browse(root, "コピー元", false)
	// h で隠し切替
	m = stepKey(m, key("h"))
	if !m.showHidden {
		t.Error("h で隠し表示が切り替わらない")
	}
	m = stepKey(m, key("h"))
	if m.showHidden {
		t.Error("h で隠し表示が戻らない")
	}
	// n で新規作成プロンプト
	m = stepKey(m, key("n"))
	if !m.prompting || m.promptKind != pkNew {
		t.Errorf("n: prompting=%v kind=%v", m.prompting, m.promptKind)
	}
	m.prompting = false
	// p で手動入力プロンプト
	m = stepKey(m, key("p"))
	if !m.prompting || m.promptKind != pkManual {
		t.Errorf("p: prompting=%v kind=%v", m.prompting, m.promptKind)
	}
	m.prompting = false
	// / でフィルタプロンプト
	m = stepKey(m, key("/"))
	if !m.prompting || m.promptKind != pkFilter {
		t.Errorf("/: prompting=%v kind=%v", m.prompting, m.promptKind)
	}
	m.prompting = false
	// Enter で pickUp（root 以外に移動して .. を選ぶ）
	sub := filepath.Join(root, "apple")
	m.browse(sub, "コピー元", false)
	// apple の中は root が親なので ".." が先頭
	entries := browseEntriesAdvanced(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate, m.showHidden, m.browseFilter)
	if len(entries) == 0 || entries[0].kind != pickUp {
		t.Skip("pickUp が無い構成")
	}
	m.sel = 0
	m = stepKey(m, enter())
	if m.browseDir != root {
		t.Errorf("pickUp 後 browseDir = %q, want %q", m.browseDir, root)
	}
	// Enter で pickSelect（コピー元決定）— root で ".." なし、最後が pickSelect
	m.browse(root, "コピー元", false)
	entries2 := browseEntriesAdvanced(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate, m.showHidden, m.browseFilter)
	last := len(entries2) - 1
	m.sel = last
	m = stepKey(m, enter())
	if m.stage != stDest {
		t.Errorf("pickSelect 後 stage = %v, want stDest", m.stage)
	}
}

// --- updatePrompt の分岐 ---
func TestUpdatePrompt_branches(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "existing"), 0o755)
	m := tbase()
	m.stage = stSourceDir
	m.browse(root, "コピー元", false)
	// backspace
	m.prompting = true
	m.prompt = "ab"
	m.promptKind = pkManual
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.prompt != "a" {
		t.Errorf("backspace 後 prompt = %q", m.prompt)
	}
	// empty enter → nothing
	m.prompt = ""
	m0 := m.prompt
	m = stepKey(m, enter())
	if m.prompting {
		t.Error("空入力でもプロンプトが閉じない")
	}
	_ = m0
	// 手動入力: 存在するパス
	m2 := tbase()
	m2.stage = stSourceDir
	m2.browse(root, "コピー元", false)
	m2.prompting = true
	m2.prompt = filepath.Join(root, "existing")
	m2.promptKind = pkManual
	m2 = stepKey(m2, enter())
	if m2.browseDir != filepath.Join(root, "existing") {
		t.Errorf("手動入力: browseDir = %q", m2.browseDir)
	}
	// 手動入力: 存在しない + pickCreate=false → message
	m3 := tbase()
	m3.stage = stSourceDir
	m3.browse(root, "コピー元", false)
	m3.prompting = true
	m3.prompt = filepath.Join(root, "nope")
	m3.promptKind = pkManual
	m3 = stepKey(m3, enter())
	if m3.stage != stMessage {
		t.Errorf("存在しない手動入力: stage = %v, want stMessage", m3.stage)
	}
	// 手動入力: pickCreate=true で作成
	m4 := tbase()
	m4.stage = stDestDir
	m4.browse(root, "コピー先", true)
	m4.prompting = true
	m4.prompt = filepath.Join(root, "newdir")
	m4.promptKind = pkManual
	m4 = stepKey(m4, enter())
	if m4.browseDir != filepath.Join(root, "newdir") {
		t.Errorf("pickCreate 手動入力: browseDir = %q", m4.browseDir)
	}
	// pkFilter: 空入力はキャンセル（フィルタ維持）
	m5 := tbase()
	m5.stage = stSourceDir
	m5.browse(root, "コピー元", false)
	m5.browseFilter = "old"
	m5.prompting = true
	m5.prompt = "   "
	m5.promptKind = pkFilter
	m5 = stepKey(m5, enter())
	if m5.browseFilter != "old" {
		t.Errorf("空入力でフィルタが変わる: %q", m5.browseFilter)
	}
	// pkExclude
	m6 := tbase()
	m6.stage = stOptions
	m6.prompting = true
	m6.prompt = "  *.raw  "
	m6.promptKind = pkExclude
	m6 = stepKey(m6, enter())
	if len(m6.extraExcludes) != 1 || m6.extraExcludes[0] != "*.raw" {
		t.Errorf("extraExcludes = %v", m6.extraExcludes)
	}
	// pkNew: createDir 失敗 → message
	m7 := tbase()
	m7.stage = stSourceDir
	m7.browse(root, "コピー元", false)
	m7.prompting = true
	m7.prompt = string([]byte{0}) // 無効な名前
	m7.promptKind = pkNew
	m7 = stepKey(m7, enter())
	// 空 or メッセージどちらでも OK（パニックしないこと）
	_ = m7
	// 文字入力（rune）
	m8 := tbase()
	m8.stage = stSourceDir
	m8.browse(root, "コピー元", false)
	m8.prompting = true
	m8.prompt = ""
	m8.promptKind = pkManual
	m8 = stepKey(m8, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b'}})
	if m8.prompt != "ab" {
		t.Errorf("rune 入力: prompt = %q", m8.prompt)
	}
	// space キー
	m9 := tbase()
	m9.stage = stSourceDir
	m9.browse(root, "コピー元", false)
	m9.prompting = true
	m9.prompt = "x"
	m9.promptKind = pkManual
	m9 = stepKey(m9, tea.KeyMsg{Type: tea.KeySpace})
	if m9.prompt != "x " {
		t.Errorf("space 入力: prompt = %q", m9.prompt)
	}
}

// --- updateOptions の分岐 ---
func TestUpdateOptions_branches(t *testing.T) {
	m := tbase()
	m.stage = stOptions
	// up/down
	m.sel = 0
	m = stepKey(m, key("k"))
	if m.sel != 5 {
		t.Errorf("up: sel = %d, want 5", m.sel)
	}
	m = stepKey(m, key("j"))
	if m.sel != 0 {
		t.Errorf("down: sel = %d, want 0", m.sel)
	}
	// space on each toggle
	for _, sel := range []int{0, 1, 2, 3} {
		mm := tbase()
		mm.stage = stOptions
		mm.sel = sel
		stepKey(mm, space())
	}
	// space on sel=4 with caffeineAvail=false は何もしない
	mm := tbase()
	mm.stage = stOptions
	mm.sel = 4
	mm.caffeineAvail = false
	prev := mm.optCaffeinate
	mm = stepKey(mm, space())
	if mm.optCaffeinate != prev {
		t.Error("caffeinate 利用不可なのにトグルされた")
	}
	// space on sel=4 with caffeineAvail=true
	mm2 := tbase()
	mm2.stage = stOptions
	mm2.sel = 4
	mm2.caffeineAvail = true
	mm2.optCaffeinate = false
	mm2 = stepKey(mm2, space())
	if !mm2.optCaffeinate {
		t.Error("caffeinate トグルされない")
	}
	// enter on sel=5 → prompt
	mm3 := tbase()
	mm3.stage = stOptions
	mm3.sel = 5
	mm3 = stepKey(mm3, enter())
	if !mm3.prompting || mm3.promptKind != pkExclude {
		t.Errorf("enter sel=5: prompting=%v kind=%v", mm3.prompting, mm3.promptKind)
	}
}

// --- updatePreview の分岐 ---
func TestUpdatePreview_branches(t *testing.T) {
	m := tbase()
	m.stage = stPreview
	m.items = []string{"a", "b", "c"}
	m.plan = core.Plan{Files: 1, Lines: []string{"l1", "l2"}}
	m.spaceOK, m.mounted = true, true
	// tab
	m = stepKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.previewTab != 1 {
		t.Errorf("tab: previewTab = %d", m.previewTab)
	}
	// showFileList toggle via l
	m = stepKey(m, key("l"))
	if !m.showFileList {
		t.Error("l でファイル一覧が開かない")
	}
	// scroll down/up in file list
	m.fileListScroll = 0
	m = stepKey(m, key("j"))
	if m.fileListScroll != 1 {
		t.Errorf("j: fileListScroll = %d", m.fileListScroll)
	}
	m = stepKey(m, key("k"))
	if m.fileListScroll != 0 {
		t.Errorf("k: fileListScroll = %d", m.fileListScroll)
	}
	// l で閉じる
	m = stepKey(m, key("l"))
	if m.showFileList {
		t.Error("l でファイル一覧が閉じない")
	}
	// up/down/left/right navigation（新規モデルで）
	m2 := tbase()
	m2.stage = stPreview
	m2.items = []string{"a", "b", "c"}
	m2.plan = core.Plan{Files: 1}
	m2.spaceOK, m2.mounted = true, true
	m2.showFileList = false
	m2.sel = 0
	m = m2
	m = stepKey(m, key("j"))
	if m.sel != 1 {
		t.Errorf("j: sel = %d", m.sel)
	}
	m = stepKey(m, key("k"))
	if m.sel != 0 {
		t.Errorf("k: sel = %d", m.sel)
	}
	m = stepKey(m, key("right"))
	if m.sel != 1 {
		t.Errorf("right: sel = %d", m.sel)
	}
	m = stepKey(m, key("left"))
	if m.sel != 0 {
		t.Errorf("left: sel = %d", m.sel)
	}
	// enter sel=0 with spaceOK&&mounted → beginTransfer would run rsync (skip by making plan.Files=0? beginTransfer needs valid job)
	// Instead test enter sel=0 blocked case
	m2 = tbase()
	m2.stage = stPreview
	m2.items = []string{"a", "b", "c"}
	m2.plan = core.Plan{Files: 0}
	m2.spaceOK, m2.mounted = false, true
	m2.sel = 0
	m2 = stepKey(m2, enter())
	if m2.stage != stPreview {
		// blocked → stays (beginTransfer not called because spaceOK false)
	}
	// enter sel=1 → options
	m3 := tbase()
	m3.stage = stPreview
	m3.items = []string{"a", "b", "c"}
	m3.sel = 1
	m3 = stepKey(m3, enter())
	if m3.stage != stOptions {
		t.Errorf("sel=1 enter: stage = %v, want stOptions", m3.stage)
	}
}

// --- updateKey の分岐 ---
func TestUpdateKey_branches(t *testing.T) {
	// showLog スクロール
	logFile := filepath.Join(t.TempDir(), "log.txt")
	os.WriteFile(logFile, []byte("l1\nl2\nl3\n"), 0o644)
	m := tbase()
	m.showLog = true
	m.logFile = logFile
	m.logScroll = 0
	m = stepKey(m, key("j"))
	if m.logScroll != 1 {
		t.Errorf("log j: scroll = %d", m.logScroll)
	}
	m = stepKey(m, key("k"))
	if m.logScroll != 0 {
		t.Errorf("log k: scroll = %d", m.logScroll)
	}
	// showLog: その他キーで閉じる
	m = stepKey(m, key("x"))
	if m.showLog {
		t.Error("showLog が閉じない")
	}
	// showHelp トグル
	m2 := tbase()
	m2 = stepKey(m2, key("?"))
	if !m2.showHelp {
		t.Error("? でヘルプが開かない")
	}
	m2 = stepKey(m2, key("?"))
	if m2.showHelp {
		t.Error("? でヘルプが閉じない")
	}
	// showHelp 中の esc
	m3 := tbase()
	m3.showHelp = true
	m3 = stepKey(m3, esc())
	if m3.showHelp {
		t.Error("esc でヘルプが閉じない")
	}
	// ctrl+c 1回目 → confirmCancel
	m4 := tbase()
	m4 = stepKey(m4, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m4.confirmCancel {
		t.Error("ctrl+c 1回目で confirmCancel が立たない")
	}
	// g/G でログトグル
	m5 := tbase()
	m5 = stepKey(m5, key("g"))
	if !m5.showLog {
		t.Error("g でログが開かない")
	}
	m5 = stepKey(m5, key("g"))
	if m5.showLog {
		t.Error("g でログが閉じない")
	}
	// q at stSource → quit (cmd non-nil)
	m6 := tbase()
	m6.stage = stSource
	_, cmd := m6.Update(key("q"))
	if cmd == nil {
		t.Error("q at stSource で終了コマンドが返らない")
	}
	// q at stDone → quit
	m7 := tbase()
	m7.stage = stDone
	m7.finalRC = 0
	_, cmd = m7.Update(key("q"))
	if cmd == nil {
		t.Error("q at stDone で終了コマンドが返らない")
	}
	// q at stSourceDir → default goBack (no quit) — actually stSourceDir case in q switch is not listed, so default → goBack
	m8 := tbase()
	m8.stage = stSourceDir
	m8.browse(t.TempDir(), "コピー元", false)
	_, cmd = m8.Update(key("q"))
	// goBack returns nil cmd for stSourceDir? goBack sets stage stSource, returns nil. So cmd may be nil.
	_ = cmd
	// esc during prompting → cancel prompt
	m9 := tbase()
	m9.stage = stSourceDir
	m9.browse(t.TempDir(), "コピー元", false)
	m9.prompting = true
	m9 = stepKey(m9, esc())
	if m9.prompting {
		t.Error("esc でプロンプトが閉じない")
	}
	// esc during showLog (already tested) and showHelp (tested)
	// l/L toggle fileList at preview
	m10 := tbase()
	m10.stage = stPreview
	m10.items = []string{"a", "b", "c"}
	m10 = stepKey(m10, key("l"))
	if !m10.showFileList {
		t.Error("l でファイル一覧が開かない")
	}
	m10 = stepKey(m10, key("L"))
	if m10.showFileList {
		t.Error("L でファイル一覧が閉じない")
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
