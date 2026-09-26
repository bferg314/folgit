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

func TestGitHubRepos(t *testing.T) {
	got := GitHubRepos([]string{
		"git@github.com:Me/Fork.git",
		"https://github.com/upstream/fork",
		"https://github.com/me/fork", // duplicate of the first
		"https://gitlab.com/me/other",
		"/local/path",
	})
	want := []string{"me/fork", "upstream/fork"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("GitHubRepos = %v, want %v", got, want)
	}
}
