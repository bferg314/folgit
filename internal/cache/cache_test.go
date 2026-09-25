package cache

import (
	"testing"
	"time"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/provider"
)

func TestRoundTrip(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("LocalAppData", cacheHome)   // windows
	t.Setenv("XDG_CACHE_HOME", cacheHome) // linux
	t.Setenv("HOME", cacheHome)           // darwin

	root := t.TempDir()
	if f := Load(root); len(f.Local) != 0 {
		t.Fatalf("expected empty cache, got %+v", f)
	}

	when := time.Unix(1_700_000_000, 0)
	in := &File{
		Root: root,
		Local: []Local{
			{Path: root + "/a", Status: &gitinfo.Status{Branch: "main", Ahead: 2, LastCommit: when, Remotes: []string{"git@github.com:o/a.git"}}},
			{Path: root + "/b"},
		},
		Remote: []provider.Repo{{Host: "github.com", Owner: "o", Name: "c", PushedAt: when}},
	}
	if err := Save(in); err != nil {
		t.Fatal(err)
	}

	out := Load(root)
	if len(out.Local) != 2 || out.Local[0].Status == nil || out.Local[0].Status.Ahead != 2 ||
		!out.Local[0].Status.LastCommit.Equal(when) || out.Local[1].Status != nil {
		t.Fatalf("local round trip mismatch: %+v", out.Local)
	}
	if len(out.Remote) != 1 || out.Remote[0].Key() != "github.com/o/c" {
		t.Fatalf("remote round trip mismatch: %+v", out.Remote)
	}

	if err := Clear(root); err != nil {
		t.Fatal(err)
	}
	if f := Load(root); len(f.Local) != 0 {
		t.Fatal("cache not cleared")
	}
}
