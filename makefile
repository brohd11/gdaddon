# Go release makefile. Copy into a repo beside main.go and set the three project variables
# below; everything under the "end config" marker is shared verbatim and is rewritten by
# the workspace's render-go.sh -- edit sh-templates/go/templates/makefile.template.
#
#   APP_NAME     the installed binary name. It need not match the repo or the directory --
#                only install.sh's BINARY has to agree with it.
#   VERSION_PKG  full import path of the package declaring `var version = "dev"`:
#                `main` for a stdlib-flag CLI, `<module>/cmd` for a cobra one (`head -1 go.mod`).
#   PLATFORMS    cross-compile targets. Per-repo: not every app runs everywhere.
#
# A wrong VERSION_PKG fails silently: the linker drops an unknown -X symbol and the binary
# just reports "dev". Verify after any change with
#   make && ./build/$(go env GOOS)-$(go env GOARCH)/<APP_NAME> --version

APP_NAME    = gdaddon
VERSION_PKG = github.com/brohd11/gdaddon/cmd
PLATFORMS   = darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64
# ---- end config ----
OUT_DIR     = build
DIST_DIR    = dist
VERSION     = $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     = -ldflags "-s -w -X $(VERSION_PKG).version=$(VERSION)"

HOST_OS     = $(shell go env GOOS)
HOST_ARCH   = $(shell go env GOARCH)

.PHONY: build binary-path all test package clean $(PLATFORMS)

# Host build -> build/<os>-<arch>/$(APP_NAME). Default target so the dev loop compiles one
# target, not five.
build:
	go build $(LDFLAGS) -o $(OUT_DIR)/$(HOST_OS)-$(HOST_ARCH)/$(APP_NAME) .

# Report the host binary path for local tooling, including workspace installation.
binary-path:
	@printf '%s\n' '$(abspath $(OUT_DIR))/$(HOST_OS)-$(HOST_ARCH)/$(APP_NAME)'

# Run this module's test suite. The sibling modules are separate repos consumed as tagged
# dependencies and each has its own CI, so this covers only the module it sits in.
test:
	go test ./...

# Cross-compile every release target.
all: $(PLATFORMS)

$(PLATFORMS):
	@os=$(word 1,$(subst /, ,$@)); arch=$(word 2,$(subst /, ,$@)); \
	ext=$$( [ "$$os" = "windows" ] && echo .exe || echo ); \
	echo "building $$os/$$arch"; \
	GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
	  go build $(LDFLAGS) -o $(OUT_DIR)/$$os-$$arch/$(APP_NAME)$$ext .

# Build all targets, then archive each into dist/ for a GitHub release.
#
# Archive names are version-less on purpose: it lets install.sh use GitHub's
# /releases/latest/download/<name> redirect and skip the API entirely. The release tag
# carries the version. The name must match install.sh's "$BINARY-$target.$ARCHIVE_EXT",
# so APP_NAME and BINARY have to agree. Archives are flat -- one bare executable at the root.
package: all
	@mkdir -p $(DIST_DIR); \
	for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; \
	  ext=$$( [ "$$os" = "windows" ] && echo .exe || echo ); \
	  name=$(APP_NAME)-$$os-$$arch.zip; \
	  echo "packaging $$name"; \
	  rm -f $(DIST_DIR)/$$name; \
	  ( cd $(OUT_DIR)/$$os-$$arch && zip -q -j ../../$(DIST_DIR)/$$name $(APP_NAME)$$ext ); \
	done; \
	echo "done -> $(DIST_DIR)/"

clean:
	rm -rf $(OUT_DIR) $(DIST_DIR)
