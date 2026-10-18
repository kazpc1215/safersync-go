package tui

// nav.go - メニューの組み立て・画面遷移・チェック（bash 版 step_src/dst 相当）

import (
	"os"

	"github.com/kazpc1215/safersync/internal/core"
	"github.com/kazpc1215/safersync/internal/version"
)

func (m *model) loadSourceMenu() {
	m.items = nil
	for _, v := range sourceVolumes(m.mountRoot) {
		m.items = append(m.items, volumeLabel(v))
	}
	m.sel = 0
}

func (m *model) loadDestMenu() {
	vols := destVolumes(m.mountRoot, m.src)
	if len(vols) == 0 {
		m.message("保存先にできるボリュームがありません", stDest)
		return
	}
	m.items = nil
	for _, v := range vols {
		m.items = append(m.items, volumeLabel(v))
	}
	m.sel = 0
}

// browse はフォルダ選択の状態を初期化する。
func (m *model) browse(dir, label string, canCreate bool) {
	m.browseDir = core.AbsPath(dir)
	m.browseRoot = core.AbsPath(dir)
	m.pickLabel = label
	m.pickCreate = canCreate
	m.sel = 0
}

// message は「[Enter] で戻る」の1枚絵を出す（bash 版 tui_message 相当）。
func (m *model) message(text string, back stage) {
	m.msg = text
	m.returnTo = back
	m.stage = stMessage
}

// stMessage はメッセージ表示用の擬似ステージ。
const stMessage stage = 99

// checkSrc はコピー元決定時の検査。エラーメッセージ（空なら OK）。
func (m *model) checkSrc(p string) string {
	if ok, _ := isDirFollow(p); !ok {
		return "フォルダが見つかりません: " + p
	}
	return ""
}

// checkDst はコピー先決定時の検査。
func (m *model) checkDst(p string) string {
	if p == m.src {
		return "コピー元とコピー先が同じです"
	}
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return "コピー先がフォルダではありません: " + p
	}
	return ""
}

// previewItems はプレビュー画面の選択肢（空き容量・マウント状態で変える）。
func (m *model) previewItems() []string {
	if m.spaceOK && m.mounted {
		return []string{"今すぐ開始する", "設定を戻す", "中止する"}
	}
	return []string{"開始できません（上記の注意を解消してください）", "設定を戻す", "中止する"}
}

func progTitle() string { return "safersync v" + version.Version }
