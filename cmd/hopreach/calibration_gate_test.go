package main

import "testing"

func TestCalibrationGateAvoidsUnusedEvidenceFetches(t *testing.T) {
	c := appConfig{calibratedRequiresGPU: true, calibratedPrecisionRequiresGPU: true}
	if calibrationEnabled(c, false) {
		t.Fatal("CPU-only run would fetch unused calibration evidence")
	}
	if !calibrationEnabled(c, true) {
		t.Fatal("GPU calibration disabled")
	}
	c.calibratedRequiresGPU = false
	if !calibrationEnabled(c, false) {
		t.Fatal("CPU calibration disabled")
	}
}
