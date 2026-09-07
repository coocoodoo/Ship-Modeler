module modeler-patina-assets

go 1.26

require github.com/patina-ui/patina v0.0.0

require github.com/ebitengine/purego v0.11.0 // indirect

// Patina lives at github.com/coocoodoo/Patina, pinned to the revision the
// embedded assets were generated from; its go.mod declares the patina-ui path.
replace github.com/patina-ui/patina => github.com/coocoodoo/Patina v0.0.0-20260907160806-28143d519861
