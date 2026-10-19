package tui

// volumes.go - ボリューム一覧・ラベル（bash 版 step_src.sh / step_dst.sh と同じ）

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kazpc1215/safersync-go/internal/core"
)

// volumeItem はボリューム候補。
type volumeItem struct {
	path     string
	note     string
	internal bool // "/"（内蔵ディスク）か
}

// volumeLabel は表示用ラベル。「Macintosh HD 内蔵ディスク   空き 122Gi / 使用 12Gi [████░░]」。
func volumeLabel(v volumeItem) string {
	name := filepath.Base(v.path)
	if v.path == "/" {
		name = "Macintosh HD"
	}
	_, used, avail := core.DfHuman(v.path)
	info := ""
	if avail != "" {
		info = fmt.Sprintf("空き %s / 使用 %s", avail, used)
	}
	// 簡易容量バー（幅 8）。取得できない場合は省略。
	bar := ""
	if total := core.TotalBytes(v.path); total > 0 {
		free := core.FreeBytes(v.path)
		usedPct := int((total - free) * 8 / total)
		if usedPct < 0 {
			usedPct = 0
		}
		if usedPct > 8 {
			usedPct = 8
		}
		bar = " [" + strings.Repeat("█", usedPct) + strings.Repeat("░", 8-usedPct) + "]"
	}
	return fmt.Sprintf("%s %s   %s%s", name, v.note, info, bar)
}

// 走査対象の実ディレクトリ（シンボリックリンクを除外。bash 版と同じ: [ -L ] && continue）
func realDir(path string) bool {
	fi, err := isDirFollow(path) // リンク先を評価
	if err != nil {
		return false
	}
	return fi
}

// sourceVolumes は「/」＋ ボリューム基点直下の実フォルダ。
func sourceVolumes(mountRoot string) []volumeItem {
	vols := []volumeItem{{path: "/", note: "内蔵ディスク", internal: true}}
	for _, v := range core.Volumes(mountRoot) {
		if isSymlink(v) {
			continue // /Volumes/Macintosh HD などのリンクは除外
		}
		if !realDir(v) {
			continue
		}
		vols = append(vols, volumeItem{path: v, note: "(外付け)", internal: false})
	}
	return vols
}

// destVolumes はコピー先の候補（コピー元と同じボリュームも含める。同一なら印を付ける）。
func destVolumes(mountRoot, src string) []volumeItem {
	srcMP := core.MountPointOf(src)
	var out []volumeItem
	for _, v := range sourceVolumes(mountRoot) {
		if v.internal {
			// "/" の note は内蔵ディスクを保ったまま、同一判定を上書きする
			if core.MountPointOf("/") == srcMP {
				v.note = "内蔵ディスク (コピー元と同じボリューム)"
			}
			out = append(out, v)
			continue
		}
		if core.MountPointOf(v.path) == srcMP {
			v.note = "(コピー元と同じボリューム)"
		}
		out = append(out, v)
	}
	return out
}
