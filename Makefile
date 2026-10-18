# safersync (Go 版) ビルド用 Makefile
APP     := safersync
VERSION := 1.4.0
DIST    := dist

# ターゲット: このMac (darwin/amd64), Apple Silicon, Linux x86_64/ARM64
TARGETS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

# 静的リンク（libc 不依存・任意の環境で動く単一バイナリ）。
# バージョンはリリースビルド時に上書きできる。
LDFLAGS := -s -w -X github.com/kazpc1215/safersync/internal/version.Version=$(VERSION)

.PHONY: build test cover vet fmt clean install release formula $(TARGETS)

build: $(TARGETS) ## 全ターゲットを dist/ へビルド

test: ## ユニットテスト
	go test ./...

cover: ## カバレッジ計測（合計のみ表示。coverage.out に詳細）
	go test -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(DIST)

install: build ## 現在のプラットフォームのバイナリを ~/.local/bin/safersync へ
	@command -v safersync >/dev/null 2>&1 && test "$(command -v safersync)" != "$(CURDIR)/$(DIST)/$$(go env GOOS)-$$(go env GOARCH)/$(APP)" || true
	ln -sfn "$(CURDIR)/$(DIST)/$$(go env GOOS)-$$(go env GOARCH)/$(APP)" ~/.local/bin/$(APP)
	@echo "  linked: ~/.local/bin/$(APP) -> $(CURDIR)/$(DIST)/$$(go env GOOS)-$$(go env GOARCH)/$(APP)"

$(TARGETS):
	@os=$(word 1,$(subst /, ,$@)); arch=$(word 2,$(subst /, ,$@)); \
	echo "== $$os/$$arch =="; \
	CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
	  go build -trimpath -ldflags "$(LDFLAGS)" \
	  -o $(DIST)/$$os-$$arch/$(APP) .

# リリース物の作成: 各プラットフォームの tarball + SHA256SUMS
#   dist/release/safersync-$(VERSION)-<os>-<arch>.tar.gz
release: clean $(TARGETS) ## リリース用 tarball + チェックサムを dist/release/ へ
	@rm -rf $(DIST)/release; mkdir -p $(DIST)/release
	@for d in $(DIST)/*/; do \
	  base=$$(basename "$$d"); \
	  case "$$base" in release) continue;; esac; \
	  os=$${base%%-*}; arch=$${base##*-}; \
	  tar -C "$$d" -czf "$(DIST)/release/$(APP)-$(VERSION)-$$os-$$arch.tar.gz" $(APP); \
	  echo "  tar: $(APP)-$(VERSION)-$$os-$$arch.tar.gz"; \
	done
	@cd $(DIST)/release && shasum -a 256 *.tar.gz > SHA256SUMS && echo "  done: dist/release/SHA256SUMS"

formula: ## Homebrew formula を dist/release/SHA256SUMS から生成（make release の後）
	@test -f $(DIST)/release/SHA256SUMS || { \
	  echo "✗ $(DIST)/release/SHA256SUMS がありません。先に make release を実行してください" >&2; exit 1; }
	@sh packaging/homebrew/update-formula.sh $(DIST)/release/SHA256SUMS $(VERSION)

help:
	@awk -F':.*##' '/^[a-zA-Z_.-]+:/ { printf "  make %-12s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
