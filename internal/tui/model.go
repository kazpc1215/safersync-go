package tui

// model.go - TUI の状態機械（bash 版 app.sh の stage 遷移と同じ）

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kazpc1215/safersync/internal/core"
)

type stage int

const (
	stSource     stage = iota // ボリューム選択（コピー元）
	stSourceDir               // フォルダ階層移動（コピー元）
	stDest                    // ボリューム選択（コピー先）
	stDestDir                 // フォルダ階層移動（コピー先）
	stOptions                 // 同期オプション
	stPlanBusy                // 差分スキャン中
	stPreview                 // 最終確認メニュー
	stTransfer                // 転送ダッシュボード
	stVerifyBusy              // 照合中
	stDone                    // 完了レポート
)

// -----------------------------------------------------------------------
// 非同期メッセージ（goroutine → tea.Program.Send）
// -----------------------------------------------------------------------

type spinnerTickMsg struct{}
type interruptRequestMsg struct{} // 外部シグナル（TERM/HUP/INT）による中断要求
type planReadyMsg struct {
	srcFiles int
	srcBytes int64
	plan     core.Plan
}
type frameMsg struct {
	pct                     int
	done, total, speed, eta int64
	file                    string
}
type transferDoneMsg struct{ rc int }
type verifyDoneMsg struct {
	rc     int
	output string
}

// -----------------------------------------------------------------------
// model
// -----------------------------------------------------------------------

type model struct {
	prog *tea.Program // goroutine からの通知用

	mountRoot   string // ボリューム基点
	prefillFrom string // --from（フォルダならスキップ）
	prefillTo   string // --to
	logFile     string

	// TUI オプション（bash 版既定）
	optLRV        bool
	optTHM        bool
	optTMP        bool
	optChecksum   bool
	optCaffeinate bool
	caffeineAvail bool

	src, dst string

	// リスト共通
	stage     stage
	items     []string
	sel       int
	returnTo  stage // メッセージ([Enter] で戻る) の復帰先
	msg       string
	prompting bool
	prompt    string // 入力バッファ
	promptLbl string

	// フォルダ選択の状態
	browseDir, browseRoot, pickLabel string
	pickCreate                       bool
	browseFilter                     string
	showHidden                       bool

	// 除外パターン（CLI --exclude 相当）
	extraExcludes []string

	// プレビュー・転送の結果
	free    int64
	plan    core.Plan
	srcScan core.ScanResult
	spaceOK bool
	mounted bool
	warnMP  bool

	// 実行中表示（転送フレーム）
	frame frameMsg
	// プロンプトの種類
	promptKind promptKind

	xferFiles int
	xferBytes int64
	elapsed   time.Duration
	xferRC    int
	verifyRC  int
	verifyOut string
	finalRC   int
	interrupt bool

	// 実行中
	transfer *core.Transfer
	verifyAt time.Time
	startAt  time.Time

	// UI
	spinner        int
	confirmCancel  bool
	showHelp       bool
	showFileList   bool
	fileListScroll int
	showLog        bool
	logScroll      int
	previewTab     int

	// 描画
	width, height int
}

func newModel(mountRoot, from, to string) *model {
	m := model{
		mountRoot:     mountRoot,
		prefillFrom:   from,
		prefillTo:     to,
		optLRV:        true,
		optTHM:        true,
		optTMP:        false,
		optChecksum:   false,
		optCaffeinate: true,
		caffeineAvail: hasCaffeinate(),
		returnTo:      stSource,
		stage:         stSource,
		logFile:       core.LogFile(),
	}
	// 引数があればプリフィル（--tui と位置引数の併用。bash 版と同じ）
	if m.prefillFrom != "" {
		p := core.AbsPath(m.prefillFrom)
		if ok, _ := isDirFollow(p); ok {
			m.src = p
			m.stage = stDest
			if m.prefillTo != "" {
				m.dst = core.AbsPath(m.prefillTo)
				m.stage = stOptions
			}
		}
	}
	return &m
}

// job は現在のオプションから core.Job を組み立てる。
func (m model) job() *core.Job {
	excl := m.excludes()
	return &core.Job{
		Src: m.src, Dst: m.dst,
		Excludes:     excl,
		FAT:          true, // TUI は既定（exFAT 安全側）に固定。CLI --apfs を使う
		Checksum:     m.optChecksum,
		Verbose:      false,
		ShowProgress: false,
		ListFiles:    true,
		NoCaffeinate: !m.optCaffeinate,
		Log:          core.LogMsg,
	}
}

func (m model) excludes() []string {
	var out []string
	if m.optLRV {
		out = append(out, "*.LRV")
	}
	if m.optTHM {
		out = append(out, "*.THM")
	}
	if m.optTMP {
		out = append(out, "*.tmp", "*.temp")
	}
	out = append(out, m.extraExcludes...)
	return out
}

func (m model) excludesSummary() string {
	if s := strings.Join(m.excludes(), " "); s != "" {
		return s + " "
	}
	return "（なし）"
}

// optionsSummaryLine はジョブ本文に乗せるオプション行（デバッグ的な簡潔さ）
func (m model) titleOf(s stage) string {
	names := [...]string{
		stSource: "[1/4] コピー元の選択", stSourceDir: "[1/4] コピー元の選択",
		stDest: "[2/4] コピー先の選択", stDestDir: "[2/4] コピー先の選択",
		stOptions:  "[3/4] 同期オプション",
		stPlanBusy: "[4/4] 転送内容の最終確認", stPreview: "[4/4] 転送内容の最終確認",
		stTransfer: " 同期中", stVerifyBusy: " 照合中", stDone: " 完了",
	}
	if int(s) < len(names) {
		return names[s]
	}
	return "safersync"
}

// ----------------------------------------------------------------------

func (m model) Init() tea.Cmd { return nil }

// Update はキー入力とメッセージを処理する（戻り値は同じ *model を返す）。
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg.String())

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinnerTickMsg:
		m.spinner++
		return m, nil

	case interruptRequestMsg:
		return m, m.interruptAndQuit()

	case planReadyMsg:
		if msg.plan.Err != nil {
			// 差分を確認できないまま進めないようにする（「すべて最新」の誤表示防止）
			m.message("差分を確認できませんでした: "+msg.plan.Err.Error(), stOptions)
			return m, nil
		}
		m.srcScan = core.ScanResult{Files: msg.srcFiles, Bytes: msg.srcBytes}
		m.plan = msg.plan
		free := core.FreeBytes(m.dst)
		m.free = free
		m.spaceOK = free >= m.plan.Bytes
		m.mounted = core.DestIsMounted(m.dst, m.mountRoot)
		m.warnMP = core.IsMountPoint(m.dst)
		m.stage = stPreview
		m.items = m.previewItems()
		m.sel = 0
		if !m.spaceOK || !m.mounted {
			m.sel = 1
		}
		return m, nil

	case frameMsg:
		m.frame = msg
		return m, nil

	case transferDoneMsg:
		m.xferRC = msg.rc
		if msg.rc != 0 {
			m.verifyRC = 1 // bash 版と同じ: 転送失敗なら照合は行わない
			m.elapsed = time.Since(m.startAt)
			m.finalRC = msg.rc
			m.stage = stDone
			return m, nil
		}
		m.stage = stVerifyBusy
		return m, startVerifyCmd(m)

	case verifyDoneMsg:
		m.verifyRC = msg.rc
		m.verifyOut = msg.output
		m.elapsed = time.Since(m.startAt)
		// 終了コードは CLI とそろえる（コピー失敗 > 照合失敗 > 成功）
		if m.xferRC != 0 {
			m.finalRC = m.xferRC
		} else if m.verifyRC != 0 {
			m.finalRC = 1
		}
		m.stage = stDone
		return m, nil
	}
	return m, nil
}

func (m *model) updateKey(key string) (tea.Model, tea.Cmd) {
	if key != "ctrl+c" {
		m.confirmCancel = false
	}
	if m.showLog {
		data, _ := os.ReadFile(m.logFile)
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		switch key {
		case "up", "k":
			if m.logScroll > 0 {
				m.logScroll--
			}
		case "down", "j":
			if m.logScroll < len(lines)-1 {
				m.logScroll++
			}
		default:
			m.showLog = false
		}
		return m, nil
	}
	switch key {
	case "ctrl+c":
		if !m.confirmCancel {
			m.confirmCancel = true
			return m, nil
		}
		// いつでも安全中断: 子プロセスも止め、130 で終了（bash 版 tui_on_interrupt と同じ）
		return m, m.interruptAndQuit()
	case "esc":
		if m.showLog {
			m.showLog = false
			return m, nil
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.prompting {
			m.prompting = false
			return m, nil
		}
		if m.stage != stTransfer && m.stage != stVerifyBusy && m.stage != stPlanBusy {
			return m, m.goBack()
		}
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "l", "L":
		if m.stage == stPreview {
			m.showFileList = !m.showFileList
			m.fileListScroll = 0
			return m, nil
		}
	case "g", "G":
		m.showLog = !m.showLog
		m.logScroll = 0
		return m, nil
	case "q", "Q":
		// bash 版と同じ: ソース系・プレビューでは中止、他は Esc と同じ「戻る」
		if m.prompting || m.stage == stTransfer || m.stage == stVerifyBusy || m.stage == stPlanBusy {
			return m, nil
		}
		switch m.stage {
		case stSource, stSourceDir, stPreview:
			m.finalRC = 0
			return m, tea.Quit
		case stDone:
			// 完了画面では終了コードを保ったまま終了する（bash 版と同じ）
			return m, tea.Quit
		default:
			return m, m.goBack()
		}
	}
	if m.stage == stMessage {
		// 「[Enter] で戻る」の1枚絵（bash 版 tui_message 相当）
		if key == "enter" || key == " " {
			m.stage = m.returnTo
			m.msg = ""
			return m, nil
		}
		return m, nil
	}
	if m.prompting {
		return m.updatePrompt(key)
	}
	if m.stage == stDone {
		if key == "enter" || key == " " {
			return m, tea.Quit
		}
		// 転送失敗時の再試行/再開フロー
		if m.xferRC != 0 {
			switch key {
			case "r", "R":
				m.xferRC = 0
				m.verifyRC = 0
				m.finalRC = 0
				if err := m.beginTransfer(); err != nil {
					m.message("再試行できませんでした: "+err.Error(), stDone)
					return m, nil
				}
				m.stage = stTransfer
				return m, nil
			case "v", "V":
				m.stage = stVerifyBusy
				return m, startVerifyCmd(m)
			}
		}
		return m, nil
	}

	switch m.stage {
	case stSource, stDest:
		return m.updateVolume(key)
	case stSourceDir, stDestDir:
		return m.updateBrowse(key)
	case stOptions:
		return m.updateOptions(key)
	case stPreview:
		return m.updatePreview(key)
	case stTransfer, stVerifyBusy, stPlanBusy:
		// 処理中は無入力
		return m, nil
	}
	return m, nil
}

// interruptAndQuit は転送中の rsync も止めて、130（中断）でウィザードを終了する。
// Ctrl-C の2回目と、外部シグナル（SIGTERM/SIGHUP）の両方から使う。
func (m *model) interruptAndQuit() tea.Cmd {
	if m.transfer != nil && m.transfer.StartedOK() {
		m.transfer.Interrupt()
	}
	m.finalRC = core.ExitInterrupted
	m.interrupt = true
	return tea.Quit
}

// goBack は Esc で1つ前のステージへ。step_src→終了、src→vol、…、preview→options。
func (m *model) goBack() tea.Cmd {
	switch m.stage {
	case stSource:
		m.finalRC = 0
		return tea.Quit // bash 版: 中止
	case stDest:
		m.stage = stSource
		m.loadSourceMenu()
	case stSourceDir:
		m.stage = stSource
		m.loadSourceMenu()
	case stDestDir:
		m.stage = stDest
		m.loadDestMenu()
	case stOptions:
		m.stage = stDest
		m.loadDestMenu()
	case stPreview:
		m.stage = stOptions
	case stDone:
		return tea.Quit // Enter 以外でも Esc で終了できるようにする
	}
	return nil
}

// ----------------------------------------------------------------------
// 各ステージでのキー処理
// ----------------------------------------------------------------------

func (m *model) updateVolume(key string) (tea.Model, tea.Cmd) {
	n := len(m.items)
	switch key {
	case "up", "k":
		m.sel = mod(m.sel-1, n)
	case "down", "j":
		m.sel = mod(m.sel+1, n)
	case "enter":
		vols := m.volumeCandidates()
		if m.sel >= 0 && m.sel < len(vols) {
			v := vols[m.sel].path
			if m.stage == stSource {
				m.browse(v, "コピー元", false)
				m.stage = stSourceDir
			} else {
				m.browse(v, "コピー先", true)
				m.stage = stDestDir
			}
		}
	}
	return m, nil
}

// volumeCandidates は表示中のボリューム一覧をもう一度組立てる（更新対応）。
func (m model) volumeCandidates() []volumeItem {
	if m.stage == stSource {
		return sourceVolumes(m.mountRoot)
	}
	return destVolumes(m.mountRoot, m.src)
}

func (m *model) updateBrowse(key string) (tea.Model, tea.Cmd) {
	n := len(browseEntriesAdvanced(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate, m.showHidden, m.browseFilter))
	switch key {
	case "up", "k":
		m.sel = mod(m.sel-1, n)
	case "down", "j":
		m.sel = mod(m.sel+1, n)
	case "h", "H":
		m.showHidden = !m.showHidden
		m.sel = 0
		return m, nil
	case "n", "N":
		m.prompting = true
		m.prompt = ""
		m.promptLbl = "新しいフォルダ名を入力してください（例: backup/2024）"
		m.promptKind = pkNew
	case "p", "P":
		m.prompting = true
		m.prompt = ""
		m.promptLbl = "パスを入力してください（例: /Volumes/HDD/GoPro）"
		m.promptKind = pkManual
	case "/":
		m.prompting = true
		m.prompt = ""
		m.promptLbl = "フィルタ文字列を入力（空で解除）"
		m.promptKind = pkFilter
	case "enter":
		entries := browseEntriesAdvanced(m.browseDir, m.browseRoot, m.pickLabel, m.pickCreate, m.showHidden, m.browseFilter)
		if m.sel < 0 || m.sel >= len(entries) {
			return m, nil
		}
		it := entries[m.sel]
		switch it.kind {
		case pickUp, pickDir:
			m.browseDir = it.path
			m.browseFilter = ""
			m.sel = 0
		case pickSelect:
			if m.stage == stSourceDir {
				if e := m.checkSrc(m.browseDir); e != "" {
					m.message(e, m.stage)
					return m, nil
				}
				m.src = m.browseDir
				m.stage = stDest
				m.loadDestMenu()
				return m, nil
			}
			// コピー先
			if e := m.checkDst(m.browseDir); e != "" {
				m.message(e, m.stage)
				return m, nil
			}
			m.dst = m.browseDir
			m.stage = stOptions
			m.sel = 0 // 選択カーソルは画面ごとに先頭へ（bash 版 TUI_MENU_SEL=0 と同じ）
		case pickNew:
			m.prompting = true
			m.prompt = ""
			m.promptLbl = "新しいフォルダ名を入力してください（例: backup/2024）"
			m.promptKind = pkNew
		case pickManual:
			m.prompting = true
			m.prompt = ""
			m.promptLbl = "パスを入力してください（例: /Volumes/HDD/GoPro）"
			m.promptKind = pkManual
		}
	}
	return m, nil
}

type promptKind int

const (
	pkNew promptKind = iota
	pkManual
	pkFilter
	pkExclude
)

func (m *model) updatePrompt(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		input := strings.TrimSpace(m.prompt)
		m.prompting = false
		if input == "" {
			return m, nil
		}
		if m.promptKind == pkNew {
			name := core.StripTrailingSlashes(input)
			if name == "" {
				return m, nil
			}
			target, err := createDir(m.browseDir, name)
			if err != nil {
				m.message("フォルダを作成できません: "+target, m.stage)
				return m, nil
			}
			m.browseDir = target
			m.sel = 0
			return m, nil
		}
		if m.promptKind == pkFilter {
			m.browseFilter = input
			m.sel = 0
			return m, nil
		}
		if m.promptKind == pkExclude {
			if input != "" {
				m.extraExcludes = append(m.extraExcludes, input)
			}
			m.sel = 0
			return m, nil
		}
		// 手動入力
		p := core.AbsPath(input)
		if ok, _ := isDirFollow(p); ok {
			m.browseDir = p
			m.sel = 0
			return m, nil
		}
		if m.pickCreate {
			if err := os.MkdirAll(p, 0o755); err == nil {
				m.browseDir = p
				m.sel = 0
				return m, nil
			}
			m.message("フォルダを作成できません: "+p, m.stage)
			return m, nil
		}
		m.message("フォルダが見つかりません: "+p, m.stage)
		return m, nil
	case "backspace":
		if r := []rune(m.prompt); len(r) > 0 {
			m.prompt = string(r[:len(r)-1])
		}
		return m, nil
	default:
		if len(key) == 1 {
			m.prompt += key
		}
		return m, nil
	}
}

func (m *model) updateOptions(key string) (tea.Model, tea.Cmd) {
	n := 6
	switch key {
	case "up", "k":
		m.sel = mod(m.sel-1, n)
	case "down", "j":
		m.sel = mod(m.sel+1, n)
	case " ":
		switch m.sel {
		case 0:
			m.optLRV = !m.optLRV
		case 1:
			m.optTHM = !m.optTHM
		case 2:
			m.optTMP = !m.optTMP
		case 3:
			m.optChecksum = !m.optChecksum
		case 4:
			if m.caffeineAvail {
				m.optCaffeinate = !m.optCaffeinate
			}
		case 5:
			m.prompting = true
			m.prompt = ""
			m.promptLbl = "追加で除外するパターンを入力（例: *.tmp）。空でキャンセル"
			m.promptKind = pkExclude
		}
	case "enter":
		if m.sel == 5 {
			m.prompting = true
			m.prompt = ""
			m.promptLbl = "追加で除外するパターンを入力（例: *.tmp）。空でキャンセル"
			m.promptKind = pkExclude
			return m, nil
		}
		m.stage = stPlanBusy
		return m, startPlanCmd(m)
	}
	return m, nil
}

func (m *model) updatePreview(key string) (tea.Model, tea.Cmd) {
	if key == "tab" {
		m.previewTab = (m.previewTab + 1) % 3
		return m, nil
	}
	if m.showFileList {
		switch key {
		case "up", "k":
			if m.fileListScroll > 0 {
				m.fileListScroll--
			}
		case "down", "j":
			if m.fileListScroll < len(m.plan.Lines)-1 {
				m.fileListScroll++
			}
		case "l", "L", "esc":
			m.showFileList = false
		}
		return m, nil
	}
	n := len(m.items)
	switch key {
	case "up", "k", "left":
		m.sel = mod(m.sel-1, n)
	case "down", "j", "right":
		m.sel = mod(m.sel+1, n)
	case "enter":
		switch m.sel {
		case 0:
			if m.spaceOK && m.mounted {
				if err := m.beginTransfer(); err != nil {
					m.message("実行できませんでした: "+err.Error(), stOptions)
					return m, nil
				}
				m.stage = stTransfer
				return m, nil
			}
		case 1:
			m.stage = stOptions
		default:
			m.finalRC = 0
			return m, tea.Quit
		}
	}
	return m, nil
}

func mod(a, n int) int {
	if n <= 0 {
		return 0
	}
	return ((a % n) + n) % n
}
