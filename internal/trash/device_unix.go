//go:build !windows

package trash

import (
	"os"
	"syscall"
)

// sameDevice reports whether a and b are on the same filesystem, so a
// rename between them won't fail.
func sameDevice(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	sa, ok1 := fa.Sys().(*syscall.Stat_t)
	sb, ok2 := fb.Sys().(*syscall.Stat_t)
	return ok1 && ok2 && sa.Dev == sb.Dev
}
