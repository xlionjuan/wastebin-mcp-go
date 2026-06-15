# justfile for wastebin-mcp-go
#
# Common commands:
#   just           — run default (test)
#   just build     — build binary
#   just test      — run tests with race detector
#   just check     — full pre-PR gate (fmt → vet → lint → test-cover)
#   just clean     — remove build artifacts

binary := "wastebin-mcp-go"
coverfile := "coverage.out"

# Run tests (default)
default: test

# Build the binary
build:
    go build -o {{ binary }} .

# Run tests with race detector
test:
    go test -race -shuffle=on ./...

# Run tests with coverage
test-cover:
    go test -race -shuffle=on -coverprofile={{ coverfile }} ./...

# Run tests with verbose output
test-verbose:
    go test -race -shuffle=on -v ./...

# Format code
fmt:
    golangci-lint fmt

# Run go vet
vet:
    go vet ./...

# Run golangci-lint
lint:
    golangci-lint run --timeout 5m

# Verify and tidy dependencies
mod-tidy:
    go mod tidy
    git diff --exit-code go.mod go.sum

# Verify module checksums
mod-verify:
    go mod verify

# View coverage in browser
cover:
    go tool cover -html={{ coverfile }}

# View coverage as text
cover-text:
    go tool cover -func={{ coverfile }}

# Full pre-PR gate: fmt → vet → lint → test-cover
check: fmt vet lint test-cover

# CI is an alias for check
alias ci := check

# Quick check (no formatting, just vet + lint + test)
quick: vet lint test

# Remove build artifacts and coverage files
clean:
    rm -f {{ binary }} {{ coverfile }}

# Run govulncheck
vulncheck:
    go tool govulncheck ./...
