package dashboard

import (
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/thiemok/tiny-dash/api/internal/render"
)

// The image endpoints skip dithering when the display offers designColors.
// That is only safe while the template paints with exactly those colors.

var (
	hexColor  = regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b`)
	funcColor = regexp.MustCompile(`\b(rgba?|hsla?)\(`)
)

// designHex returns the designColors as lowercase #rrggbb strings.
func designHex() map[string]bool {
	allowed := map[string]bool{}
	for _, e := range render.PaletteFromColors(designColors) {
		allowed[fmt.Sprintf("#%02x%02x%02x", e.R, e.G, e.B)] = true
	}
	return allowed
}

// expandHex turns #rgb into #rrggbb.
func expandHex(c string) string {
	c = strings.ToLower(c)
	if len(c) == 4 {
		return "#" + strings.Repeat(c[1:2], 2) + strings.Repeat(c[2:3], 2) + strings.Repeat(c[3:4], 2)
	}
	return c
}

func TestTemplatesUseOnlyDesignColors(t *testing.T) {
	allowed := designHex()
	templates := os.DirFS("../../templates")
	err := fs.WalkDir(templates, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		src, err := fs.ReadFile(templates, path)
		if err != nil {
			return err
		}
		for _, c := range hexColor.FindAllString(string(src), -1) {
			if !allowed[expandHex(c)] {
				t.Errorf("%s: color %s is not one of the panel's design colors", path, c)
			}
		}
		if m := funcColor.FindString(string(src)); m != "" {
			t.Errorf("%s: uses %s...), write panel colors as hex", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCalendarPaletteUsesOnlyDesignColors(t *testing.T) {
	allowed := designHex()
	for _, c := range calendarPalette {
		for _, v := range []string{string(c.Fill), string(c.Ink)} {
			if !allowed[expandHex(v)] {
				t.Errorf("calendar color %s is not one of the panel's design colors", v)
			}
		}
		e := render.PaletteFromColors([]byte{c.Index})[0]
		if got := fmt.Sprintf("#%02x%02x%02x", e.R, e.G, e.B); got != string(c.Fill) {
			t.Errorf("calendar color index %d is %s, but Fill is %s", c.Index, got, c.Fill)
		}
	}
}

func TestCalendarColorsOnlyOffersDisplayableFills(t *testing.T) {
	bwyr := render.PaletteFromColors([]byte{0, 1, 2, 3})
	for _, c := range calendarColors(bwyr) {
		if !bwyr.Covers([]byte{c.Index}) {
			t.Errorf("offered %s, which a black/white/yellow/red display can't show", c.Fill)
		}
	}
	if got := calendarColors(render.PaletteFromColors([]byte{0, 1})); len(got) != 1 || got[0].Fill != "#000000" {
		t.Errorf("black/white display got %v, want a single black fill", got)
	}
}
