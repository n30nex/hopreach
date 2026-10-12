//go:build !cgo || !((linux && amd64) || (darwin && (amd64 || arm64)) || (windows && amd64) || android)

package gpucompute_test

import (
	"math"
	"strings"
	"testing"

	"hopreach/internal/compute"
	"hopreach/internal/demgrid"
	"hopreach/internal/gpucompute"
	"hopreach/internal/propagation"
)

func TestUnavailableGPUFailsExplicitly(t *testing.T) {
	be, err := gpucompute.Init()
	if be != nil || err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Init() = %v, %v; want nil backend and an unavailable error", be, err)
	}
	if err := gpucompute.Verify(nil); err == nil {
		t.Fatal("Verify accepted an unavailable GPU")
	}
	out, err := gpucompute.ComputeMargins(nil, nil, nil, propagation.Bounds{}, 0, 0, 0, propagation.Params{}, nil)
	if out != nil || err == nil {
		t.Fatalf("ComputeMargins() = %v, %v; want no output and an error", out, err)
	}
}

func TestAutoUsesTrustedCPUWhenGPUUnavailable(t *testing.T) {
	engine := compute.New()
	engine.Setup("auto")
	if engine.Available() || engine.LocalAdapterID() != "" {
		t.Fatal("CPU-only build advertised a GPU")
	}
	grid, err := demgrid.NewFromElev(0, 0, 0, 1, 1, make([]float32, 256*256))
	if err != nil {
		t.Fatal(err)
	}
	sites := []propagation.Site{{Lat: 43.025, Lon: -80.05, TxHeightM: 10}}
	bounds := propagation.Bounds{South: 43, North: 43.05, West: -80.1, East: -80}
	p := propagation.Params{FrequencyMHz: 915, TxPowerDBm: 22, RxSensitivityDB: -124, RxHeightM: 2}
	want := propagation.ComputeMarginsCPU(grid, sites, bounds, 4, 4, 20, p, nil)
	got := engine.Margins(grid, sites, bounds, 4, 4, 20, p, nil)
	if len(got) != len(want) {
		t.Fatalf("pixel count = %d, want %d", len(got), len(want))
	}
	covered := 0
	for i := range want {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("pixel %d = %v, CPU reference = %v", i, got[i], want[i])
		}
		if !math.IsNaN(float64(got[i])) {
			covered++
		}
	}
	if covered == 0 {
		t.Fatal("fixture did not exercise any predicted coverage")
	}
}
