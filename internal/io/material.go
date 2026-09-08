package io

import (
	"fmt"
	"github.com/ncruces/zenity"
	"image"
	"image/draw"
	_ "image/jpeg"
	"modeler/internal/geom/mesh"
	"os"
)

func materialMemberName(uid mesh.FaceUID, kind string) string {
	return fmt.Sprintf("paint/%d_%s.png", uid, kind)
}
func AskOpenMaterial() (string, bool, error) {
	return ask(zenity.SelectFile(zenity.Title("Import PBR texture"), zenity.FileFilter{Name: "Texture images", Patterns: []string{"*.png", "*.jpg", "*.jpeg"}, CaseFold: true}))
}
func ReadMaterialImage(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 64<<20 {
		return nil, fmt.Errorf("texture file exceeds 64 MB")
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
		return nil, fmt.Errorf("texture must be between 1 and 4096 pixels per side")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out, nil
}
