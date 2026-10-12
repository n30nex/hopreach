package main

func calibrationEnabled(c appConfig, gpuAvailable bool) bool {
	return !c.calibratedRequiresGPU || !c.calibratedPrecisionRequiresGPU || gpuAvailable
}
