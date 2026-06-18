package wastebin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var errOpenEmptyPath = errors.New("open: empty relative path")

// openFileResolved opens a file with symlink-safe semantics:
//   - When allowed paths are configured, uses openat(2) with O_NOFOLLOW from each
//     candidate root directory's pinned fd, so that a post-validation symlink swap
//     can only cause the open to fail (ELOOP) rather than follow a symlink to a
//     blocked path.
//   - When no allowed paths are configured, walks every path component from the
//     root directory / using openat(2) with O_NOFOLLOW, providing the same level
//     of protection against intermediate symlink swaps.
func openFileResolved(resolvedPath string, allowedPaths []string) (*os.File, error) {
	if len(allowedPaths) > 0 {
		return openFileFromRoot(resolvedPath, allowedPaths)
	}

	// Walk from / with openat+O_NOFOLLOW so that no component — intermediate
	// or final — can be a symlink.  This is equivalent to treating "/" as
	// the implicit allowed root.
	return openFileFromRoot(resolvedPath, []string{"/"})
}

// openFileFromRoot opens resolvedPath by locating the narrowest allowed root
// that contains it, pinning that root directory with an os.File, and then
// walking every path component via openat(2) with O_NOFOLLOW from the pinned
// fd.  This guarantees that the final fd refers to the same inode as the
// allowed root, even if the filesystem is concurrently modified.
func openFileFromRoot(resolvedPath string, allowedPaths []string) (*os.File, error) {
	rootPath, relPath, ok := findAllowedRoot(resolvedPath, allowedPaths)
	if !ok {
		return nil, errPathNotAllowed
	}

	//nolint:gosec // rootPath comes from validated allowed paths
	root, err := os.OpenFile(rootPath, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer root.Close() //nolint:errcheck // Read-only directory; close error non-critical

	return openRelNoFollow(root, relPath)
}

// findAllowedRoot finds the first allowed path that contains resolvedPath and
// returns that root along with the relative path from it.  Both inputs and
// outputs are cleaned, absolute paths.
func findAllowedRoot(resolvedPath string, allowedPaths []string) (string, string, bool) {
	cleaned := filepath.Clean(resolvedPath)
	for _, allowed := range allowedPaths {
		a := filepath.Clean(allowed)
		if isContainedPath(a, cleaned) {
			rel, err := filepath.Rel(a, cleaned)
			if err != nil {
				return "", "", false
			}

			return a, rel, true
		}
	}

	return "", "", false
}

// openRelNoFollow walks every component of relPath from the directory fd dir
// using openat(2) with O_RDONLY|O_NOFOLLOW|O_CLOEXEC.  Intermediate components
// are verified to be directories; the final component is returned as an
// *os.File.  If any component is a symlink (or is replaced by one concurrently)
// the call fails with ELOOP instead of following it.
func openRelNoFollow(dir *os.File, relPath string) (*os.File, error) {
	parts := splitPath(relPath)
	if len(parts) == 0 {
		return nil, errOpenEmptyPath
	}

	firstFd := int(dir.Fd())
	parentFd := firstFd

	for i, part := range parts {
		isLast := i == len(parts)-1
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC

		fd, err := unix.Openat(parentFd, part, flags, 0)
		if err != nil {
			if parentFd != firstFd {
				_ = unix.Close(parentFd) //nolint:errcheck // Best-effort close during error unwind
			}

			return nil, err
		}

		if isLast {
			if parentFd != firstFd {
				_ = unix.Close(parentFd) //nolint:errcheck // Best-effort close; fd copied into os.File
			}

			return os.NewFile(uintptr(fd), filepath.Join(dir.Name(), relPath)), nil
		}

		// Verify intermediate component is a directory.
		var stat unix.Stat_t

		fstatErr := unix.Fstat(fd, &stat)
		if fstatErr != nil {
			_ = unix.Close(fd) //nolint:errcheck // Best-effort close during error unwind

			if parentFd != firstFd {
				_ = unix.Close(parentFd) //nolint:errcheck // Best-effort close during error unwind
			}

			return nil, fstatErr
		}

		if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			_ = unix.Close(fd) //nolint:errcheck // Best-effort close for unexpected type

			if parentFd != firstFd {
				_ = unix.Close(parentFd) //nolint:errcheck // Best-effort close; no longer needed
			}

			return nil, errFilePathCannotBeUsed
		}

		if parentFd != firstFd {
			_ = unix.Close(parentFd) //nolint:errcheck // Parent fd closed after child opened
		}

		parentFd = fd
	}

	return nil, errOpenEmptyPath
}

// splitPath splits a relative path into non-empty components.
func splitPath(relPath string) []string {
	cleaned := filepath.ToSlash(filepath.Clean(relPath))
	parts := strings.Split(cleaned, "/")

	var result []string

	for _, p := range parts {
		if p != "" && p != "." {
			result = append(result, p)
		}
	}

	return result
}
