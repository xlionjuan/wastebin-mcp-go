package wastebin

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// errInvalidSandboxMount is returned when a mount string does not match host:sandbox format.
var (
	errInvalidSandboxMount = errors.New("invalid sandbox mount format")
	errOverlappingMounts   = errors.New("overlapping sandbox mount paths")
)

// SandboxMount maps a host path to a sandbox path.
type SandboxMount struct {
	HostPath    string
	SandboxPath string
}

// ParseSandboxMounts parses the WASTEBIN_MCP_SANDBOX_MOUNTS env var format:
// "host1:sand1,host2:sand2".
func ParseSandboxMounts(s string) ([]SandboxMount, error) {
	if s == "" {
		return nil, nil
	}

	var mounts []SandboxMount

	pairs := strings.Split(s, ",")
	for i, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, ":", 2) //nolint:mnd // splitting into 2 parts is inherent to the format
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf(
				"%w at index %d: %q (expected host:sandbox format)",
				errInvalidSandboxMount, i, pair,
			)
		}

		hostPath := strings.TrimSpace(parts[0])
		sandboxPath := path.Clean(strings.TrimSpace(parts[1]))

		if !strings.HasPrefix(hostPath, "/") {
			return nil, fmt.Errorf(
				"%w at index %d: host path %q must be absolute",
				errInvalidSandboxMount, i, hostPath,
			)
		}

		if !path.IsAbs(sandboxPath) {
			return nil, fmt.Errorf(
				"%w at index %d: sandbox path %q must be absolute",
				errInvalidSandboxMount, i, sandboxPath,
			)
		}

		mounts = append(mounts, SandboxMount{
			HostPath:    hostPath,
			SandboxPath: sandboxPath,
		})
	}

	// Validate that no sandbox path contains another (overlapping mounts).
	for i := range mounts {
		for j := i + 1; j < len(mounts); j++ {
			a := mounts[i]
			b := mounts[j]

			if isContainedSandboxPath(a.SandboxPath, b.SandboxPath) ||
				isContainedSandboxPath(b.SandboxPath, a.SandboxPath) {
				return nil, fmt.Errorf(
					"%w: sandbox mount %d (%q) overlaps with mount %d (%q); "+
						"each mount's sandbox path must be unique and non-overlapping",
					errOverlappingMounts, i, a.SandboxPath, j, b.SandboxPath,
				)
			}
		}
	}

	return mounts, nil
}

// isContainedSandboxPath reports whether target is under or equal to base.
// Sandbox paths are always POSIX-style paths, independent of the host OS.
func isContainedSandboxPath(base, target string) bool {
	_, ok := sandboxRelativePath(base, target)

	return ok
}

func sandboxRelativePath(base, target string) (string, bool) {
	base = path.Clean(base)
	target = path.Clean(target)

	if !path.IsAbs(base) || !path.IsAbs(target) {
		return "", false
	}

	if target == base {
		return ".", true
	}

	if base == "/" {
		return strings.TrimPrefix(target, "/"), true
	}

	prefix := base + "/"
	if !strings.HasPrefix(target, prefix) {
		return "", false
	}

	return strings.TrimPrefix(target, prefix), true
}

// Translator translates sandbox paths to host paths.
type Translator struct {
	mounts []SandboxMount
}

// NewTranslator creates a new Translator from the given mounts.
func NewTranslator(mounts []SandboxMount) *Translator {
	return &Translator{mounts: mounts}
}

// isUnderMountHost checks whether the given path is under any configured
// mount's HostPath (either equal to it or a subdirectory).
func isUnderMountHost(path string, mounts []SandboxMount) bool {
	cleaned := filepath.Clean(path)

	for _, m := range mounts {
		hostClean := filepath.Clean(m.HostPath)
		if isContainedPath(hostClean, cleaned) {
			return true
		}
	}

	return false
}

// Translate converts sandbox path to host path.
// Returns empty string and false if no mount matches.
func (t *Translator) Translate(sandboxPath string) (string, bool) {
	for _, m := range t.mounts {
		rel, ok := sandboxRelativePath(m.SandboxPath, sandboxPath)
		if !ok {
			continue
		}

		if rel == "." {
			return m.HostPath, true
		}

		return filepath.Join(m.HostPath, rel), true
	}

	return "", false
}
