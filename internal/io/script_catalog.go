package io

import (
	"reflect"
	"sort"
	"strings"
)

// ScriptCatalog is generated from the parser so the live connection advertises
// the same operation names and payload fields used by the application.
func ScriptCatalog() map[string]any {
	names := make([]string, 0, len(knownOps))
	for name := range knownOps {
		if name != "library.testdir" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	fields := map[string]string{}
	t := reflect.TypeFor[Op]()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name != "-" {
			fields[name] = f.Type.String()
		}
	}
	return map[string]any{"operations": names, "fields": fields, "documentation": "docs/AI_CONTROL.md", "coordinates": "sketch coordinates in world units; pointer coordinates in window pixels; paint uv and points in texels"}
}
