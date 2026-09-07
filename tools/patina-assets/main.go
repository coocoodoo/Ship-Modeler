// Regenerate the embedded Modeler icons using Patina's own SVG renderer.
package main

import (
	"flag"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/patina-ui/patina"
	"github.com/patina-ui/patina/icons"
)

func main() {
	out := flag.String("out", "../../internal/ui/patina", "output asset directory")
	flag.Parse()
	if err := patina.Init(); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	assets := map[string]string{
		"home": icons.Home, "check": icons.Check, "cross": icons.Close,
		"pencil": icons.Edit, "trash": icons.Trash, "settings": icons.Settings,
		"import": icons.Download, "export": icons.Upload, "new": icons.File,
		"open": icons.Folder, "copy": icons.Copy, "lock": icons.Lock,
		"search": icons.Search, "refresh": icons.Refresh,
	}
	for name, svg := range assets {
		img, err := patina.RenderSVGTinted(svg, 96, 96, patina.RGB(255, 255, 255))
		if err != nil {
			log.Fatal(err)
		}
		f, err := os.Create(filepath.Join(*out, name+".png"))
		if err != nil {
			log.Fatal(err)
		}
		err = png.Encode(f, img)
		f.Close()
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*out, name+".svg"), []byte(svg), 0644); err != nil {
			log.Fatal(err)
		}
	}
}
