// Package trash moves folders to the desktop trash, so a deletion can be
// undone from the file manager.
package trash

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Dir returns the trash folder path would be moved into, or "" when it
// can't be trashed: there is no trash on this OS, or the trash is on
// another filesystem so moving would mean copying.
func Dir(path string) string {
	dir := trashDir()
	if dir == "" {
		return ""
	}
	// The trash may not exist yet; compare with its nearest ancestor.
	probe := dir
	for {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return ""
		}
		probe = parent
	}
	if !sameDevice(path, probe) {
		return ""
	}
	return dir
}

// trashDir is the user's trash folder for this OS, or "" if there isn't
// one folgit knows how to use.
func trashDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	switch runtime.GOOS {
	case "windows":
		// The Recycle Bin needs the shell API.
		return ""
	case "darwin":
		return filepath.Join(home, ".Trash")
	}
	// The freedesktop.org trash, used by GNOME, KDE and most others.
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" || !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "Trash")
}

// Move moves path into the trash and returns where it went. It never
// copies: if a rename isn't possible, path is left alone and an error is
// returned.
func Move(path string) (string, error) {
	dir := Dir(path)
	if dir == "" {
		return "", errors.New("no trash on the same filesystem")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		return moveUnique(path, dir, nil)
	}

	files, info := filepath.Join(dir, "files"), filepath.Join(dir, "info")
	for _, d := range []string{files, info} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
	}
	// The spec claims a name by creating its .trashinfo exclusively, then
	// moves the file; the info file is what lets it be restored.
	return moveUnique(path, files, func(name string) (func(), error) {
		p := filepath.Join(info, name+".trashinfo")
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		_, werr := fmt.Fprintf(f, "[Trash Info]\nPath=%s\nDeletionDate=%s\n",
			escapePath(path), time.Now().Format("2006-01-02T15:04:05"))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		undo := func() { _ = os.Remove(p) }
		if werr != nil {
			undo()
			return nil, werr
		}
		return undo, nil
	})
}

// moveUnique renames path into dir under its own name, or name.2, name.3
// and so on if taken. claim, when set, reserves a name before the move and
// returns how to release it if the move fails.
func moveUnique(path, dir string, claim func(name string) (func(), error)) (string, error) {
	base := filepath.Base(path)
	for i := 1; i < 1000; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s.%d", base, i)
		}
		dest := filepath.Join(dir, name)
		if _, err := os.Lstat(dest); err == nil {
			continue
		}
		undo := func() {}
		if claim != nil {
			u, err := claim(name)
			if errors.Is(err, os.ErrExist) {
				continue
			}
			if err != nil {
				return "", err
			}
			undo = u
		}
		if err := os.Rename(path, dest); err != nil {
			undo()
			return "", err
		}
		return dest, nil
	}
	return "", errors.New("too many items with this name in the trash")
}

// escapePath percent-encodes an absolute path for a .trashinfo file,
// keeping the slashes.
func escapePath(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
