package cli

import (
	"fmt"
	"strings"

	"github.com/kazpc1215/safersync-go/internal/core"
)

// Parse は引数を解析して Options を返す。bash 版 parse_args と同じ規則:
//   - コピー元・コピー先は位置引数 2 つ、または --from / --to（順序自由・重複不可）
//   - 位置引数が 3 つ以上あると「引数が多すぎます」エラー
//   - --mount-root は省略値 / 環境変数より優先
func Parse(argv []string) (*core.Options, error) {
	opts := core.NewOptions()
	positional := &posArgSet{opts: opts}

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
		case a == "--dry-run" || a == "-n":
			opts.DryRun = true
		case a == "--verify-only":
			opts.VerifyOnly = true
		case a == "--no-verify":
			opts.NoVerify = true
		case a == "--checksum" || a == "-c":
			opts.Checksum = true
		case a == "--fat":
			opts.FAT = true
			opts.APFS = false
		case a == "--apfs":
			opts.FAT = false
			opts.APFS = true
		case a == "--ignore-space-check":
			opts.IgnoreSpaceCheck = true
		case a == "--verbose" || a == "--stats" || a == "-v":
			opts.Stats = true
		// --progress / --no-progress は最後に指定された方が勝つ（bash 版と同じ）
		case a == "--progress":
			opts.Progress = true
			opts.NoProgress = false
		case a == "--no-progress":
			opts.NoProgress = true
			opts.Progress = false
		case a == "--no-caffeinate":
			opts.NoCaffeinate = true
		case a == "--from" || a == "-f":
			v, _ := value(argv, &i)
			if v == "" {
				return nil, fmt.Errorf("--from にはパスを指定してください")
			}
			if opts.From != "" {
				return nil, fmt.Errorf("コピー元は1つだけ指定できます（--from か位置引数のどちらか）")
			}
			opts.From = v
		case strings.HasPrefix(a, "--from="):
			v := strings.TrimPrefix(a, "--from=")
			if v == "" {
				return nil, fmt.Errorf("--from にはパスを指定してください")
			}
			if opts.From != "" {
				return nil, fmt.Errorf("コピー元は1つだけ指定できます（--from か位置引数のどちらか）")
			}
			opts.From = v
		case strings.HasPrefix(a, "-f="):
			v := strings.TrimPrefix(a, "-f=")
			if v == "" {
				return nil, fmt.Errorf("--from にはパスを指定してください")
			}
			if opts.From != "" {
				return nil, fmt.Errorf("コピー元は1つだけ指定できます（--from か位置引数のどちらか）")
			}
			opts.From = v
		case a == "--to" || a == "-t":
			v, _ := value(argv, &i)
			if v == "" {
				return nil, fmt.Errorf("--to にはパスを指定してください")
			}
			if opts.To != "" {
				return nil, fmt.Errorf("コピー先は1つだけ指定できます（--to か位置引数のどちらか）")
			}
			opts.To = v
		case strings.HasPrefix(a, "--to="):
			v := strings.TrimPrefix(a, "--to=")
			if v == "" {
				return nil, fmt.Errorf("--to にはパスを指定してください")
			}
			if opts.To != "" {
				return nil, fmt.Errorf("コピー先は1つだけ指定できます（--to か位置引数のどちらか）")
			}
			opts.To = v
		case strings.HasPrefix(a, "-t="):
			v := strings.TrimPrefix(a, "-t=")
			if v == "" {
				return nil, fmt.Errorf("--to にはパスを指定してください")
			}
			if opts.To != "" {
				return nil, fmt.Errorf("コピー先は1つだけ指定できます（--to か位置引数のどちらか）")
			}
			opts.To = v
		case a == "--exclude" || a == "-x":
			v, _ := value(argv, &i)
			if v == "" {
				return nil, fmt.Errorf("--exclude にはパターンを指定してください")
			}
			opts.Excludes = append(opts.Excludes, v)
		case strings.HasPrefix(a, "--exclude="):
			v := strings.TrimPrefix(a, "--exclude=")
			if v == "" {
				return nil, fmt.Errorf("--exclude にはパターンを指定してください")
			}
			opts.Excludes = append(opts.Excludes, v)
		case strings.HasPrefix(a, "-x="):
			v := strings.TrimPrefix(a, "-x=")
			if v == "" {
				return nil, fmt.Errorf("--exclude にはパターンを指定してください")
			}
			opts.Excludes = append(opts.Excludes, v)
		case a == "--mount-root":
			v, _ := value(argv, &i)
			if v == "" {
				return nil, fmt.Errorf("--mount-root にはパスを指定してください")
			}
			opts.MountRoot = v
		case a == "--job":
			v1, ok1 := value(argv, &i)
			v2, ok2 := value(argv, &i)
			if !ok1 || v1 == "" || !ok2 || v2 == "" {
				return nil, fmt.Errorf("--job にはコピー元とコピー先のパスを指定してください（例: --job ~/写真 /Volumes/HDD/写真）")
			}
			opts.Jobs = append(opts.Jobs, core.JobSpec{From: v1, To: v2})
		case strings.HasPrefix(a, "--mount-root="):
			v := strings.TrimPrefix(a, "--mount-root=")
			if v == "" {
				return nil, fmt.Errorf("--mount-root にはパスを指定してください")
			}
			opts.MountRoot = v
		case a == "--":
			// 以降はすべて位置引数として扱う（bash 版と同じ）
			i++
			for ; i < len(argv); i++ {
				if err := positional.add(argv[i]); err != nil {
					return nil, err
				}
			}
			return finish(opts, positional)
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return nil, fmt.Errorf("不明なオプション: %s（--help を参照）", a)
		default:
			if err := positional.add(a); err != nil {
				return nil, err
			}
		}
	}
	return finish(opts, positional)
}

// posArgSet は位置引数を「コピー元 → コピー先」の順に空いている方へ入れる。
//
//	safersync A B / safersync A --to B / safersync --from A B はすべて同じ意味。
type posArgSet struct {
	opts *core.Options
}

func (p *posArgSet) add(a string) error {
	if a == "" {
		return fmt.Errorf("パスが空です")
	}
	switch {
	case p.opts.From == "":
		p.opts.From = a
	case p.opts.To == "":
		p.opts.To = a
	default:
		return fmt.Errorf("引数が多すぎます: '%s'（コピー元とコピー先を1つずつ指定してください）", a)
	}
	return nil
}

// value はフラグの直後の値を取り出す。なければ "" を返す。
func value(argv []string, i *int) (string, bool) {
	*i++
	if *i >= len(argv) {
		return "", false
	}
	return argv[*i], true
}

func finish(opts *core.Options, positional *posArgSet) (*core.Options, error) {
	// --job とコピー元・コピー先（位置引数 / --from / --to）の併用は、
	// どちらかが黙って無視されてしまうためエラーにする。
	if len(opts.Jobs) > 0 && (opts.From != "" || opts.To != "") {
		return nil, fmt.Errorf("コピー元・コピー先は --job と同時に指定できません（--job を使うか、位置引数で1組だけ指定してください）")
	}
	return opts, nil
}
