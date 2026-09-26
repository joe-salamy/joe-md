package ui

import (
	"testing"

	"github.com/joe-salamy/joe-md/internal/config"
)

func TestPaletteOver(t *testing.T) {
	p := palettes["dracula"].over(config.Theme{Accent: "39"})
	if p.Accent != "39" {
		t.Errorf("accent = %q, want the setting 39", p.Accent)
	}
	if p.Bar != "#44475a" {
		t.Errorf("bar = %q, want dracula's #44475a", p.Bar)
	}
	if q := palettes["dark"].over(config.Theme{}); q.Accent != "" {
		t.Errorf("dark has accent %q, want the default", q.Accent)
	}
}
