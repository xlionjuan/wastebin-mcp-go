package wastebin

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Sentinel errors are defined in errors.go.

// SandboxMount maps a host path to a sandbox path.
type SandboxMount struct {
	HostPath    string
	SandboxPath string
}

// ParseSandboxMounts parses the WASTEBIN_MCP_SANDBOX_MOUNTS env var format:
// "host1:sand1,host2:sand2".
//
//nolint:funlen,gocyclo // Format parser with per-field validation branches
func ParseSandboxMounts(input string) ([]SandboxMount, error) {
	if input == "" {
		return nil, nil
	}

	var mounts []SandboxMount

	pairs := strings.Split(input, ",")
	for idx, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, ":", 2) //nolint:mnd // splitting into 2 parts is inherent to the format
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf(
				"%w at index %d: %q (expected host:sandbox format)",
				errInvalidSandboxMount, idx, pair,
			)
		}

		hostPath := strings.TrimSpace(parts[0])
		sandboxPath := strings.TrimSpace(parts[1])

		if !strings.HasPrefix(hostPath, "/") {
			return nil, fmt.Errorf(
				"%w at index %d: host path %q must be absolute",
				errInvalidSandboxMount, idx, hostPath,
			)
		}

		if hasSandboxParentDir(sandboxPath) {
			return nil, fmt.Errorf(
				"%w at index %d: sandbox path %q must not contain parent-directory traversal",
				errInvalidSandboxMount, idx, sandboxPath,
			)
		}

		sandboxPath = path.Clean(sandboxPath)

		if !path.IsAbs(sandboxPath) {
			return nil, fmt.Errorf(
				"%w at index %d: sandbox path %q must be absolute",
				errInvalidSandboxMount, idx, sandboxPath,
			)
		}

		mounts = append(mounts, SandboxMount{
			HostPath:    hostPath,
			SandboxPath: sandboxPath,
		})
	}

	// Validate that no sandbox path contains another (overlapping mounts).
	for idx := range mounts {
		for other := idx + 1; other < len(mounts); other++ {
			first := mounts[idx]
			second := mounts[other]

			if isContainedSandboxPath(first.SandboxPath, second.SandboxPath) ||
				isContainedSandboxPath(second.SandboxPath, first.SandboxPath) {
				return nil, fmt.Errorf(
					"%w: sandbox mount %d (%q) overlaps with mount %d (%q); "+
						"each mount's sandbox path must be unique and non-overlapping",
					errOverlappingMounts, idx, first.SandboxPath, other, second.SandboxPath,
				)
			}
		}
	}

	return mounts, nil
}

func hasSandboxParentDir(sandboxPath string) bool {
	for part := range strings.SplitSeq(sandboxPath, "/") {
		if part == parentDirComponent {
			return true
		}
	}

	return false
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
	for _, mount := range t.mounts {
		rel, ok := sandboxRelativePath(mount.SandboxPath, sandboxPath)
		if !ok {
			continue
		}

		if rel == "." {
			return mount.HostPath, true
		}

		return filepath.Join(mount.HostPath, rel), true
	}

	return "", false
}
