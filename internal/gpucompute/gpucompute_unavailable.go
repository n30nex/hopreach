//go:build !cgo || !((linux && amd64) || (darwin && (amd64 || arm64)) || (windows && amd64) || android)

// Package gpucompute reports GPU availability to the coverage dispatcher.
// wgpu v0.17.1 has no Linux ARM64 library, and requires cgo on every platform.
// These builds use the existing compute.Engine CPU/remote fallback instead.
package gpucompute

import (
	"errors"

	"hopreach/internal/demgrid"
	"hopreach/internal/propagation"
)

// Backend preserves the dispatcher/worker API; Init never creates one here.
type Backend struct {
	AdapterID string
}

// No GPU buffers can be allocated by this backend. These limits are only
// consulted after successful Init, so a zero budget is never used by the CPU.
const MaxElevChunks = 0
const ElevChunkBudgetBytes = 0

var errUnavailable = errors.New("GPU compute unavailable in this build: wgpu-native requires cgo and a supported platform; use gpu.mode=auto or cpu")

// Init rejects GPU use explicitly so auto mode falls back and forced GPU fails.
func Init() (*Backend, error) { return nil, errUnavailable }

// Verify cannot establish GPU correctness on a build without GPU support.
func Verify(*Backend) error { return errUnavailable }

// ComputeMargins never fabricates GPU output; the dispatcher owns CPU fallback.
func ComputeMargins(*Backend, *demgrid.Grid, []propagation.Site, propagation.Bounds, int, int, float64, propagation.Params, func(done, total int)) ([]float32, error) {
	return nil, errUnavailable
}
