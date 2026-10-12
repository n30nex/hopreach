package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"hopreach/internal/config"
)

func TestRunConfiguredIsolatedSequentialRegionsContinueAfterFailure(t *testing.T) {
	c := config.Default()
	c.Regions.Entries = []config.IATARegion{{IATA: "YKF"}, {IATA: "YYZ"}}
	var seen []string
	err := runConfigured(c, true, false, func(a appConfig) error {
		seen = append(seen, a.source.CoreScope.Region)
		if a.analyticsRoot() != c.OutputDir {
			t.Fatalf("regional history escaped shared analytics root: %s", a.analyticsRoot())
		}
		if !strings.HasSuffix(a.outputDir, "/regions/"+a.source.CoreScope.Region) || !a.forceRecompute {
			t.Fatalf("unscoped config %+v", a)
		}
		if len(seen) == 1 {
			return errors.New("fixture fetch failed")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "YKF") || !reflect.DeepEqual(seen, []string{"YKF", "YYZ"}) {
		t.Fatalf("seen=%v err=%v", seen, err)
	}
}

func TestRunConfiguredPreservesSingleRegion(t *testing.T) {
	c := config.Default()
	calls := 0
	if err := runConfigured(c, false, false, func(a appConfig) error {
		calls++
		if a.outputDir != c.OutputDir {
			t.Fatal(a.outputDir)
		}
		return nil
	}); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
