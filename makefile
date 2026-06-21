
APP_NAME = gdaddon
OUT_DIR = build
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -ldflags "-X gdaddon/cmd.version=$(VERSION)"

.PHONY: all clean

all: mac-arm mac-intel linux windows

mac-arm:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(OUT_DIR)/mac-arm64/$(APP_NAME) .

mac-intel:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(OUT_DIR)/mac-x86_64/$(APP_NAME) .

linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(OUT_DIR)/linux/$(APP_NAME) .

windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(OUT_DIR)/windows/$(APP_NAME).exe .
 
clean:
	rm -rf $(OUT_DIR)