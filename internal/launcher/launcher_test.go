package launcher

import "testing"

func TestGitErrorLine(t *testing.T) {
	cases := map[string]string{
		"fatal: 'x' does not appear to be a git repository\nfatal: Could not read from remote repository.\n\nPlease make sure you have the correct access rights\nand the repository exists.\n": "'x' does not appear to be a git repository",
		"hint: something\nerror: cannot pull with rebase\n": "cannot pull with rebase",
		"Not possible to fast-forward, aborting.\n":         "Not possible to fast-forward, aborting.",
	}
	for in, want := range cases {
		if got := gitErrorLine(in); got != want {
			t.Errorf("gitErrorLine(%q) = %q, want %q", in, got, want)
		}
	}
}
