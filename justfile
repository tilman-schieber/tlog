build:
    go build -o tlog ./cmd/tlog

test:
    go vet ./...
    go test ./...
    node omarchy/tests/model_test.js

# What CI runs, so a red build is reproducible here rather than only on GitHub.
# -race is not optional: the watcher runs a sweep goroutine and the store keeps
# a map of what it wrote.
ci:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then echo "not gofmt'd:"; echo "$unformatted"; exit 1; fi
    go vet ./...
    go test -race -count=1 ./...
    node omarchy/tests/model_test.js
    for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
      GOOS=${target%/*} GOARCH=${target#*/} go build -o /dev/null ./cmd/tlog
    done
    echo "ci: all pass"

# Install the CLI and the outliner into ~/.local/bin (on PATH)
install:
    GOBIN="$HOME/.local/bin" go install ./cmd/tlog

# Teach Claude Code to drive tlog: link the skill into ~/.claude/skills
skill-install:
    mkdir -p "$HOME/.claude/skills"
    ln -sfn "$(pwd)/skills/tlog" "$HOME/.claude/skills/tlog"
    @echo "linked ~/.claude/skills/tlog — say 'add a task to my notes' and watch it use tlog"

# See what a Logseq import would do, without writing anything
import-dry:
    go run ./cmd/tlog import -dry-run

# Cross-compile CLI release binaries into dist/
dist:
    GOOS=darwin GOARCH=arm64 go build -o dist/tlog-darwin-arm64 ./cmd/tlog
    GOOS=darwin GOARCH=amd64 go build -o dist/tlog-darwin-amd64 ./cmd/tlog
    GOOS=linux  GOARCH=amd64 go build -o dist/tlog-linux-amd64 ./cmd/tlog
    GOOS=linux  GOARCH=arm64 go build -o dist/tlog-linux-arm64 ./cmd/tlog
