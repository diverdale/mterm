package tui

import (
	"reflect"
	"testing"
)

func TestThemesIncludesAllShipped(t *testing.T) {
	got := []string{}
	for _, th := range Themes() {
		got = append(got, th.Name)
	}
	want := []string{"midnight", "matrix", "synthwave"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Themes() names = %v, want %v", got, want)
	}
}

func TestSetThemeSwitchesActiveAndStyles(t *testing.T) {
	t.Cleanup(func() { setTheme(Midnight) })

	setTheme(Matrix)
	if active.Name != "matrix" {
		t.Fatalf("active.Name = %q, want matrix", active.Name)
	}
	if got := sty.title.GetForeground(); got != Matrix.Accent {
		t.Fatalf("after setTheme(Matrix), title fg = %v, want %v", got, Matrix.Accent)
	}

	setTheme(Synthwave)
	if active.Name != "synthwave" {
		t.Fatalf("active.Name = %q, want synthwave", active.Name)
	}
}
