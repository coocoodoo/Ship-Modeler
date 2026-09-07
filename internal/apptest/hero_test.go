package apptest

import (
	"strings"
	"testing"
)

// The README's hero image is rendered by readme_hero, which is the sample ship
// with three seconds let pass before the shot. Reported against the old one:
// "update the screenshot as well in github, is using old UI" — and the
// regenerated golden still carried the three toasts the build leaves behind,
// because settle returns the moment nothing is animating and a toast counting
// down is not an animation. The wait op is what lets them go.
func TestTheHeroShotHasNoToastsOnIt(t *testing.T) {
	stdout, _ := runScript(t, "readme_hero")
	if strings.Contains(stdout, "\ntoast \"") {
		t.Fatalf("the hero dump still lists toasts:\n%s", stdout)
	}
}

// The negative control: the same ship without the wait does leave toasts, so
// the test above is checking the wait and not an absence that was always there.
func TestTheSampleShipDoesLeaveToastsBehind(t *testing.T) {
	stdout, _ := runScript(t, "m9_sample")
	if !strings.Contains(stdout, "\ntoast \"") {
		t.Fatal("m9_sample no longer leaves toasts, so the hero test proves nothing")
	}
}
