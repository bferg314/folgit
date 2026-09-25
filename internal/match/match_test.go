package match

import "testing"

func TestKey(t *testing.T) {
	cases := map[string]string{
		"git@github.com:Owner/Repo.git":          "github.com/owner/repo",
		"https://github.com/Owner/Repo":          "github.com/owner/repo",
		"https://github.com/Owner/Repo.git/":     "github.com/owner/repo",
		"https://user@github.com/owner/repo.git": "github.com/owner/repo",
		"ssh://git@github.com:22/owner/repo.git": "github.com/owner/repo",
		"git://github.com/owner/repo":            "github.com/owner/repo",
		"/home/me/repos/thing":                   "",
		"file:///tmp/thing":                      "",
		"":                                       "",
	}
	for in, want := range cases {
		if got := Key(in); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
}
