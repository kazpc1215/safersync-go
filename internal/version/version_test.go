package version

import (
	"os"
	"strings"
	"testing"
)

// バージョンは VERSION ファイルが単一ソースになっている。
func TestVersionFromFile(t *testing.T) {
	if Version == "" {
		t.Fatal("Version が空")
	}
	if strings.TrimSpace(Version) != Version {
		t.Fatalf("Version に前後の空白が残っている: %q", Version)
	}
	data, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != Version {
		t.Errorf("VERSION ファイルと Version が一致しない: %q / %q", data, Version)
	}
}
