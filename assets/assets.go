// Package assets holds the files compiled into the executable: the sample ship
// and anything else the program must have without a folder beside it (PLAN §4).
//
// A release is one file. Anything the program needs in order to start — or to
// show a first-time user what it can do — has to be inside that file, not
// beside it, or the first thing a fresh install does is fail to find something.
package assets

import _ "embed"

// SampleShip is the op script that builds the ship offered on the welcome
// screen (SPEC-UX §14).
//
// It is a script rather than a saved .ship on purpose: it doubles as the
// full-app end-to-end test — every tool the program has, driven in order, with
// the result compared against a golden — and as the README's hero image
// generator. A .ship would only prove the loader works.
//
//go:embed sample_ship.json
var SampleShip []byte
