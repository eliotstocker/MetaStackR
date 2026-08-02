.PHONY: all build test install clean

PREFIX ?= /usr/local/bin

all: build test

build:
	go build -o git-meta ./cmd/git-meta
	go build -o metastackrd ./cmd/metastackrd

test:
	go test -v ./...

install: build
	mkdir -p $(PREFIX)
	cp git-meta $(PREFIX)/git-meta
	@echo "✅ Installed git-meta to $(PREFIX)/git-meta"
	@echo "You can now run 'git meta status', 'git meta checkout', 'git meta push', etc."

clean:
	rm -f git-meta metastackrd
