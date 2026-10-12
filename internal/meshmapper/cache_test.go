package meshmapper

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheRechecksLateIATAWithoutCrossRegionCopies(t *testing.T) {
	regions := []Region{{IATA: "YKF", Lat: 43.46, Lon: -80.38, RadiusKm: 10}, {IATA: "YYZ", Lat: 43.68, Lon: -79.63, RadiusKm: 10}}
	yyz := strings.ReplaceAll(strings.ReplaceAll(fixtureCell, "43.4600", "43.6800"), "43.4627", "43.6827")
	yyz = strings.ReplaceAll(strings.ReplaceAll(yyz, "-80.3800", "-79.6300"), "-80.37616", "-79.62616")
	input := `{"regions":[{"grid_squares":[` + fixtureCell + `,` + yyz + `],"region":"YKF"}],"success":true,"global":true,"schema_version":2,"generated_at":1791832854}`
	meta, got, err := parseFeed(strings.NewReader(input), t.TempDir(), regions, fixtureContains, 10)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = writeCache(&out, Options{Regions: regions}, meta, got["YKF"], true); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Count int    `json:"cell_count"`
		Cells []Cell `json:"cells"`
	}
	if err = json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Count != 1 || len(decoded.Cells) != 1 || decoded.Cells[0].Bounds.West != -80.38 {
		t.Fatalf("cross-region cache: %s", out.String())
	}
}

func TestPendingCleanupPreservesOperatorDirectoriesAndCurrentImports(t *testing.T) {
	o := Options{StateDir: t.TempDir(), Now: time.Now()}
	for _, name := range []string{"pending-12345", "pending-notes", "pending-98765"} {
		path := filepath.Join(o.StateDir, name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if name != "pending-98765" {
			old := o.Now.Add(-13 * time.Hour)
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := cleanupPending(o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(o.StateDir, "pending-12345")); !os.IsNotExist(err) {
		t.Fatal("expired generated child retained")
	}
	for _, name := range []string{"pending-notes", "pending-98765"} {
		if _, err := os.Stat(filepath.Join(o.StateDir, name)); err != nil {
			t.Fatal("operator or current data removed")
		}
	}
}
