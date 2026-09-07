package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const previousDefaultTheme = `{
 "bg":"#111319","panel":"#1B1F27","card":"#262C36",
 "stroke":"#37404D","text":"#E8EAF0","textDim":"#9CA6B6",
 "warn":"#FFA63C","error":"#FF5D5D","success":"#3DD68C",
 "viewportTop":"#2A303C","viewportBottom":"#12141A",
 "accents":{"model":"#53A4FF","sketch":"#FFD94A","extrude":"#2ED0CC","boolean":"#B48CFF","paint":"#FF6FB5","marker":"#A6F04E"},
 "axis":{"x":"#E5484D","y":"#46A758","z":"#3E63DD"}
}`

func TestPatinaUpgradesOnlyUntouchedPreviousDefault(t *testing.T) {
	defer ResetTheme()
	for _, custom := range []bool{false, true} {
		ResetTheme()
		data := previousDefaultTheme
		if custom {
			data = strings.Replace(data, "#262C36", "#010203", 1)
		}
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if ok, err := LoadTheme(path); !ok || err != nil {
			t.Fatalf("load: %v", err)
		}
		if custom {
			if ColorCard != rgb(1, 2, 3) || ColorBG != rgb(0x11, 0x13, 0x19) {
				t.Fatal("custom theme changed")
			}
		} else if ColorCard != builtinTheme.card || AccentModel != builtinTheme.model {
			t.Fatal("old default hid the new theme")
		}
		stored, err := os.ReadFile(path)
		if err != nil || string(stored) != data {
			t.Fatal("loading rewrote the user's file")
		}
	}
}
