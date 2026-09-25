// Package cache persists the last scan and remote listing per root directory
// so folgit can draw a full screen instantly and refresh in the background.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/provider"
)

// version is bumped whenever the file format changes; older files are ignored.
const version = 1

// File is the cached state for one root.
type File struct {
	Version  int             `json:"version"`
	Root     string          `json:"root"`
	SavedAt  time.Time       `json:"saved_at"`
	Local    []Local         `json:"local"`
	Remote   []provider.Repo `json:"remote,omitempty"`
	RemoteAt time.Time       `json:"remote_at,omitzero"`
}

// Local is one cached repository. Status is nil if it never loaded.
type Local struct {
	Path   string          `json:"path"`
	Status *gitinfo.Status `json:"status,omitempty"`
}

// Load returns the cache for root, or an empty File if there is none or it
// is unreadable. A bad cache is never fatal: the scan rebuilds it.
func Load(root string) *File {
	empty := &File{Version: version, Root: root}
	path, err := pathFor(root)
	if err != nil {
		return empty
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return empty
	}
	var f File
	if json.Unmarshal(data, &f) != nil || f.Version != version || !sameRoot(f.Root, root) {
		return empty
	}
	return &f
}

// Save writes f atomically (temp file + rename).
func Save(f *File) error {
	path, err := pathFor(f.Root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f.Version = version
	f.SavedAt = time.Now()
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "cache-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Clear removes the cache for root.
func Clear(root string) error {
	path, err := pathFor(root)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func pathFor(root string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(normalize(root)))
	return filepath.Join(dir, "folgit", hex.EncodeToString(sum[:8])+".json"), nil
}

func sameRoot(a, b string) bool { return normalize(a) == normalize(b) }

// normalize makes paths that differ only in case map to the same cache on
// case-insensitive filesystems.
func normalize(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}
