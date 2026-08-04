.PHONY: all build test install clean build-extensions build-chrome build-vscode build-jetbrains

PREFIX ?= /usr/local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0")
LDFLAGS := -ldflags "-X metastackr/internal/cli.Version=$(VERSION)"

all: build test

build:
	go build $(LDFLAGS) -o git-meta ./cmd/git-meta
	go build $(LDFLAGS) -o metastackrd ./cmd/metastackrd

build-release: build
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o git-meta-darwin-arm64 ./cmd/git-meta
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o git-meta-darwin-amd64 ./cmd/git-meta
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o git-meta-linux-amd64 ./cmd/git-meta
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o git-meta-linux-arm64 ./cmd/git-meta
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o metastackrd-darwin-arm64 ./cmd/metastackrd
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o metastackrd-darwin-amd64 ./cmd/metastackrd
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o metastackrd-linux-amd64 ./cmd/metastackrd
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o metastackrd-linux-arm64 ./cmd/metastackrd

build-extensions: build-chrome build-vscode build-jetbrains

build-chrome:
	@echo "📦 Packaging Chrome Extension..."
	cd extensions/chrome && zip -r ../../metastackr-chrome.zip . -x "*.DS_Store"

build-vscode:
	@echo "📦 Packaging VS Code Extension..."
	cd extensions/vscode && npx -y @vscode/vsce package -o ../../metastackr-vscode.vsix

build-jetbrains:
	@echo "📦 Building JetBrains Plugin..."
	cd extensions/jetbrains && (./gradlew buildPlugin 2>/dev/null || gradle buildPlugin)

test:
	go test -v ./...

install: build
	mkdir -p $(PREFIX)
	cp git-meta $(PREFIX)/git-meta
	@echo "✅ Installed git-meta to $(PREFIX)/git-meta"
	@echo "You can now run 'git meta status', 'git meta checkout', 'git meta push', etc."

clean:
	rm -f git-meta metastackrd metastackr-chrome.zip metastackr-vscode.vsix

