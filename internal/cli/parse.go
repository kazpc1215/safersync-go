package cli

import (
	"fmt"
	"strings"

	"github.com/kazpc1215/safersync/internal/core"
)

// Parse は引数を解析して Options を返す。
// bash 版と同じ規則:
//   - コピー元・コピー先は位置引数 2 つ、または --from / --to（順序自由）
//   - 位置引数が 3 つ以上あると「引数が多すぎます」エラー
func Parse(argv []string) (*core.Options, error) {
	opts := core.NewOptions()
	var positional []string

	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--version" || a == "-V":
			opts.ShowVersion = true
		case a == "--help" || a == "-h":
			opts.ShowHelp = true
		case a == "--tui" || a == "-i":
			opts.TUI = true
		case a == "--status":
			opts.ShowStatus = true
		case a == "--from" || a == "-f":
			if a, err := value(argv, &i, "--from"); err != nil {
				return nil, err
			} else {
				opts.From = a
			}
		case strings.HasPrefix(a, "--from="):
			opts.From = strings.TrimPrefix(a, "--from=")
		case a == "--to" || a == "-t":
			if a, err := value(argv, &i, "--to"); err != nil {
				return nil, err
			} else {
				opts.To = a
			}
		case strings.HasPrefix(a, "--to="):
			opts.To = strings.TrimPrefix(a, "--to=")
		case a == "--exclude" || a == "-x":
			if a, err := value(argv, &i, "--exclude"); err != nil {
				return nil, err
			} else {
				opts.Excludes = append(opts.Excludes, a)
			}
		case strings.HasPrefix(a, "--exclude="):
			opts.Excludes = append(opts.Excludes, strings.TrimPrefix(a, "--exclude="))
		case a == "--mount-root":
			if a, err := value(argv, &i, "--mount-root"); err != nil {
				return nil, err
			} else {
				opts.MountRoot = a
			}
		case strings.HasPrefix(a, "--mount-root="):
			opts.MountRoot = strings.TrimPrefix(a, "--mount-root=")
		case a == "--fat":
			opts.FAT = true
		case a == "--apfs":
			opts.APFS = true
		case a == "--dry-run" || a == "-n":
			opts.DryRun = true
		case a == "--verify-only":
			opts.VerifyOnly = true
		case a == "--no-verify":
			opts.NoVerify = true
		case a == "--checksum" || a == "-c":
			opts.Checksum = true
		case a == "--ignore-space-check":
			opts.IgnoreSpaceCheck = true
		case a == "--stats" || a == "-v":
			opts.Stats = true
		case a == "--progress":
			opts.Progress = true
		case a == "--no-progress":
			opts.NoProgress = true
		case a == "--no-caffeinate":
			opts.NoCaffeinate = true
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return nil, fmt.Errorf("不明なオプションです: %s", a)
		default:
			positional = append(positional, a)
		}
	}

	// 位置引数はコピー元・コピー先の 2 つまで（bash 版と同じ文言でエラー）
	switch len(positional) {
	case 0:
	case 1:
		if opts.From == "" {
			opts.From = positional[0]
		} else if opts.To == "" {
			opts.To = positional[0]
		}
	case 2:
		opts.From = positional[0]
		opts.To = positional[1]
	default:
		return nil, fmt.Errorf("引数が多すぎます: '%s'（コピー元とコピー先を1つずつ指定してください）", positional[2])
	}

	return opts, nil
}

// value はフラグの直後の値を取り出す（--exclude PAT 形式）。
func value(argv []string, i *int, flag string) (string, error) {
	*i++
	if *i >= len(argv) {
		return "", fmt.Errorf("%s には値が必要です", flag)
	}
	return argv[*i], nil
}
