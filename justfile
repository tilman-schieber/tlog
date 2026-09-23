build:
    go build -o tlog ./cmd/tlog

test:
    go vet ./...
    go test ./...
    node --check app/frontend/app.js
    node app/frontend/app_test.js

# Install the CLI and the outliner into ~/.local/bin (on PATH)
install:
    GOBIN="$HOME/.local/bin" go install ./cmd/tlog

# One-time: the Wails CLI, needed to build the desktop app.
# Goes to ~/.local/bin because that is what is on PATH here, not ~/go/bin.
app-setup:
    GOBIN="$HOME/.local/bin" go install github.com/wailsapp/wails/v2/cmd/wails@latest
    @echo "On Arch the desktop app also needs: pacman -S gtk3 webkit2gtk-4.1"

# Build the desktop app. macOS: app/build/bin/tlog.app — Linux: a binary.
app: (_wails "build" "-s" "-skipbindings")

# Run the desktop app with live reload while working on the frontend
app-dev: (_wails "dev" "-s" "-skipbindings")

# Find the Wails CLI wherever it was installed, and say so plainly if it is not.
_wails *args:
    #!/usr/bin/env bash
    set -euo pipefail
    for candidate in wails "$HOME/.local/bin/wails" "$(go env GOPATH)/bin/wails"; do
      if command -v "$candidate" >/dev/null 2>&1; then
        cd app && exec "$candidate" {{args}}
      fi
    done
    echo "The Wails CLI is not installed. Run:  just app-setup" >&2
    exit 1

# See what a Logseq import would do, without writing anything
import-dry:
    go run ./cmd/tlog import -dry-run

# Cross-compile CLI release binaries into dist/
dist:
    GOOS=darwin GOARCH=arm64 go build -o dist/tlog-darwin-arm64 ./cmd/tlog
    GOOS=darwin GOARCH=amd64 go build -o dist/tlog-darwin-amd64 ./cmd/tlog
    GOOS=linux  GOARCH=amd64 go build -o dist/tlog-linux-amd64 ./cmd/tlog
    GOOS=linux  GOARCH=arm64 go build -o dist/tlog-linux-arm64 ./cmd/tlog
