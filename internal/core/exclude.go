package core

// exclude.go - rsync の --exclude 相当のパターン照合
//
// 走査（ScanDir による進捗の分母・照合の集計）と実際の rsync 転送で
// 除外の解釈がずれると、「rsync は転送するのに走査は数えない」などの
// 不一致が生じる。ここでは rsync の exclude ルールのうち実用上重要な
// ものを再現する:
//
//   - パターンに "/"（末尾を除く）も "**" も含まない場合は、最終要素
//     （ベース名）に対して一致する。
//   - "/" または "**" を含む場合は、転送ルートからの相対パスに対して
//     一致する。先頭 "/" でルートに固定され、そうでなければパス末尾の
//     任意のディレクトリ境界から一致する。
//   - 末尾 "/" はディレクトリだけに一致する。
//   - "*" は "/" をまたがない、"**" は "/" をまたぐ、"?" は "/" 以外の
//     1 文字、"[...]" は文字クラス。
//
// rsync の全機能（merge-file ルール、"+ " などのプレフィクス、per-dir
// ルール等）は対象外。safersync が渡す --exclude の範囲を再現する。

import (
	"path"
	"strings"
)

// MatchExclude は relPath（走査ルートからの相対、"/" 区切り）が pattern に
// より除外されるかを返す。isDir は relPath がディレクトリかどうか。
func MatchExclude(pattern, relPath string, isDir bool) bool {
	if pattern == "" {
		return false
	}
	pat := pattern

	// 末尾 "/" はディレクトリ限定。
	dirOnly := strings.HasSuffix(pat, "/")
	for strings.HasSuffix(pat, "/") && len(pat) > 1 {
		pat = strings.TrimSuffix(pat, "/")
	}
	if pat == "" || pat == "/" {
		return false
	}
	if dirOnly && !isDir {
		return false
	}

	// 先頭 "/" はルート固定。
	anchored := strings.HasPrefix(pat, "/")
	pat = strings.TrimLeft(pat, "/")
	if pat == "" {
		return false
	}

	// "/" も "**" も含まない → 最終要素（ベース名）に対して一致。
	if !strings.Contains(pat, "/") && !strings.Contains(pat, "**") {
		if anchored {
			return globMatch(pat, relPath)
		}
		return globMatch(pat, path.Base(relPath))
	}

	// それ以外は転送ルート相対のパスに対して一致。固定されていなければ
	// 任意のディレクトリ境界から一致させる（"foo/bar" は "a/foo/bar" に一致）。
	if anchored {
		return globMatch(pat, relPath)
	}
	if globMatch(pat, relPath) {
		return true
	}
	for i := 0; i < len(relPath); i++ {
		if relPath[i] == '/' && globMatch(pat, relPath[i+1:]) {
			return true
		}
	}
	return false
}

// globMatch はシェル風のワイルドカードで s 全体が pattern に一致するかを返す。
// "*" は "/" をまたがず、"**" は "/" をまたぐ。
func globMatch(pattern, s string) bool {
	return globRunes([]rune(pattern), []rune(s))
}

func globRunes(pat, s []rune) bool {
	for len(pat) > 0 {
		switch pat[0] {
		case '*':
			stars := 0
			for len(pat) > 0 && pat[0] == '*' {
				pat = pat[1:]
				stars++
			}
			// 連続する "*" が 2 個以上なら "/" をまたぐ。
			crossSlash := stars >= 2
			// "**/" は 0 個以上のディレクトリに一致する（rsync と同じ）。
			// "**/*.tmp" がルート直下の "x.tmp" にも一致するよう、
			// スラッシュを消費しない選択肢も試す。
			if crossSlash && len(pat) > 0 && pat[0] == '/' && globRunes(pat[1:], s) {
				return true
			}
			if len(pat) == 0 {
				if crossSlash {
					return true
				}
				return !strings.ContainsRune(string(s), '/')
			}
			// 次の断片を、到達可能な各位置で試す。
			for i := 0; i <= len(s); i++ {
				if globRunes(pat, s[i:]) {
					return true
				}
				if i < len(s) && s[i] == '/' && !crossSlash {
					break
				}
			}
			return false
		case '?':
			if len(s) == 0 || s[0] == '/' {
				return false
			}
			pat, s = pat[1:], s[1:]
		case '[':
			next, ok := matchClass(pat, s)
			if !ok {
				return false
			}
			pat, s = next, s[1:]
		default:
			if len(s) == 0 || s[0] != pat[0] {
				return false
			}
			pat, s = pat[1:], s[1:]
		}
	}
	return len(s) == 0
}

// matchClass は pat の先頭 "[" から始まる文字クラスを s の先頭 1 文字と照合し、
// クラスを消費した残りのパターンを返す。クラスが不正（閉じ "] " が無い）なら
// ok=false（呼び出し側では一致なし扱い）。
func matchClass(pat, s []rune) (rest []rune, matched bool) {
	if len(pat) == 0 || pat[0] != '[' || len(s) == 0 {
		return pat, false
	}
	i := 1
	negate := false
	if i < len(pat) && (pat[i] == '!' || pat[i] == '^') {
		negate = true
		i++
	}
	hit := false
	first := true
	for i < len(pat) && (pat[i] != ']' || first) {
		first = false
		// 範囲指定 "a-z"
		if i+2 < len(pat) && pat[i+1] == '-' && pat[i+2] != ']' {
			if pat[i] <= s[0] && s[0] <= pat[i+2] {
				hit = true
			}
			i += 3
			continue
		}
		if pat[i] == s[0] {
			hit = true
		}
		i++
	}
	if i >= len(pat) || pat[i] != ']' {
		return pat, false // 閉じていない
	}
	return pat[i+1:], hit != negate
}
