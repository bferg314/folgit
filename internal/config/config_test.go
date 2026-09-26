package config

import (
	"runtime"
	"testing"
)

func TestMigrateAgy(t *testing.T) {
	c := &Config{Tools: []Tool{
		{Name: "Antigravity", Cmd: "agy", Args: []string{"{path}"}, Key: "a", Mode: ModeDetach},
		{Name: "Custom agy", Cmd: "agy", Args: []string{"--new-window", "{path}"}, Key: "A", Mode: ModeDetach},
	}}
	changed := c.migrate()

	if runtime.GOOS != "linux" {
		if changed || c.Tools[0].Mode != ModeDetach {
			t.Fatalf("migrate changed agy on %s: %+v", runtime.GOOS, c.Tools[0])
		}
		return
	}
	if !changed || c.Tools[0].Mode != ModeTerminal || len(c.Tools[0].Args) != 0 {
		t.Fatalf("old default not migrated: %+v", c.Tools[0])
	}
	if c.Tools[1].Mode != ModeDetach {
		t.Fatalf("user-customised tool was changed: %+v", c.Tools[1])
	}
}
