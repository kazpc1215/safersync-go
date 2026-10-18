package tui

// browse.go - フォルダ選択（階層移動）のデータ（bash 版 picker.sh と同じルール）

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kazpc1215/safersync/internal/core"
)

// 項目の種類。paths 側は実パス＋予約語という bash 版の設計を、型で表現したもの。
type pickKind int

const (
	pickDir pickKind = iota
	pickUp
	pickSelect
	pickNew
	pickManual
)

type pickItem struct {
	kind  pickKind
	path  string
	label string
}

// isSymlink はリンクそのものなら true。
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// isDirFollow はリンク先を評価してディレクトリか（bash の [ -d ] 相当）。
func isDirFollow(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

// browseEntries は dir の中の候補を作る。
//   - 上の階層（dir が起点や "/" でなければ「..」）
//   - 隠しでないサブフォルダ（名前順）
//   - 決定 / 新規作成 / 手動入力 の 3 予約項目
//
// browseEntries は隠しフォルダを非表示にする従来挙動（テスト互換）。
func browseEntries(dir, root string, pickLabel string, canCreate bool) []pickItem {
	return browseEntriesWithOptions(dir, root, pickLabel, canCreate, false)
}

// browseEntriesWithOptions は隠しフォルダの表示を切り替えられる版。
func browseEntriesWithOptions(dir, root string, pickLabel string, canCreate bool, showHidden bool) []pickItem {
	return browseEntriesAdvanced(dir, root, pickLabel, canCreate, showHidden, "")
}

// browseEntriesAdvanced は隠し表示＋フィルタを適用する版。
func browseEntriesAdvanced(dir, root string, pickLabel string, canCreate bool, showHidden bool, filter string) []pickItem {
	var items []pickItem
	if dir != root && dir != "/" {
		items = append(items, pickItem{
			kind: pickUp, path: filepath.Dir(dir),
			label: ".. （上の階層へ）",
		})
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") && !showHidden {
				continue // 隠しフォルダは既定で出さない（bash 版と同じ）
			}
			if !e.IsDir() {
				continue
			}
			label := e.Name() + "/"
			if strings.HasPrefix(e.Name(), ".") {
				label = e.Name() + "/  (隠し)"
			}
			if filter != "" && !strings.Contains(strings.ToLower(e.Name()), strings.ToLower(filter)) {
				continue
			}
			items = append(items, pickItem{kind: pickDir, path: dir + "/" + e.Name(), label: label})
		}
	}
	action := "このフォルダを" + pickLabel + "にする"
	items = append(items, pickItem{kind: pickSelect, label: action})
	return items
}

// createDir は「新しいフォルダを作成する」の実行。
// 相対入力（例: backup/2024）は dir の下、絶対入力・"~"入力はそのまま解釈する。
// 成功時は作ったフォルダの絶対パスを返す。
func createDir(dir, name string) (string, error) {
	var target string
	if strings.HasPrefix(name, "~") {
		target = core.AbsPath(name) // "~..." はホーム基準（CLI と同じ）
	} else if filepath.IsAbs(name) {
		target = core.AbsPath(name)
	} else {
		target = core.StripTrailingSlashes(filepath.Join(dir, name))
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	return target, nil
}
