
APP_NAME = gdaddon
OUT_DIR = build
DIST_DIR = dist
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -ldflags "-X gdaddon/cmd.version=$(VERSION)"

.PHONY: all clean test package package-mac-arm package-linux package-windows

all: mac-arm linux windows

# Run the test suite for both modules (bubblestack is a separate Go module, so a
# root `go test ./...` does not descend into it).
test:
	go test ./...
	cd bubblestack && go test ./...

mac-arm:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(OUT_DIR)/mac-arm64/$(APP_NAME) .

linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(OUT_DIR)/linux/$(APP_NAME) .

windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(OUT_DIR)/windows/$(APP_NAME).exe .

# Build all targets and zip each one into dist/ for a GitHub release.
package: package-mac-arm package-linux package-windows

package-mac-arm: mac-arm
	mkdir -p $(DIST_DIR)
	cd $(OUT_DIR)/mac-arm64 && zip -q -j ../../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-darwin-arm64.zip $(APP_NAME) ../../install_scripts/install.command ../../install_scripts/install.sh

package-linux: linux
	mkdir -p $(DIST_DIR)
	cd $(OUT_DIR)/linux && zip -q -j ../../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-amd64.zip $(APP_NAME) ../../install_scripts/install.sh

package-windows: windows
	mkdir -p $(DIST_DIR)
	cd $(OUT_DIR)/windows && zip -q -j ../../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-amd64.zip $(APP_NAME).exe ../../install_scripts/install.bat

clean:
	rm -rf $(OUT_DIR) $(DIST_DIR)
