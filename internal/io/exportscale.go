package io

import (
	"fmt"
	"math"
)

const MinModelExportScale = 0.000001
const MaxModelExportScale = 1000000

func ValidateModelExportScale(scale float64) error {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale < MinModelExportScale || scale > MaxModelExportScale {
		return fmt.Errorf("ship scale must be between 0.000001 and 1000000")
	}
	return nil
}

// The optional multiplier defaults to 1 for existing export callers.
func modelExportScale(values []float64) (float64, error) {
	if len(values) == 0 {
		return 1, nil
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("choose one ship scale")
	}
	return values[0], ValidateModelExportScale(values[0])
}
