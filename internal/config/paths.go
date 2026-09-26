package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var winEnv = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_()]*)%`)

// ExpandPath expands a leading ~ to the home directory and environment
// variables written as $VAR, ${VAR} or %VAR% (Windows style).
func ExpandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	p = winEnv.ReplaceAllStringFunc(p, func(m string) string {
		if v, ok := os.LookupEnv(m[1 : len(m)-1]); ok {
			return v
		}
		return m
	})
	p = os.ExpandEnv(p)
	return filepath.Clean(p)
}

// ResolveDir picks the directory to scan: an explicit argument wins, then
// DefaultDir, then the current directory. If DefaultDir is set but isn't
// a directory, the current directory is used and notice explains why.
func (c *Config) ResolveDir(arg string) (dir, notice string) {
	if arg != "" {
		return arg, ""
	}
	if c.DefaultDir == "" {
		return ".", ""
	}
	d := ExpandPath(c.DefaultDir)
	if fi, err := os.Stat(d); err == nil && fi.IsDir() {
		return d, ""
	}
	return ".", "Default folder " + c.DefaultDir + " not found; opened the current folder instead"
}
