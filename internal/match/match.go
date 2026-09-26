// Package match normalizes git remote URLs so local clones can be matched
// against repositories listed by a provider.
package match

import (
	"net/url"
	"strings"
)

// Key turns any common remote URL form into "host/owner/repo" (lowercase).
//
//	git@github.com:Owner/Repo.git        -> github.com/owner/repo
//	https://github.com/Owner/Repo        -> github.com/owner/repo
//	ssh://git@github.com:22/Owner/Repo   -> github.com/owner/repo
//
// It returns "" for URLs it does not understand (local paths, etc).
func Key(raw string) string {
	raw = strings.TrimSpace(raw)
	var host, path string

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.Scheme == "file" {
			return ""
		}
		host, path = u.Hostname(), u.Path
	} else if at := strings.Index(raw, "@"); at >= 0 {
		// scp-like: user@host:owner/repo
		rest := raw[at+1:]
		h, p, ok := strings.Cut(rest, ":")
		if !ok {
			return ""
		}
		host, path = h, p
	} else {
		return ""
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" {
		return ""
	}
	return strings.ToLower(host + "/" + path)
}

// GitHubRepos returns "owner/repo" for each github.com URL in urls, in
// order and without duplicates.
func GitHubRepos(urls []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range urls {
		host, path, ok := strings.Cut(Key(u), "/")
		if !ok || host != "github.com" || strings.Count(path, "/") != 1 || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}
