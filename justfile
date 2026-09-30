set shell := ["bash", "-euo", "pipefail", "-c"]

# Tool versions are pinned here and installed into .cache/tools/bin.
golangci_lint := "v2.14.0"
gofumpt := "v0.12.0"
goimports := "v0.50.0"
goreleaser := "v2.18.2"
actionlint := "v1.7.12"

tools_bin := justfile_directory() / ".cache/tools/bin"
bd_versions := "1.2.2 1.3.0"

default:
    @just --list

# Install the pinned lint, format and release tools; skips tools already at the pinned version.
tools:
    #!/usr/bin/env bash
    set -euo pipefail
    bin="{{ tools_bin }}"
    mkdir -p "$bin"
    install() {
        local name=$1 version=$2 pkg=$3
        local stamp="$bin/.$name-$version"
        [ -e "$stamp" ] && return 0
        GOBIN="$bin" go install "$pkg@$version"
        find "$bin" -maxdepth 1 -name ".$name-*" -delete
        touch "$stamp"
    }
    install golangci-lint {{ golangci_lint }} github.com/golangci/golangci-lint/v2/cmd/golangci-lint
    install gofumpt {{ gofumpt }} mvdan.cc/gofumpt
    install goimports {{ goimports }} golang.org/x/tools/cmd/goimports
    install goreleaser {{ goreleaser }} github.com/goreleaser/goreleaser/v2
    install actionlint {{ actionlint }} github.com/rhysd/actionlint/cmd/actionlint

# Download every fixtured bd release into .cache/bd/<version>/bd.
bd-install:
    for v in {{ bd_versions }}; do scripts/install-bd.sh "$v"; done

build:
    CGO_ENABLED=0 go build ./...

vet:
    go vet ./...

# Everything except real-bd integration tests.
test-short:
    go test -short -count=1 ./...

# Full suite including real-bd integration tests against every pinned bd.
test: bd-install
    BDASH_TEST_REQUIRE_BD=all go test -count=1 ./...

test-race: bd-install
    BDASH_TEST_REQUIRE_BD=all go test -race -count=1 ./...

# Real-bd integration tests against one pinned bd version.
integration version:
    scripts/install-bd.sh {{ version }}
    BDASH_TEST_REQUIRE_BD={{ version }} go test -count=1 -run Integration ./...

# Coverage floor for the pure model package (percent).
model-coverage floor="85":
    #!/usr/bin/env bash
    set -euo pipefail
    out=$(go test -count=1 -cover ./internal/model)
    echo "$out"
    pct=$(sed -nE 's/.*coverage: ([0-9]+)\.[0-9]+% of statements.*/\1/p' <<<"$out")
    [ -n "$pct" ] && [ "$pct" -ge {{ floor }} ] || { echo "internal/model coverage below {{ floor }}%" >&2; exit 1; }

# Re-capture fixtures with the pinned bd binaries and fail when committed testdata is stale.
fixture-freshness: bd-install
    scripts/fixture-freshness.sh

# Rewrite golden files after an intended rendering change.
golden-update:
    pkgs=$(go list -f '{{ "{{" }}.ImportPath{{ "}}" }} {{ "{{" }}join .TestImports " "{{ "}}" }} {{ "{{" }}join .XTestImports " "{{ "}}" }}' ./... | grep internal/testgolden | awk '{ print $1 }'); \
    go test -count=1 -short $pkgs -update

lint: tools
    PATH="{{ tools_bin }}:$PATH" golangci-lint run ./...

fmt: tools
    PATH="{{ tools_bin }}:$PATH" golangci-lint fmt ./...

fmt-check: tools
    test -z "$(PATH="{{ tools_bin }}:$PATH" golangci-lint fmt --diff ./... 2>&1)"

tidy-check:
    go mod tidy -diff

release-check: tools
    PATH="{{ tools_bin }}:$PATH" goreleaser check

workflows-check: tools
    PATH="{{ tools_bin }}:$PATH" actionlint

# Mirrored 1:1 by the GitHub Actions jobs on ubuntu and macOS. test-race runs as its own ubuntu-only job.
ci: tidy-check fmt-check lint vet build test model-coverage release-check workflows-check

# Builds bdash and checks what a real host picks: the clipboard and notification backends and the binary itself.
smoke:
    go test -short -count=1 -run 'Smoke' ./internal/host ./internal/clipboard ./internal/notify
    go build -o ".cache/bdash-smoke$(go env GOEXE)" ./cmd/bdash
    ".cache/bdash-smoke$(go env GOEXE)" --version

# Windows runs build, the non-bd tests and the smoke checks.
ci-windows: build test-short smoke
