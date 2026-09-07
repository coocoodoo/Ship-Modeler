package ui

// ApplyAppearance switches the full interface and viewport palette. Body and
// paint colors are document data and stay unchanged.
func ApplyAppearance(name string) {
	ResetTheme()
	if name != "light" {
		return
	}
	_ = ApplyThemeFile(ThemeFile{
		BG: "#E8EBF1", Panel: "#F7F8FB", Card: "#FFFFFF", Stroke: "#C5CBD6",
		Text: "#202633", TextDim: "#586174",
		Warn: "#956000", Error: "#C02E40", Success: "#16734B",
		ViewportTop: "#E5EAF2", ViewportBottom: "#CAD3E0",
		Accents: ThemeAccents{Model: "#5054C9", Sketch: "#806400", Extrude: "#007A78",
			Boolean: "#803EB6", Paint: "#B62B72", Marker: "#497318"},
	})
	ColorGridMinor = rgba(0x20, 0x26, 0x33, 0x20)
	ColorGridMajor = rgba(0x20, 0x26, 0x33, 0x44)
	ColorHover = rgba(0x20, 0x26, 0x33, 0x14)
	ColorBevel = rgba(0xFF, 0xFF, 0xFF, 0x80)
	ColorShadow = rgba(0x20, 0x26, 0x33, 0x20)
}
