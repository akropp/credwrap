.PHONY: build build-all clean test install

VERSION := 0.1.0
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

# Default: build for current platform
build:
	go build $(LDFLAGS) -o bin/credwrap ./cmd/credwrap
	go build $(LDFLAGS) -o bin/credwrap-server ./cmd/credwrap-server

# Build for all platforms
build-all: clean
	mkdir -p dist
	
	# Linux amd64
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/credwrap-linux-amd64 ./cmd/credwrap
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/credwrap-server-linux-amd64 ./cmd/credwrap-server
	
	# Linux arm64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/credwrap-linux-arm64 ./cmd/credwrap
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/credwrap-server-linux-arm64 ./cmd/credwrap-server
	
	# macOS amd64 (Intel)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/credwrap-darwin-amd64 ./cmd/credwrap
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/credwrap-server-darwin-amd64 ./cmd/credwrap-server
	
	# macOS arm64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/credwrap-darwin-arm64 ./cmd/credwrap
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/credwrap-server-darwin-arm64 ./cmd/credwrap-server
	
	# Create archives
	cd dist && for f in credwrap-* credwrap-server-*; do \
		if [ -f "$$f" ]; then \
			tar czf "$$f.tar.gz" "$$f" && rm "$$f"; \
		fi \
	done
	
	@echo "Built artifacts:"
	@ls -la dist/

# macOS only (both architectures)
build-macos:
	mkdir -p dist
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/credwrap-darwin-amd64 ./cmd/credwrap
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/credwrap-server-darwin-amd64 ./cmd/credwrap-server
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/credwrap-darwin-arm64 ./cmd/credwrap
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/credwrap-server-darwin-arm64 ./cmd/credwrap-server

clean:
	rm -rf bin/ dist/

test:
	go test -v ./...

install: build
	cp bin/credwrap /usr/local/bin/
	cp bin/credwrap-server /usr/local/bin/
