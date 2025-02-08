.PHONY: build build-linux build-mac

# Build for both Linux and macOS
build: build-linux build-mac

# Build for Linux
build-linux:
	@echo "Building for Linux..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/pomo_linux main.go

# Build for macOS
build-mac:
	@echo "Building for macOS..."
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/pomo_mac main.go
