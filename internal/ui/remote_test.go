package ui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/provider"
)

func TestOnlyMineToggleIsSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	a := New(config.Default(), path, t.TempDir(), nil, "")
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
	a.remote.all = []provider.Repo{
		{Host: "github.com", Owner: "ada", Name: "mine", Mine: true},
		{Host: "github.com", Owner: "org", Name: "theirs"},
	}
	a.remote.refresh(a.localKeys())
	if len(a.remote.view) != 2 {
		t.Fatalf("both repos should show by default: %+v", a.remote.view)
	}

	a.remoteKey("m")
	if len(a.remote.view) != 1 || a.remote.view[0].Name != "mine" || a.remote.missing != 1 {
		t.Fatalf("only the user's repo should show: %+v", a.remote.view)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GitHub.OnlyMine {
		t.Error("only_mine should be saved")
	}

	a.remoteKey("m")
	if len(a.remote.view) != 2 {
		t.Errorf("toggling again should show every repo: %+v", a.remote.view)
	}
}
