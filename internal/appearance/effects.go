// Package appearance contains portable interface preferences, separate from
// document data and from either rendering engine.
package appearance

type Effects struct {
	Glow            bool   `json:"glow"`
	ColorCycle      bool   `json:"colorCycle"`
	Ripple          bool   `json:"ripple"`
	Gradients       bool   `json:"gradients"`
	HoverLift       bool   `json:"hoverLift"`
	IconSpin        bool   `json:"iconSpin"`
	IconPulse       bool   `json:"iconPulse"`
	SVGAnimation    bool   `json:"svgAnimation"`
	PathMotion      bool   `json:"pathMotion"`
	Entrances       bool   `json:"entrances"`
	MenuAnimation   bool   `json:"menuAnimation"`
	Tooltips        bool   `json:"tooltips"`
	ThemeFade       bool   `json:"themeFade"`
	PreviewRotation bool   `json:"previewRotation"`
	MotionStyle     string `json:"motionStyle"`
	PressEffect     string `json:"pressEffect"`
}

func Defaults() Effects {
	return Effects{Glow: true, Ripple: true, Gradients: true, IconPulse: true,
		SVGAnimation: true, PathMotion: true, Entrances: true, MenuAnimation: true,
		Tooltips: true, ThemeFade: true, PreviewRotation: true, MotionStyle: "ease", PressEffect: "bounce"}
}

func (e *Effects) Normalize() {
	switch e.MotionStyle {
	case "ease", "spring", "bouncy":
	default:
		e.MotionStyle = "ease"
	}
	switch e.PressEffect {
	case "none", "subtle", "bounce", "rubber", "gelatin":
	default:
		e.PressEffect = "subtle"
	}
}
