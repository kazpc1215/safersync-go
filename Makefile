# safersync (Go 版) ビルド用 Makefile
APP     := safersync
VERSION := 1.2.0
DIST    := dist

# ターゲット: このMac (darwin/amd64), Apple Silicon, Linux x86_64/ARM64
TARGETS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: build test vet fmt clean install $(TARGETS)

build: $(TARGETS) ## 全ターゲットを dist/ へビルド

test: ## ユニットテスト
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(DIST)

install: build ## dist/<os>-<arch>/safersync を PATH へ
	ln -sfn "$(CURDIR)/$(DIST)/darwin-amd64/$(APP)" ~/.local/bin/$(APP)-go

$(TARGETS):
	@os=$(word 1,$(subst /, ,$@)); arch=$(word 2,$(subst /, ,$@)); \
	echo "== $$os/$$arch =="; \
	CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
	  go build -trimpath -ldflags "-s -w" \
	  -o $(DIST)/$$os-$$arch/$(APP) .

help:
	@awk -F':.*##' '/^[a-zA-Z_.-]+:/ { printf "  make %-12s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
