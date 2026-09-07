package app

import (
	"hash/fnv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/model"
)

// Pair colors depend only on the appended text, so matching sockets keep the
// same color across letters, projects, reloads and changes to the marker list.
// Case and surrounding spaces do not create a different visual pair.
func attachmentPairColor(text string) rl.Color {
	key := strings.ToLower(strings.TrimSpace(text))
	if key == "" {
		return markerColor(model.MarkerAttachment)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	hue := float32(h.Sum32()%36000) / 100
	return rl.ColorFromHSV(hue, 0.48, 1)
}

func placedMarkerColor(m model.Marker) rl.Color {
	if m.Kind == model.MarkerAttachment {
		return attachmentPairColor(m.AppendText)
	}
	return markerColor(m.Kind)
}
