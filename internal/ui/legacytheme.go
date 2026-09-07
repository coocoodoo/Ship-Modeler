package ui

// Early releases wrote a complete default theme.json on first launch. That
// unedited file must not freeze users on the old built-in appearance. Custom
// themes are retained verbatim; only the exact previous default is upgraded.
func upgradeLegacyDefault(t ThemeFile) ThemeFile {
	old := DefaultThemeFile()
	old.BG, old.Panel, old.Card = "#111319", "#1B1F27", "#262C36"
	old.Stroke, old.Text, old.TextDim = "#37404D", "#E8EAF0", "#9CA6B6"
	old.Warn, old.Error, old.Success = "#FFA63C", "#FF5D5D", "#3DD68C"
	old.ViewportTop, old.ViewportBottom = "#2A303C", "#12141A"
	old.Accents.Model, old.Accents.Boolean = "#53A4FF", "#B48CFF"
	old.Accents.Sketch = "#FFD94A"
	if t == old {
		return DefaultThemeFile()
	}
	return t
}
